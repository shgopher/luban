# 44.26-mysql-lifecycle：一条 SQL 的一生（Server 层 + InnoDB 日志）

[44.26 MySQL 一条 SQL 的一生](../../../v2/L4-组件与技术/44-数据库/44.26-MySQL一条SQL的一生.md) 的配套实验：起一个一次性 MySQL 8.0，跑四段观察——参数默认值、一个 update 的 binlog 形态、执行器的 `rows_examined`、redo 的四个 LSN（双 1 与 `flush_log_at_trx_commit=0` 两态对比）。

## 跑法

```bash
cd tools/examples/44.26-mysql-lifecycle
bash code/run.sh                 # 默认端口 33199
PORT=33197 bash code/run.sh      # 33199 被占用时换一个
```

前置：

- Docker 在跑，镜像 `mysql:8.0`（首次会拉取，约 600MB）
- `mysql` 客户端在 PATH（脚本用它连 `127.0.0.1:<PORT>`）
- `mysqlbinlog` 在 PATH 才做第 7 段行镜像解码；没有就跳过，第 3 段的 `SHOW BINLOG EVENTS` 已能看清事件类型

脚本 `--rm` 起容器、结束 `docker rm -f` 清掉，不往 Docker 里留东西。`long_query_time` 与 `innodb_flush_log_at_trx_commit` 只在容器内改，容器删掉即消失。

## 环境

| 项 | 值 |
|---|---|
| 版本 | MySQL **8.0.46** |
| 隔离级别 | REPEATABLE-READ |
| `innodb_page_size` | 16384（16KB） |
| `innodb_buffer_pool_size` | 128M |
| 表 | `lifecycle.t(id int pk, c int)` 2 行；`lifecycle.big` 2 万行（`pad char(200)`） |
| binlog | `binlog_format=ROW`，`binlog_checksum=CRC32` |

## 真实输出（下面全部是本机跑出来的，未编辑）

### 1) 参数默认值

```text
+---------+-----------------+-----------+--------------+
| version | isolation       | page_size | bp_mb        |
+---------+-----------------+-----------+--------------+
| 8.0.46  | REPEATABLE-READ |     16384 | 128.00000000 |
+---------+-----------------+-----------+--------------+
+-----------------------------------------+-----------+
| Variable_name                           | Value     |
+-----------------------------------------+-----------+
| binlog_cache_size                       | 32768     |
| binlog_checksum                         | CRC32     |
| binlog_format                           | ROW       |
| binlog_group_commit_sync_delay          | 0         |
| binlog_group_commit_sync_no_delay_count | 0         |
| binlog_order_commits                    | ON        |
| innodb_flush_log_at_trx_commit          | 1         |
| innodb_flush_neighbors                  | 0         |
| innodb_io_capacity                      | 200       |
| innodb_io_capacity_max                  | 2000      |
| innodb_log_buffer_size                  | 16777216  |
| innodb_lru_scan_depth                   | 1024      |
| innodb_max_dirty_pages_pct              | 90.000000 |
| innodb_max_dirty_pages_pct_lwm          | 10.000000 |
| innodb_redo_log_capacity                | 104857600 |
| log_bin                                 | ON        |
| sync_binlog                             | 1         |
| wait_timeout                            | 28800     |
+-----------------------------------------+-----------+
```

### 2) 一个 update 的事务形态

```text
+---------------+-----+----------------+-----------+-------------+--------------------------------------+
| Log_name      | Pos | Event_type     | Server_id | End_log_pos | Info                                 |
+---------------+-----+----------------+-----------+-------------+--------------------------------------+
| binlog.000003 |   4 | Format_desc    |         1 |         126 | Server ver: 8.0.46, Binlog ver: 4    |
| binlog.000003 | 126 | Previous_gtids |         1 |         157 |                                      |
| binlog.000003 | 157 | Anonymous_Gtid |         1 |         236 | SET @@SESSION.GTID_NEXT= 'ANONYMOUS' |
| binlog.000003 | 236 | Query          |         1 |         325 | BEGIN                                |
| binlog.000003 | 325 | Table_map      |         1 |         378 | table_id: 85 (lifecycle.t)           |
| binlog.000003 | 378 | Update_rows    |         1 |         432 | table_id: 85 flags: STMT_END_F       |
| binlog.000003 | 432 | Xid            |         1 |         463 | COMMIT /* xid=40 */                  |
| binlog.000003 | 463 | Rotate         |         1 |         507 | binlog.000004;pos=4                  |
+---------------+-----+----------------+-----------+-------------+--------------------------------------+
```

