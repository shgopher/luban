# 44.13-repl-lab：单实例上能测到的复制面

[44.13 主从复制与读写分离](../../../v2/L4-组件与技术/44-数据库/44.13-主从复制与读写分离.md) 的配套实验。复制本身要两台实例，这个实验只做**单实例可观测、可量化**的部分：binlog 体积（延迟的原料）、并行复制与依赖跟踪的默认值、半同步模块的加载方式、位点与 GTID 集合、探针代价、`read_only` 的边界。

## 跑法

```bash
cd tools/examples/44.13-repl-lab
bash code/run.sh                    # 默认端口 33199，自己起容器，跑完 docker rm -f
PORT=33202 bash code/run.sh         # 端口被占时换一个
SKIP_START=1 PORT=33199 bash code/run.sh   # 复用已就绪的实例，不新建容器
```

前置条件：

- 本机有 Docker，能拉 `mysql:8.0`（实测 `8.0.46`）；
- 有 `mysql` 命令行客户端（宿主机的即可，脚本用 `--protocol=TCP` 连容器）；
- 有 `python3`（只用来算耗时差值）；
- 端口默认 `33199` 未被占用。

脚本动作：`docker run -d --rm` 起容器 → 等就绪 → `code/schema.sql` 建 50,000 行表 → 七个场景 → `docker rm -f`。中途退出由 `trap` 清理容器。**首次启动要 initialize 数据目录，机器负载高时可达 4~5 分钟**，脚本最多等 300 秒；不想反复付这个时间就用 `SKIP_START=1` 复用一个已就绪的同参数实例。

## 环境

| 项 | 值 |
|---|---|
| 版本 | MySQL `8.0.46` |
| 隔离级别 | `REPEATABLE-READ`（默认） |
| `binlog_format` | `ROW`（8.0 默认） |
| `innodb_page_size` | 16384（默认） |
| `innodb_buffer_pool_size` | 128M（启动参数指定） |
| `max_connections` | 200 |
| 数据量 | 表 `big` 50,000 行，行长约 68 字节（`id INT` + `c INT` + `pad CHAR(60)`） |

`code/schema.sql` 用递归 CTE 固定生成 50,000 行，行数不随环境变化，因此 binlog 字节数可复现。

## 期望输出（本机真实粘贴，节选）

```text
== 复用已在 33109 上就绪的实例（SKIP_START=1，不新建容器）==
-- 环境 --
+--------+-----------------+------+-------+--------------+
| ver    | iso             | fmt  | page  | bp_mb        |
+--------+-----------------+------+-------+--------------+
| 8.0.46 | REPEATABLE-READ | ROW  | 16384 | 128.00000000 |
+--------+-----------------+------+-------+--------------+

########## 1. 并行复制与依赖跟踪的默认值 ##########
+------------------------+---------------+
| Variable_name          | Value         |
+------------------------+---------------+
| slave_parallel_type    | LOGICAL_CLOCK |
| slave_parallel_workers | 4             |
+------------------------+---------------+
+--------------------------+---------------+
| Variable_name            | Value         |
+--------------------------+---------------+
| replica_parallel_type    | LOGICAL_CLOCK |
| replica_parallel_workers | 4             |
+--------------------------+---------------+
+----------------------------------------+--------------+
| Variable_name                          | Value        |
+----------------------------------------+--------------+
| binlog_transaction_dependency_tracking | COMMIT_ORDER |
+----------------------------------------+--------------+

########## 2. 半同步模块：默认没装 ##########
-- 装之前 --
-- 装之后 --
+---------------------------------------------+------------+
| Variable_name                               | Value      |
+---------------------------------------------+------------+
| rpl_semi_sync_source_enabled                | OFF        |
| rpl_semi_sync_source_timeout                | 10000      |
| rpl_semi_sync_source_trace_level            | 32         |
| rpl_semi_sync_source_wait_for_replica_count | 1          |
| rpl_semi_sync_source_wait_no_replica        | ON         |
| rpl_semi_sync_source_wait_point             | AFTER_SYNC |
+---------------------------------------------+------------+

########## 3. 位点与 GTID：CHANGE MASTER TO 要填的东西 ##########
+---------------+----------+--------------+------------------+-------------------+
| File          | Position | Binlog_Do_DB | Binlog_Ignore_DB | Executed_Gtid_Set |
+---------------+----------+--------------+------------------+-------------------+
| binlog.000003 |  2318729 |              |                  |                   |
+---------------+----------+--------------+------------------+-------------------+
+-----------+---------------+
| gtid_mode | gtid_executed |
+-----------+---------------+
| OFF       | []            |
+-----------+---------------+

########## 4. 同一个 50,000 行 UPDATE，三种格式的 binlog 体积 ##########
-- [ROW + binlog_row_image=FULL] binlog 增长 1105121 字节，语句耗时 0.45s
   50000
-- [ROW + binlog_row_image=MINIMAL] binlog 增长 502493 字节，语句耗时 0.42s
   50000
-- [STATEMENT] binlog 增长 347 字节，语句耗时 0.33s
   50000

########## 5. 探针代价：select 1 vs 查真实表（performance_schema 计时）##########
+-------------------------------------------------------+--------+-----------+
| SQL_TEXT                                              | ms     | ROWS_SENT |
+-------------------------------------------------------+--------+-----------+
| SELECT COUNT(*) FROM repl_lab.big                     |  3.738 |         1 |
| SELECT 1 AS probe                                     |  0.122 |         1 |
| TRUNCATE performance_schema.events_statements_history | 52.117 |         0 |
+-------------------------------------------------------+--------+-----------+

########## 6. 只读语义：read_only 与 super_read_only ##########
-- 普通账号（read_only=ON）--
ERROR 1290 (HY000) at line 1: The MySQL server is running with the --read-only option so it cannot execute this statement
-- root 即 SUPER（read_only=ON）--
inserted
1
-- root 即 SUPER（super_read_only=ON）--
ERROR 1290 (HY000) at line 1: The MySQL server is running with the --super-read-only option so it cannot execute this statement

########## 7. 单实例上没有复制关系 ##########
（0 行：本机是单实例，多机才有的字段都不存在）

== 清理：容器 mysqllab-repl-33109 删除 ==
```

