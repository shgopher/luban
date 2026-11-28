# 44.7 join 实验：NLJ / hash join、left join 改写、MRR 与 BKA

对应节：[44.7 SQL 与查询优化](../../../v2/L4-组件与技术/44-数据库/44.7-SQL与查询优化.md) 的「join 能不能用」「join 的账单」「join 优化四板斧」「left join」四节。

## 跑法

前置：Docker（`mysql:8.0` 镜像）、`mysql` 客户端。默认端口 **33199**，可用 `PORT=xxxx` 覆盖；容器 `--rm`，脚本退出时 `docker rm -f`。

```bash
cd tools/examples/44.7-join-lab
bash code/run.sh
```

脚本依次执行：

| 文件 | 内容 |
|---|---|
| `code/00-setup.sql` | 建 `t1` 10,000 行、`t2` 100,000 行、`t3`（`a` 逆序）、`la`/`lb` 各 6 行，并 `ANALYZE TABLE` |
| `code/10-join-plans.sql` | NLJ / hash join / `straight_join` 对调 / 关 join buffer 的 `EXPLAIN` |
| `code/20-left-join.sql` | `LEFT JOIN` 的 `ON` vs `WHERE`：行数、驱动表、`SHOW WARNINGS` |
| `code/30-mrr-bka.sql` | MRR 开关、BKA 开关、覆盖索引不触发 BKA |
| `code/40-timings.sql` | 各计划与 `join_buffer_size` 的 3 次耗时（`SHOW PROFILES`） |

## 环境

```text
version=8.0.46 arch=aarch64 page=16384 bp=128.00000000M isolation=REPEATABLE-READ
```

Linux/arm64 容器内 MySQL 8.0.46，`innodb_page_size=16384`，`innodb_buffer_pool_size=128M`，RR。**8.0.46 没有复现出 `Block Nested Loop`**：无索引 join 走 `Using join buffer (hash join)`，`hash_join=off` 也没有改回 BNL，只有 `block_nested_loop=off` 会退回 Simple NLJ。5.7 的 BNL 数字来自课程原文，本文只在本机复现 8.0.46 的行为。

## 期望输出（一次真实运行，节选）

NLJ：被驱动表 `t2.a` 有索引，`type=ref`：

```text
|  1 | SIMPLE | t1 | NULL | index | a | a | 5 | NULL         | 10337 | 100.00 | Using where; Using index |
|  1 | SIMPLE | t2 | NULL | ref   | a | a | 5 | joinlab.t1.a |     1 | 100.00 | Using index              |
```

无索引：被驱动表 `type=ALL`，`Extra` 是 hash join：

```text
|  1 | SIMPLE | t1 | NULL | index | a    | a    | 5    | NULL | 10337 | 100.00 | Using index                                |
|  1 | SIMPLE | t2 | NULL | ALL   | NULL | NULL | NULL | NULL | 97739 |  10.00 | Using where; Using join buffer (hash join) |
```

`straight_join` 反过来：`t2` 变成第一行（`rows=100000` 量级），`t1` 变成 `ref`。耗时从 `t1` 驱动的 **15.2–26.9ms** 涨到 `t2` 驱动的 **196–303ms**（同一容器 3 次）。

`LEFT JOIN` 的行数与驱动表：

```text
q1_rows = 6            -- 条件都在 ON
q2_rows = 4            -- a.f2=b.f2 写在 WHERE

Q2 EXPLAIN:
|  1 | SIMPLE | lb | NULL | ALL | NULL | NULL | NULL | NULL          | 6 | 100.00 | Using where |
|  1 | SIMPLE | la | NULL | ref | f1   | f1   | 5    | joinlab.lb.f1 | 1 |  16.67 | Using where |

Q2 SHOW WARNINGS:
/* select#1 */ select ... from `joinlab`.`la` join `joinlab`.`lb`
where ((`joinlab`.`la`.`f1` = `joinlab`.`lb`.`f1`) and (`joinlab`.`la`.`f2` = `joinlab`.`lb`.`f2`))
```

MRR / BKA：

```text
mrr_cost_based=on  -> Extra: Using index condition
mrr_cost_based=off -> Extra: Using index condition; Using MRR

BKA off, select t2.b -> Extra: NULL
BKA on,  select t2.b -> Extra: Using join buffer (Batched Key Access)
BKA on,  select count(*) -> Extra: Using index   -- 覆盖索引不回表，BKA 不触发
```

## 耗时速查（同一容器，3 次，单位 ms）

| 计划 | 耗时 |
|---|---|
| NLJ，`t1`(10k) 驱动 `t2`(100k) | 15.2–26.9 |
| hash join，`t2.b` 无索引 | 44.9–63.2 |
| 大表驱动：`t2`(100k) 驱动 `t1`(10k) | 196–303 |
| Simple NLJ，200 × 100k（`block_nested_loop=off`） | 3830–7250 |
| hash join，200 × 100k | 34.8–38.8 |
| NLJ，200 × 100k | 0.9–6.7（首次含冷缓存） |
| hash join，`join_buffer_size=128K` | 34.3–81.2 |
| hash join，`join_buffer_size=4M` | 20.5–21.2 |

## 浮动说明

- 耗时是容器内 `SHOW PROFILES` 的真实值，但同一脚本不同时段可差 **2–3 倍**（宿主机负载、buffer pool 冷热）；只比量级，不比绝对值。
- `rows` 是优化器估算值（`ANALYZE TABLE` 后 `t1` 约 10337、`t2` 约 97739），不是精确行数。
- `00-setup.sql` 末尾必须 `ANALYZE TABLE`：不执行时 `la`/`lb` 只有 6 行，Q2 可能被估成 hash join；执行后才稳定复现「`lb` 驱动、`la type=ref`」的 NLJ。
- `hash_join=off` 在 8.0.46 实测没有把计划改回 BNL；不要拿 5.7 的 `Extra` 字样套 8.0.46。
