# 44.11-lock-lab：把「加锁规则」跑成 `data_locks` 里的行

[44.11 锁 (行锁 / 间隙锁 / 意向锁 / 死锁)](../../../v2/L4-组件与技术/44-数据库/44.11-锁.md) 的配套实验：一条命令起 MySQL 8.0，用真实并发会话复现七个锁现场，并直接读 `performance_schema.data_locks` / `data_lock_waits`。

## 跑法

```bash
cd tools/examples/44.11-lock-lab
bash code/run.sh            # 默认端口 33199，跑完自动 docker rm -f
PORT=33200 bash code/run.sh # 端口被占时换一个
```

前置条件：

- 本机有 Docker，且能拉 `mysql:8.0`（实测 `8.0.46`）；
- 有 `mysql` 命令行客户端（宿主机的即可，脚本用 `--protocol=TCP` 连容器）；
- 有 `python3`（只用来算耗时差值）；
- 端口 `33199` 未被占用。脚本用 FIFO 给两个/三个会话喂 SQL，因此必须在 bash 下跑，不能用 `sh`。

脚本动作：`docker run -d --rm` 起容器 → 等就绪 → 灌 `code/schema.sql` → 跑 7 个场景 → `docker rm -f`。中途退出也会由 `trap` 清理容器与 FIFO。

## 环境

| 项 | 值 |
|---|---|
| 版本 | MySQL `8.0.46` |
| 隔离级别 | `REPEATABLE-READ`（默认） |
| `binlog_format` | `ROW`（8.0 默认） |
| `innodb_buffer_pool_size` | 128M |
| `innodb_page_size` | 16384（默认） |
| 数据量 | 表 `t` 6 行（场景 4 临时加一行成 7 行），表 `tu` 4 行 |

表结构与《MySQL 实战 45 讲》第 20/21 讲一致：`t(id PK, c KEY, d)`，数据 `(0,0,0),(5,5,5),(10,10,10),(15,15,15),(20,20,20),(25,25,25)`。

## 期望输出（本机真实粘贴，节选）

```text
8.0.46	REPEATABLE-READ	ROW	128.00000000

########## 场景 1：select ... for update 在非唯一索引 c 上 ##########
-- A 的查询结果（当前读：只有 c=5 的 id=5 一行）--
5	5	5
A-done
-- A 持有的行锁 --
1879	c	X,GAP	GRANTED	10, 10
1879	c	X	GRANTED	5, 5
1879	PRIMARY	X,REC_NOT_GAP	GRANTED	5
-- B 插入 (8,8,8)（应为空：落在 c 索引的间隙 (5,10) 里）--
-- 谁在等谁 --
1880	c	X,GAP,INSERT_INTENTION	1879	X,GAP	10, 10
-- A 提交后 B --
B-done

########## 场景 2：update 唯一索引，记录存在 vs 不存在 ##########
-- where id=10（存在）--
1925	PRIMARY	X,REC_NOT_GAP	GRANTED	10
-- where id=7（不存在）--
1971	PRIMARY	X,GAP	GRANTED	10

########## 场景 3：insert 唯一键冲突 → 两个等待者死锁 ##########
-- 三方的锁（S WAITING 就是等着的那把共享 next-key lock）--
2015	NULL	IX	GRANTED	NULL
2015	c	X,REC_NOT_GAP	GRANTED	5, 9
2016	NULL	IX	GRANTED	NULL
2016	c	S	WAITING	5, 9
2017	NULL	IX	GRANTED	NULL
2017	c	S	WAITING	5, 9
-- A 回滚后 B（成功则无输出）--
ERROR 1213 (40001) at line 2: Deadlock found when trying to get lock; try restarting transaction
-- A 回滚后 C（死锁牺牲者）--

########## 场景 4：delete 带不带 limit 的锁范围 ##########
-- [无 limit] 记录锁 --
2108	c	X	GRANTED	10, 10
2108	c	X	GRANTED	10, 30
2108	c	X,GAP	GRANTED	15, 15
2108	PRIMARY	X,REC_NOT_GAP	GRANTED	10
2108	PRIMARY	X,REC_NOT_GAP	GRANTED	30
-- [无 limit] 记录锁行数 --
5
-- [limit 2] 记录锁 --
2159	c	X	GRANTED	10, 10
2159	c	X	GRANTED	10, 30
2159	PRIMARY	X,REC_NOT_GAP	GRANTED	10
2159	PRIMARY	X,REC_NOT_GAP	GRANTED	30
-- [limit 2] 记录锁行数 --
4

########## 场景 5：insert ... select 锁源表（RR，binlog_format=ROW）##########
-- A 在源表 t 上的锁（S = 共享 next-key lock，supremum 也锁）--
PRIMARY	S	0
PRIMARY	S	10
PRIMARY	S	15
PRIMARY	S	20
PRIMARY	S	25
PRIMARY	S	5
PRIMARY	S	supremum pseudo-record
-- B 往源表插 (9,9,9)（应为空：被 S next-key lock 挡住）--
-- A 提交后 B --
B-done

########## 场景 6：where 条件没索引的 update ##########
-- d 无索引，主键索引上逐行加锁 + supremum --
PRIMARY	X	0
PRIMARY	X	10
PRIMARY	X	15
PRIMARY	X	20
PRIMARY	X	25
PRIMARY	X	5
PRIMARY	X	supremum pseudo-record
-- 记录锁行数 --
7

########## 场景 7：两阶段锁——冲突语句放最后，别人的等待从 1.84s 降到 0.07s ##########
== A 先更新冲突行 id=5（冲突语句在前）==
-- [first] B 的 update id=5 从发出到返回：2.37s
== A 最后才更新冲突行 id=5（冲突语句在后）==
-- [last] B 的 update id=5 从发出到返回：0.28s

== 清理：容器 mysqllab-lock-33199 删除 ==
```

## 怎么读这些输出

| 观察 | 含义 |
|---|---|
| `X,GAP` + `LOCK_DATA=10, 10` | 索引 `c` 上 `(5,10)` 这个间隙锁；`LOCK_DATA` 取「右边界记录」 |
| `X` + `LOCK_DATA=5, 5` | 不是 `REC_NOT_GAP`，就是 next-key lock `(0,5]`（间隙 + 行锁合一） |
| `X,REC_NOT_GAP` | 只有行锁没有间隙；唯一索引等值命中的退化结果 |
| `X,GAP,INSERT_INTENTION` | 插入意向锁被间隙锁挡住——`insert` 阻塞的直接证据 |
| `S` + `WAITING` | 唯一键冲突时申请的共享 next-key lock 正在排队 |
| `ERROR 1213` | 死锁检测挑了一个牺牲者；本次是 B |
| `supremum pseudo-record` | 索引末尾那个假记录，防止在最大记录之后插入 |

## 浮动与合成说明

- 场景 7 的两个耗时是**等待时长**，随机器负载漂：首轮实测 `1.84s / 0.07s`，本文件贴的是另一次运行 `2.37s / 0.28s`。看**比值**（数十倍差距），不要看绝对值。
- 场景 3 里 `LOCK_DATA=5, 9` 的 `id=9` 来自自增值：失败与回滚的插入也会消耗 `auto_increment`，所以复跑时 id 会变。判断依据是 `c=5` 这条唯一键，不是 id。
- 事务 id（第一列）每次都不同；场景 4/5/6 的记录顺序按 `LOCK_DATA` 排序，稳定可比。
- 场景 3 的死锁牺牲者是「后到的一方」，B 或 C 皆有可能；本次报错在 B。
