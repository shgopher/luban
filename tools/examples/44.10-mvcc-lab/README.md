# 44.10-mvcc-lab：一致性视图、当前读与长事务

对应正文：[44.10 事务与隔离级别的实现](../../../v2/L4-组件与技术/44-数据库/44.10-事务与隔离级别.md)。

六组实验，全部在 MySQL 8.0.46（Docker）上真跑过：

| 组 | 验证什么 | 关键预期 |
|---|---|---|
| 0 | 连续 `BEGIN`、语句报错不回滚、`SAVEPOINT` | 隐式提交、错误不自动回滚、只撤销保存点之后 |
| 1 | RR 下快照读 vs 当前读 | 快照读 1，`FOR UPDATE` 读 2 |
| 2 | RC 下同一事务两次读 | 2 → 3，每条语句重建视图 |
| 3 | 一致性快照的建立时机 | `START TRANSACTION WITH CONSISTENT SNAPSHOT` 后别人提交也读不到 |
| 4 | 幻读 | 快照读 `count=3`，当前读 `count=4` |
| 5 | 长事务 | `innodb_trx` 里能看到 `trx_started`；老视图下 HLL 涨到 200 以上 |

## 跑法

需要 Docker Desktop 和 `mysql:8.0` 镜像；脚本里全部通过 `docker exec` 执行，不要求宿主机装 mysql 客户端。

```bash
cd tools/examples/44.10-mvcc-lab
bash code/run.sh            # 默认端口 33199
PORT=33299 bash code/run.sh # 端口被占用时换一个
```

脚本用 `--rm` 起容器、`trap` 退出时 `docker rm -f`；中途 Ctrl-C 也会清理。若进程被强杀，手动清理：

```bash
docker rm -f mysqllab-33199
```

正文里的实验端口是 33107（撰稿时的容器），本脚本默认 33199，两者不冲突。

## 关键实现：别在“初始化中的临时实例”上写数据

`mysql:8.0` 镜像启动时会先跑一个临时实例做初始化，它也能响应 `mysqladmin ping`，但不监听 TCP。用 ping 当就绪条件，初始化脚本会写进临时实例，随后临时实例关闭，数据全丢，还会看到 `Lost connection during query`。脚本的探活条件是**正式实例的 TCP 端口**：

```bash
docker exec "$NAME" mysql -h127.0.0.1 -P3306 -uroot -e 'select 1'
```

## 真实输出

`bash code/run.sh` 一次完整运行（MySQL 8.0.46，端口 33199）：

```text
MySQL 8.0.46  port=33199

===== 0. 事务控制：连续 BEGIN / 遇错不回滚 / SAVEPOINT =====
ERROR 1062 (23000) at line 8: Duplicate entry '张飞' for key 'tx_test.PRIMARY'
ERROR 1062 (23000) at line 16: Duplicate entry '张飞' for key 'tx_test2.PRIMARY'
+-----------------------------------+
|                                   |
+-----------------------------------+
| --after double begin + rollback-- |
+-----------------------------------+
+--------+
| name   |
+--------+
| 关羽 |
+--------+
+--------------------------------------------+
|                                            |
+--------------------------------------------+
| --after duplicate error, before rollback-- |
+--------------------------------------------+
+--------+
| name   |
+--------+
| 张飞 |
+--------+
+------------------+
|                  |
+------------------+
| --after commit-- |
+------------------+
+--------+
| name   |
+--------+
| 张飞 |
+--------+
+---------------+
|               |
+---------------+
| --savepoint-- |
+---------------+
+------+
| name |
+------+
| a    |
| c    |
+------+
会话 A 的 thread_id = 13

===== 1. RR：快照读 vs 当前读 =====

===== 2. RC：同一条 SQL 在下一个语句看到新值 =====

===== 3. 快照在 begin 后第一条语句才建立 =====

===== 4. 幻读：快照读不幻读，当前读幻读 =====

===== 5. 长事务：innodb_trx 与 History list length =====
--- 5.1 基线 ---
History list length 20
+-----------------+-----------+---------------------+---------------------+-------------------+
| trx_id          | trx_state | trx_started         | trx_isolation_level | trx_rows_modified |
+-----------------+-----------+---------------------+---------------------+-------------------+
| 562948504755416 | RUNNING   | 2026-11-28 06:10:20 | REPEATABLE READ     |                 0 |
+-----------------+-----------+---------------------+---------------------+-------------------+
--- 5.2 B 用 200 个独立事务更新 200 行，A 的旧视图还在 ---
History list length 202
--- 5.3 A 提交 2s 后 ---
History list length 202

===== A 会话输出 =====
CONN_A	13
RR-1 A begin
RR-2 A 快照读	1
RR-4 A 再快照读	1
RR-5 A 当前读 for update	2
RR-6 A commit
RC-1 A begin
RC-2 A 读	2
RC-4 A 再读	3
RC-5 A commit
CS-1 A with consistent snapshot
CS-2 A 读	3
CS-4 A 再读	3
CS-5 A commit
PH-1 A begin
PH-2 A 快照读 count	3
PH-4 A 再快照读 count	3
PH-5 A 当前读 count for update	4
PH-6 A commit
LT-1 A begin
LT-2 A 建视图	0
LT-3 A 仍读旧快照 sum(k)	0
LT-4 A commit
===== B 会话输出 =====
RR-3 B 更新并提交
RC-3 B 更新并提交
CS-3 B 更新并提交
PH-3 B 插入 id=4
```

读数：

- 第 0 组：连续 `BEGIN` 后 `ROLLBACK` 只剩 `关羽`（第一个事务被隐式提交）；主键冲突不影响前一条插入，`COMMIT` 后 `张飞` 落库；`SAVEPOINT` 只撤销 `b`。
- RR 的两次快照读都是 1，中间 B 提交了 `k=2`；`FOR UPDATE` 立刻看到 2。
- RC 的两次读是 2 → 3，视图按语句重建。
- 一致性快照读到 3，B 改成 4 后仍读 3，证明视图在 `START TRANSACTION WITH CONSISTENT SNAPSHOT` 时就建好了。
- 幻读：快照读两次都是 3 行，`FOR UPDATE` 数出 4 行——快照读“看不见”新行，不等于“阻止”插入。
- 长事务：A 只读不写，也把 200 个更新事务的 undo 钉在 history list 上；对照实验（同镜像、无长视图）同样 200 个事务后 HLL 只有 4。本次 HLL 是 20 → 202，另一次运行是 17 → 204，基数与 `trx_id` 每次都不同，看差值。

## 环境与浮动

- MySQL 8.0.46，`innodb_buffer_pool_size=128M`，`innodb_page_size=16384`，默认隔离级别 RR。
- `History list length` 的基线与 `trx_id` 每次跑都不同；关注“有长视图”和“无长视图”的差值，不要对绝对值。
- `trx_started` 是数据库容器内的 UTC 时间，比北京时间晚 8 小时。
- 第 5 组的 200 个更新事务在脚本里用循环生成，跑完约 10 秒。
