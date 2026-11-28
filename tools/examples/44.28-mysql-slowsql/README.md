# 44.28 MySQL 慢 SQL 案例集：可复现实例

对应正文：[44.28 MySQL 慢 SQL 案例集：同一个逻辑，八种慢法](../../../v2/L4-组件与技术/44-数据库/44.28-MySQL慢SQL案例集.md)

一条命令起库、灌数、跑完八个案例的全部 SQL，结束自动删容器。

## 跑法

```bash
cd tools/examples/44.28-mysql-slowsql
bash code/run.sh                 # 默认端口 33199
PORT=33200 bash code/run.sh      # 端口被占时换一个
```

前置：

| 项 | 要求 |
|---|---|
| Docker | 本机可用（macOS 上脚本会把 `/Applications/Docker.app/Contents/Resources/bin` 加进 `PATH`） |
| mysql 客户端 | 只用来连容器；不要求本机跑 MySQL 服务 |
| 端口 | 默认 `33199`，被占用时用 `PORT=` 覆盖 |
| 耗时 | 约 3~4 分钟（灌数 4 秒，案例 7 的两个等待实验各 10 秒） |

`run.sh` 的四个阶段：起容器（`--rm`）→ 等就绪（最多 90 秒，失败会打印 `docker logs` 并 `exit 1`）→ 灌 4 张表各 5 万行 → 依次跑案例 1~6 的 `*.sql` 与案例 7 的三连接锁脚本 → trap 里 `docker rm -f` 清理。

## 环境

```text
+---------+-----------------+-----------+--------------+
| version | isolation       | page_size | bp_mb        |
+---------+-----------------+-----------+--------------+
| 8.0.46  | REPEATABLE-READ |     16384 | 128.00000000 |
+---------+-----------------+-----------+--------------+
```

- 镜像 `mysql:8.0`，实测拉到 8.0.46
- 隔离级别 `REPEATABLE-READ`，`innodb_page_size=16384`（16KB），buffer pool 128M（起容器时显式指定）
- 数据：`tradelog`（5 万行，`tradeid` 为 `varchar`、`t_modified` 铺满 2016~2018）、`idcard`（5 万行，前 6 位只有 20 个县）、`tcity`（5 万行，杭州占 1 万行）、`words`（5 万行）

## 目录

```text
code/
├── run.sh                       # 一条命令：起库 → 灌数 → 跑全部实验 → 清理
├── 00-schema.sql                # 建表 + 灌数（4 张表 × 5 万行）
├── 10-exp1-implicit-cast.sql    # 案例六 6.2：隐式类型转换
├── 11-exp2-function.sql         # 案例六 6.1：函数操作 vs 范围查询
├── 12-exp3-prefix-index.sql     # 案例二：前缀区分度、覆盖索引、ANALYZE
├── 13-exp4-count.sql            # 案例三：四种 count 写法 + TABLE_ROWS
├── 14-exp5-orderby.sql          # 案例四：filesort → 免排序 → 覆盖索引
├── 15-exp6-random.sql           # 案例五：ORDER BY RAND() vs 随机 id
└── 16-exp7-lock.sh              # 案例七：三个并发连接造 MDL/行锁等待
```

## 期望输出（真实粘贴）

### 隐式类型转换：22.5ms vs 0.338ms

```text
| --- 1.1 传数字：type=ALL，全表扫描 --- |
|  1 | SIMPLE | tradelog | NULL | ALL  | idx_tradeid | NULL        | NULL | NULL  | 49075 | 10.00 | Using where |
| -> Filter: (tradelog.tradeid = 110717)  (cost=4948 rows=4908) (actual time=1.36..22.5 rows=1 loops=1)
    -> Table scan on tradelog  (cost=4948 rows=49075) (actual time=1.24..18.2 rows=50000 loops=1)

| Warning | 1739 | Cannot use ref access on index 'idx_tradeid' due to type or collation conversion on field 'tradeid'   |
| Warning | 1739 | Cannot use range access on index 'idx_tradeid' due to type or collation conversion on field 'tradeid' |

| --- 1.2 传字符串：走索引，只扫 1 行 --- |
|  1 | SIMPLE | tradelog | NULL | ref  | idx_tradeid | idx_tradeid |  130 | const |     1 | 100.00 | NULL  |
| -> Index lookup on tradelog using idx_tradeid (tradeid='110717')  (cost=0.35 rows=1) (actual time=0.336..0.338 rows=1 loops=1)
```

### 函数操作：19.2ms vs 1.77ms，结果都是 4,320 行

