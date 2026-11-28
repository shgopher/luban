# 44.30-mysql-emergency：应急手册的四组实验

[44.30 MySQL 应急手册](../../../v2/L4-组件与技术/44-数据库/44.30-MySQL应急手册.md) 的配套代码（MySQL 8.0，Docker）。

## 跑法

```bash
cd tools/examples/44.30-mysql-emergency
bash code/run.sh              # 默认端口 33199
PORT=33199 bash code/run.sh   # 指定端口
```

前置条件：

- Docker（首次会拉取 `mysql:8.0`，镜像约 600MB）；
- 本机有 `mysql` 8.0 客户端（脚本用它在宿主机发起多 session）；
- 至少 1.5GB 空闲内存：实验会建一张 60.5MB 的表并把结果集流式发出去。

脚本做的事：起容器 → 建 20 万行小表 `t` 和 60.5MB 大表 `big` → 造一个 20 万行不提交的长事务 → 跑巡检 SQL → `KILL` 长事务观察回滚 → 对 `Sending to client` 的语句先 `KILL QUERY` 再 `KILL CONNECTION` → 打印状态计数器 → 清理。

## 实验文件

| 文件 | 作用 |
|---|---|
| `code/00-setup.sql` | 建库、`t`（20 万行）、`big`（20 万行 × 300B ≈ 60.5MB） |
| `code/01-patrol.sql` | 值班巡检：连接 / 执行 / 阻塞 / 事务一行看完 |
| `code/02-long-trx.sql` | 20 万行更新后不提交，用 `SLEEP(300)` 挂住事务 |
| `code/03-stream.sql` | 60MB 结果集，配合不读下游的管道复现 `Sending to client` |
| `code/04-vars.sql` | 本节涉及的 10 个参数实际值（`max_connections=200` 由启动参数指定） |

## 真实输出

环境：MySQL **8.0.46**（`mysql:8.0` 容器），`innodb_buffer_pool_size=128M`、RR 隔离、页面 16KB。

### 1. 数据量与参数

```text
+--------+----------+--------+
| t_rows | big_rows | big_mb |
+--------+----------+--------+
| 200000 |   200000 |   60.5 |
+--------+----------+--------+
```

```text
+---------------------------+----------------+
| VARIABLE_NAME             | VARIABLE_VALUE |
+---------------------------+----------------+
| innodb_buffer_pool_size   | 134217728      |
| innodb_old_blocks_time    | 1000           |
| innodb_thread_concurrency | 0              |
| join_buffer_size          | 262144         |
| max_connections           | 200            |
| net_buffer_length         | 16384          |
| read_buffer_size          | 131072         |
| sort_buffer_size          | 262144         |
| tmp_table_size            | 16777216       |
| wait_timeout              | 28800          |
+---------------------------+----------------+
```

### 2. 长事务与锁

```text
+-----------------+-----------+---------------------+-------------------+---------------------+
| trx_id          | trx_state | trx_started         | trx_rows_modified | trx_mysql_thread_id |
+-----------------+-----------+---------------------+-------------------+---------------------+
|            2138 | RUNNING   | 2026-11-28 05:57:42 |            200000 |                  19 |
| 562948310228992 | RUNNING   | 2026-11-28 05:57:29 |                 0 |                  14 |
+-----------------+-----------+---------------------+-------------------+---------------------+
```

第二行是流式查询的一致性读事务：`trx_rows_modified=0`、`trx_mysql_thread_id=14`，对应 `SHOW PROCESSLIST` 里的客户端连接。**`innodb_trx` 里的事务不等于「大事务」**。

```text
+------------+---------+
| data_locks | waiting |
+------------+---------+
|     200377 |       0 |
+------------+---------+
```

20 万行更新对应 **200,377** 条锁记录：1 行表级 `IX` + 20 万行 `X` 记录锁 + 间隙/supremum 锁。

### 3. 巡检 SQL（长事务 + `Sending to client` 同时存在）