`BEGIN → Table_map → Update_rows → Xid` 就是一个事务的全部事件；`Xid` 是崩溃恢复时拿去和 redo 的 prepare 对上的那个 XID。行镜像（`mysqlbinlog --base64-output=DECODE-ROWS -vv`）：

```text
### UPDATE `lifecycle`.`t`
### WHERE
###   @1=2 /* INT meta=0 nullable=0 is_null=0 */
###   @2=0 /* INT meta=0 nullable=1 is_null=0 */
### SET
###   @1=2 /* INT meta=0 nullable=0 is_null=0 */
###   @2=1 /* INT meta=0 nullable=1 is_null=0 */
# at 463 ... Xid = 40
```

### 3) 执行器的计数

```text
+---------------+-----------+--------------+---------------------------------------------+
| rows_examined | rows_sent | query_time_s | sql_text                                    |
+---------------+-----------+--------------+---------------------------------------------+
|         20000 |         1 |       0.0074 | SELECT COUNT(*) AS cnt FROM big WHERE c > 0 |
+---------------+-----------+--------------+---------------------------------------------+
```

2 万行全表扫描、返回 1 行：`rows_examined` 是执行器**调引擎接口的次数累加**，与 `rows_sent` 无关。

### 4) redo 的四个 LSN

双 1（`innodb_flush_log_at_trx_commit=1`），2 万行 update 之后：

```text
Log sequence number          36947884
Log flushed up to            36947758
Pages flushed up to          29547154
Last checkpoint at           29547154
Modified db pages  437
```

`LSN − Last checkpoint = 7,400,730` 字节：这么多 redo 已经生成、对应的数据页还没落盘；容量 104,857,600 字节，所以还剩 97,456,870 字节可写。

同一负载，`SET GLOBAL innodb_flush_log_at_trx_commit=0`：

```text
Log sequence number          38644482
Log flushed up to            38125773
Pages flushed up to          29547154
Last checkpoint at           29547154
Modified db pages  482
```

`Log flushed` 落后 `LSN` 518,709 字节——这段 redo 只在 `redo log buffer` 或 page cache 里，**mysqld 进程崩溃就没了**，这就是 0 的代价。（掉电本身容器里测不了，只能按语义推。）

## 哪些数会漂

| 数 | 是否漂 | 说明 |
|---|---|---|
| `Log sequence number` 等四个 LSN 的绝对值 | **漂** | 每次启动基线不同，只比较差值（`LSN − checkpoint`、`LSN − Log flushed`）才有意义；差值也在几 MB 量级波动 |
| `Modified db pages` | **漂** | 后台刷脏在持续跑，采样瞬间的值每次不同（本机见过 306~614） |
| `query_time_s` | **漂** | 本机 0.007~0.009 秒，随机器负载变化 |
| binlog 文件名/位点、`table_id`、`xid` | **漂** | 每次启动都不同，事件**类型与顺序**不变 |
| 参数默认值 | 不漂 | 同版本固定；换小版本（8.0.x）可能变，尤其 `innodb_max_dirty_pages_pct` |
| `rows_examined=20000` | 不漂 | 由数据量与索引决定 |

## 两个坑

- 端口冲突：`run.sh` 默认 33199，被别的实验占用时用 `PORT=` 换；脚本会先 `docker rm -f` 同名容器，不会动别人的容器。
- 慢日志可见性：打开慢日志、执行语句、读 `mysql.slow_log` 必须分三次连接。同一条连接里刚写的慢日志行不一定可见，这也是脚本里拆成三条 `mysql` 命令的原因。