```text
| --- 2.1 month(t_modified)=7：type=index，全索引扫描 --- |
|  1 | SIMPLE | tradelog | NULL | index | NULL          | idx_tmodified | 5 | NULL | 49075 | 100.00 | Using where; Using index |
| -> Aggregate: count(0)  (cost=9855 rows=1) (actual time=19.2..19.2 rows=1 loops=1)
    -> Filter: (month(tradelog.t_modified) = 7)  (cost=4948 rows=49075) (actual time=9.9..18.9 rows=4320 loops=1)
        -> Covering index scan on tradelog using idx_tmodified  (cost=4948 rows=49075) (actual time=7.17..15.8 rows=50000 loops=1)

| --- 2.2 改写成按字段本身的范围查询：type=range --- |
|  1 | SIMPLE | tradelog | NULL | range | idx_tmodified | idx_tmodified | 5 | NULL | 4320 | 100.00 | Using where; Using index |
| -> Aggregate: count(0)  (cost=1297 rows=1) (actual time=1.77..1.77 rows=1 loops=1)

| by_func | by_range |
|    4320 |     4320 |
```

### 前缀索引：区分度 0.04%，回表行数估成 2,625

```text
| L     | L4 | L5 | L6 | L7 | L8 | L10 | L12 | L14  | L17   |
| 50000 |  2 | 14 | 20 | 20 | 20 |  40 | 480 | 1460 | 50000 |

| r6     | r8     | r10    | r12    | r14    | r17    |
| 0.0004 | 0.0004 | 0.0008 | 0.0096 | 0.0292 | 1.0000 |

| --- 3.3 同一个等值查询：全字段索引 vs 前缀索引，看 rows --- |
|  1 | ... | ref  | idx_card  | 74 | const |    1 | 100.00 | NULL        |
|  1 | ... | ref  | idx_card6 | 26 | const | 2625 | 100.00 | Using where |

| --- 3.4 覆盖索引：全字段索引能 Using index，前缀索引永远不行 --- |
|  1 | ... | ref  | idx_card  | 74 | const |    1 | 100.00 | Using index |
|  1 | ... | ref  | idx_card6 | 26 | const | 2625 | 100.00 | Using where |
```

`ANALYZE` 前后的对比（新回填的哈希列，统计信息还停留在“整列 NULL”）：

```text
| --- 3.7 刚回填完的新列：rows 估成整表（49880）--- |
|  1 | ... | ref | idx_crc | 5 | const | 49880 |  0.00 | Using where |
| lab.idcard | analyze | status | OK |
| --- 3.8 ANALYZE 之后：rows 回到 1 --- |
|  1 | ... | ref | idx_crc | 5 | const |     1 |  5.00 | Using where |
```

### count：8.8ms / 42.2ms / 14.1ms

```text
| --- 4.1 count(*)：优化器挑最小的那棵索引树（第一轮冷缓存）--- |
|  1 | SIMPLE | tradelog | NULL | index | NULL | idx_tmodified | 5 | NULL | 49717 | 100.00 | Using index |
| -> Count rows in tradelog  (actual time=8.84..8.84 rows=1 loops=1)

| --- 4.2 count(1)：不取值，按行累加 --- |
| -> Count rows in tradelog  (actual time=7.52..7.52 rows=1 loops=1)

| --- 4.4 count(可空字段)：字段没索引，只能扫主键，还要判 NULL --- |
| -> Aggregate: count(tradelog.coupon_id)  (actual time=42.2..42.2 rows=1 loops=1)
    -> Table scan on tradelog  (actual time=0.559..37.3 rows=50000 loops=1)

| --- 4.4b 给这个字段建索引后：从主键全表扫描变成索引扫描 --- |
| -> Aggregate: count(tradelog.coupon_id)  (actual time=14.1..14.1 rows=1 loops=1)
    -> Covering index scan on tradelog using idx_coupon  (actual time=0.0868..9.86 rows=50000 loops=1)

| c_star | c_1   | c_id  | c_col |
|  50000 | 50000 | 50000 | 45000 |
```

### order by：198ms → 21.4ms → 1.8ms

```text
| --- 5.1 只有 city 索引：Extra 出现 Using filesort --- |
|  1 | SIMPLE | tcity | NULL | ref | idx_city | 66 | const | 18488 | 100.00 | Using filesort |
| -> Limit: 1000 row(s)  (actual time=197..198 rows=1000 loops=1)
    -> Sort: tcity.`name`, limit input to 1000 row(s) per chunk  (actual time=197..198 rows=1000 loops=1)

| --- 5.4 加 (city,name) 联合索引：Using filesort 消失 --- |
|  1 | SIMPLE | tcity | NULL | ref | idx_city_name | 66 | const | 18488 | 100.00 | NULL |
| -> Limit: 1000 row(s)  (actual time=18.3..21.4 rows=1000 loops=1)

| --- 5.5 加 (city,name,age) 联合索引：多出 Using index，彻底不回表 --- |
|  1 | SIMPLE | tcity | NULL | ref | idx_city_name_age | 66 | const | 19272 | 100.00 | Using index |
| -> Limit: 1000 row(s)  (actual time=0.694..1.8 rows=1000 loops=1)
```

落盘排序的 `filesort_summary`（`sort_buffer_size=32768`、杭州 1 万行）：

```text
[{"key_size": 264, "row_size": 402, "sort_mode": "<varlen_sort_key, packed_additional_fields>",
  "num_rows_found": 10000, "sort_algorithm": "std::sort", "memory_available": 32768,
  "peak_memory_used": 40328, "num_rows_estimate": 18488, "max_rows_per_buffer": 81,
  "num_initial_chunks_spilled_to_disk": 16}]
```