```text
+-------------------+-----------------+-----------------+-------------------+----------+---------+--------------+
| threads_connected | max_connections | threads_running | sending_to_client | open_trx | big_trx | lock_waiters |
+-------------------+-----------------+-----------------+-------------------+----------+---------+--------------+
|                 4 |             200 |               3 |                 1 |        2 |       1 |            0 |
+-------------------+-----------------+-----------------+-------------------+----------+---------+--------------+
```

`open_trx=2` 但 `big_trx=1`：另一个是只读的一致性读事务。

### 4. `KILL` 长事务后的回滚

```text
KILL 之前：
+---------------+-------+
| Variable_name | Value |
+---------------+-------+
| Com_kill      | 0     |
+---------------+-------+

t+0  s ROLLING BACK rows_modified=176963 op=rollback
t+1  s ROLLING BACK rows_modified=69686  op=rollback
t+1  s ROLLING BACK rows_modified=13205  op=rollback
t+2  s trx gone
```

`trx_rows_modified` 单调下降，是「回滚进行到哪」最直接的指标。20 万行回滚约 2 秒；行数越大越久，且期间锁不释放。

### 5. `Sending to client`：`KILL QUERY` 无效，`KILL CONNECTION` 才退出

```text
+----+---------+------+-------------------+-------------------------+
| id | command | time | state             | info                    |
+----+---------+------+-------------------+-------------------------+
| 65 | Query   |   10 | Sending to client | SELECT * FROM luban.big |
+----+---------+------+-------------------+-------------------------+

-- KILL QUERY 65 之后 2 秒，仍是这一行
+----+---------+------+-------------------+-------------------------+
| id | command | time | state             | info                    |
+----+---------+------+-------------------+-------------------------+
| 65 | Query   |   10 | Sending to client | SELECT * FROM luban.big |
+----+---------+------+-------------------+-------------------------+

-- KILL CONNECTION 65 之后
+-------------+
| still_there |
+-------------+
|           0 |
+-------------+
```

服务端阻塞在网络写，写调用返回之前不会走到检查 `killed` 的埋点。同一时刻的内存：

```text
           before      blocked     delta
VmRSS      529212 kB   529860 kB   +648 kB
cgroup     740827136 B 750477312 B +9.2 MiB（页缓存）
```

60.5MB 的结果集经过服务端，常驻内存只多了不到 1MB——`net_buffer` 是「边读边发」的。

### 6. 状态计数器

```text
+-------------------+-------+
| Variable_name     | Value |
+-------------------+-------+
| Aborted_clients   | 2     |
| Com_kill          | 3     |
| Com_select        | 105   |
| Connections       | 44    |
| Queries           | 161   |
| Questions         | 160   |
| Threads_connected | 1     |
| Threads_running   | 2     |
+-------------------+-------+
```

`Com_kill=3`：长事务 1 次 + 大查询 2 次。`Questions` 与 `Queries` 都含监控连接自身的语句——`mysql` 客户端每次连接都会先执行 `select @@version_comment limit 1`，判读 QPS 时先扣掉这部分。

## 浮动与假设

- `trx_id`、线程 id、`trx_started`、耗时每次不同；20 万行回滚本机 2 秒左右，机器慢会到 5 秒以上。
- `data_locks=200377` 由「20 万行 + 表锁 + supremum」决定，换行数会变。
- `big_mb=60.5` 按 `SUM(LENGTH(pad)+17)` 估算，稳定；不同排序规则/页填充会差几 MB。
- `VmRSS` 的绝对值取决于 Buffer Pool 是否已被预热；看**差值**，不看绝对值。60MB 表在 128M Buffer Pool 里会被缓存，第二次跑增量更小。
- 60MB 流式阶段对宿主内存敏感：同时跑多个 MySQL 容器时，Docker VM 可能 OOM 并移除容器（现象是 `ERROR 2013` 后连接被拒）。单实例、≥1.5GB 空闲内存可稳定复现。
- 并发槽位实验（`innodb_thread_concurrency=1` 让两条 10 表 join 从 2.583s 变成 3.816s）不在此脚本内，命令与输出见正文。