说明：本机采集时 Docker 起新实例要 4~5 分钟（宿主 load 18），所以上表用 `SKIP_START=1` 复用了一个同参数容器采集；输出最后一行是脚本的清理提示，复用模式下容器不会被删（由外部脚本关闭）。

## 怎么读这些数字

- **`STATEMENT` 347 字节 vs `ROW` 1,105,121 字节**：同一条更新 5 万行的语句，体积差 3000 倍。体积小是 mixed 存在的理由，但 STATEMENT 下 `UPDATE ... ORDER BY id LIMIT 50000` 会被判 unsafe（`Note 1592`：用了 `LIMIT`，命中哪些行不可预测），这正是主备可能删改不同行的来源。
- **MINIMAL 约为 FULL 的 45%**：只记变更列，回放仍然记主键，所以确定性不变、体积减半。
- **回放时间与写入时间不成比例**：主库 0.3~0.5 秒写完的事务，备库按单线程 2,000 行/秒要回放约 25 秒。这就是大事务把延迟顶到分钟级的算法。
- **探针 0.122ms vs 3.738ms**：`select 1` 不进 InnoDB，所以它的耗时不随表大小与缓存状态变化。同一张表冷缓存首读时 `COUNT(*)` 曾达 366.461ms（另一轮测量），预热后 3.738ms——**探针能测出的东西，取决于它访不访问引擎**。
- **`-- 装之前 --` 为空**：`mysql:8.0` 镜像默认没有加载半同步插件，`SHOW VARIABLES LIKE 'rpl_semi_sync%'` 一行都没有。装上之后才看到 `timeout=10000`（10 秒）、`wait_point=AFTER_SYNC`、`wait_for_replica_count=1`。
- **`read_only` 挡普通账号、不挡 SUPER**：复制线程正是靠 SUPER 权限写备库；`super_read_only` 才连 SUPER 一起挡，这是切换时 fencing 旧主的开关。

## 这个实验测不到什么

- **复制延迟、`Seconds_Behind_Source`、并行回放效果**：需要两台实例，本实验为单机，`SHOW REPLICA STATUS` 返回 0 行。
- **半同步的真实等待与超时退化**：需要至少一个真实从库回 ACK；本实验只能看到插件加载后的默认值。
- **`innodb_thread_concurrency` 把查询挡在门口的现象**：本节不展开，排队行为与实测见 [44.30 MySQL 应急手册](../../../v2/L4-组件与技术/44-数据库/44.30-MySQL应急手册.md)。