`max_length_for_sort_data` 在 8.0.46 已废弃：

```text
| Warning | 1287 | '@@max_length_for_sort_data' is deprecated and will be removed in a future release. |
```

### 随机：217ms vs 0.33ms

```text
| --- 6.1 ORDER BY RAND()：Using temporary + Using filesort --- |
|  1 | SIMPLE | words | NULL | ALL | NULL | NULL | NULL | NULL | 46856 | 100.00 | Using temporary; Using filesort |
| -> Limit: 3 row(s)  (actual time=217..217 rows=3 loops=1)
    -> Sort: `rand()`, limit input to 3 row(s) per chunk  (actual time=217..217 rows=3 loops=1)
        -> Table scan on words  (actual time=7.87..142 rows=50000 loops=1)

| --- 6.3 随机算法 1 的实际语句 --- |
| M     | N     | X     |
| 50000 |     1 | 11240 |
| -> Limit: 1 row(s)  (actual time=0.331..0.331 rows=1 loops=1)
    -> Index range scan on words using PRIMARY over (11240 <= id)  (actual time=0.282..0.282 rows=1 loops=1)
```

### 等锁：MDL 与行锁各 10 秒现场

```text
=== 7.1 等 MDL 写锁：DDL/lock tables 把 select 堵在表级 ===
| id | command | time | state                           | info                       |
| 35 | Query   |    1 | Waiting for table metadata lock | SELECT COUNT(*) FROM words |
| waiting_pid | blocking_pid | object_name | waiting_query              |
|          35 |           34 | words       | SELECT COUNT(*) FROM words |
ERROR 1205 (HY000): Lock wait timeout exceeded; try restarting transaction

=== 7.2 等行锁：update 不提交，select ... lock in share mode 被堵 ===
| waiting_pid | blocking_pid | waiting_query                                     | blocking_query |
|          39 |           38 | SELECT * FROM words WHERE id=1 LOCK IN SHARE MODE | DO SLEEP(10)   |
ERROR 1205 (HY000): Lock wait timeout exceeded; try restarting transaction
```

## 正文数字与本文件的对应

| 正文结论 | 本目录哪段输出 |
|---|---|
| 隐式转换 22.5ms vs 0.338ms、Warning 1739 | `10-exp1-implicit-cast.sql` |
| `month()` 19.2ms vs 范围 1.77ms，同为 4,320 行 | `11-exp2-function.sql` |
| 前缀 6 位区分度 0.0004、`rows` 2,625 vs 1、覆盖索引消失 | `12-exp3-prefix-index.sql` |
| `ANALYZE` 前后 `rows` 49,880 → 1 | `12-exp3-prefix-index.sql`（3.7/3.8） |
| `count(*)` 8.8ms、`count(字段)` 42.2ms → 建索引 14.1ms | `13-exp4-count.sql` |
| `TABLE_ROWS` 49,075 / 49,634 vs 真实 50,000 | `13-exp4-count.sql`（4.6/4.7） |
| order by 198 → 21.4 → 1.8ms；落盘 16 块 | `14-exp5-orderby.sql` |
| `max_length_for_sort_data` 只有 Warning 1287 | `14-exp5-orderby.sql`（5.3） |
| `ORDER BY RAND()` 217ms vs 随机 id 0.33ms | `15-exp6-random.sql` |
| 等 MDL / 等行锁现场 + 1205 超时 | `16-exp7-lock.sh` |

## 浮动说明

- **绝对耗时随机器负载浮动 3~10 倍**：同一套脚本在空闲机器上 `count(*)` 是 8.8ms，在同一台机器跑满多个容器时实测 109ms；`month()` 从 19.2ms 涨到 31.4ms。**判据看 `type`/`key`/`rows`/`Extra`，不看毫秒。**
- **比值比绝对值稳**：隐式转换“全表扫 vs 索引定位”的量级差在不同负载下都在 60 倍以上；order by “filesort vs 覆盖索引”在三档之间始终是百倍量级。
- **`rows` 会小幅漂**：`ANALYZE` 采样、灌数顺序都会让它差几十行（本文件里出现过 49,075 / 49,634 / 50,337），属正常。
- **案例 7 的睡眠时长可调**：脚本里 `DO SLEEP(10)` 与 `lock_wait_timeout=8` 是给慢机器留的余量；机器很快时可以把两者一起调小。
- **案例 8（undo 版本链）未在本目录复现**：需要百万次 update + 长事务，容器 I/O 撑不住，正文数字标注为原文数据。
- **5.2 的中间结果被省略**：该步 `LIMIT 100` 会打印 100 行杭州数据，上面只保留了 `filesort_summary` 那一段。
- **`internal_tmp_disk_storage_engine`**：本机是 8.0 默认值，脚本没有改它；正文里“4001 vs 4000”的差异来自原文 5.7 环境。
