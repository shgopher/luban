# 44.8 示例：索引维护 / 覆盖索引与 ICP / ORDER BY / change buffer

对应节：[44.8 索引原理 (B+ 树 / 哈希 / 覆盖索引 / 最左前缀)](../../../v2/L4-组件与技术/44-数据库/44.8-索引原理.md)

一条命令复现正文里的四组数字：递增插入 vs 随机插入的叶子页数、覆盖索引与回表的 `EXPLAIN`、ICP 开关的扫描行数、`ORDER BY` 三种结局，以及 change buffer 在小表与大表上的两种相貌。

## 跑法

```bash
cd tools/examples/44.8-index-lab
bash code/run.sh                 # 默认端口 33199
PORT=34199 bash code/run.sh      # 33199 被占用时换一个
SKIP_CB=1 bash code/run.sh       # 跳过约 2 分钟的 change buffer 实验
```

前置：Docker 可用、能拉 `mysql:8.0`；宿主机有 `mysql` 客户端。脚本 `docker run --rm` 起容器 → 连续两次探活成功才继续 → 依次跑四组 SQL → `trap` 里 `docker rm -f` 清理，**跑完不留容器**。

| 文件 | 内容 |
|---|---|
| `code/01-page-split.sql` | 5 万行、`CHAR(200)`，递增插入与随机插入各一张表，读 `n_leaf_pages` |
| `code/02-covering-icp.sql` | 10 万行市民表，覆盖 vs 回表、ICP 开 vs 关的四条 `EXPLAIN (ANALYZE)` |
| `code/03-order-by.sql` | `(a,b)` 索引上四种 `ORDER BY` 写法 |
| `code/04-change-buffer.sql` | 60 万行、192 字符随机二级索引键，逼出 `Ibuf` 计数 |
| `code/run.sh` | 编排：起容器、探活、依次执行、打印 `SHOW ENGINE INNODB STATUS` |

## 环境

- 镜像 `mysql:8.0`，实测 **8.0.46**
- `innodb_page_size=16384`、`innodb_buffer_pool_size=128M`、默认 REPEATABLE-READ
- 宿主：macOS（Apple Silicon）+ Docker Desktop，容器与 mysqld 同机
- 下面是一次真实运行的粘贴（作者在端口 33106 上跑同一批 SQL；`run.sh` 默认 33199）。**change buffer 那组的 `Ibuf` 数值取决于缓冲池里还留着什么，每次运行会浮动**，量级一致即正常

## 期望输出（真实粘贴）

`EXPLAIN` 与 `SHOW ENGINE INNODB STATUS` 的原始回显很宽，下面按结论摘出相关列（丢掉表头分隔线与重复列），数值是原始回显里的值。

### 1. 环境

```
+---------+-----------+-------+-----------------+
| version | page_size | bp_mb | isolation       |
+---------+-----------+-------+-----------------+
| 8.0.46  |     16384 | 128.00000000 | REPEATABLE-READ |
+---------+-----------+-------+-----------------+
```

### 2. 插入顺序决定叶子页数

```
+------------+--------------+------------+
| table_name | stat_name    | stat_value |
+------------+--------------+------------+
| rand_t     | n_leaf_pages |       1062 |
| rand_t     | size         |       1249 |
| seq_t      | n_leaf_pages |        747 |
| seq_t      | size         |        801 |
+------------+--------------+------------+
+------------+------------+---------+
| table_name | table_rows | data_mb |
+------------+------------+---------+
| rand_t     |      49276 |    19.5 |
| seq_t      |      49302 |    12.5 |
+------------+------------+---------+
```

同样的 5 万行：递增插入 747 个叶子页、12.5MB；随机插入 1062 个叶子页、19.5MB（+42% 页、+56% 字节）。

`n_leaf_pages` 与 `data_length` 每次运行都稳定；`table_rows` 是采样估算，几次运行见过 44338 / 49276 / 50020 三个值，不影响结论。`EXPLAIN ANALYZE` 的毫秒数同样随缓存状态浮动（同一实例两次跑，覆盖那条从 8.13ms 到 2.71ms 都出现过），**看行数，不看毫秒**。

### 3. 覆盖索引与回表、ICP 开与关

```
-- 谓词相同，选择列表不同
| id | table | type  | key          | key_len | rows  | filtered | Extra                    |
|  1 | tuser | range | idx_name_age |     136 | 19272 |    10.00 | Using where; Using index |
|  1 | tuser | range | idx_name_age |     136 | 19272 |    10.00 | Using index condition    |

-- 覆盖：不回表
-> Filter: ((age = 10) and (name like '张%'))  (actual time=1.38..12.2 rows=2000)
    -> Covering index range scan on tuser using idx_name_age  (actual time=1.26..8.13 rows=10000)

-- 回表：ICP 先把 age 过滤掉
-> Index range scan on tuser using idx_name_age, with index condition: ((age = 10) and (name like '张%'))  (actual time=7.27..11.8 rows=2000)
```

```
-- ICP on：索引层读 2000 行
-> Filter: (ismale = 1)  (actual time=2.87..5.31 rows=1000)
    -> Index range scan ... with index condition  (actual time=2.87..5.16 rows=2000)

-- ICP off：索引层读 10000 行，回落成 Using where
-> Filter: ((ismale = 1) and (age = 10) and (name like '张%'))  (actual time=2.24..45.8 rows=1000)
    -> Index range scan on tuser using idx_name_age  (actual time=2.23..44.9 rows=10000)
```

### 4. ORDER BY 的四种写法

```
| type  | key    | rows | Extra                       |
| index | idx_ab |   10 | Using index                 |   -- ORDER BY a, b LIMIT 10
| ref   | idx_ab |   50 | Using index                 |   -- WHERE a = 3 ORDER BY b
| index | idx_ab | 5000 | Using index; Using filesort |   -- ORDER BY b
| range | idx_ab | 4800 | Using where; Using index; Using filesort |  -- WHERE a > 3 ORDER BY b
```

### 5. change buffer：小表不动，大表才动

```
-- 60 万行，主键顺序 + 二级索引键随机：数据 134.6MB、索引 202.0MB，缓冲池只有 128MB
+---------+----------+
| data_mb | index_mb |
+---------+----------+
|   134.6 |    202.0 |
+---------+----------+

-- SHOW ENGINE INNODB STATUS 的 change buffer 段
INSERT BUFFER AND ADAPTIVE HASH INDEX
-------------------------------------
Ibuf: size 305, free list len 236, seg size 542, 5329 merges
merged operations:
 insert 35192, delete mark 0, delete 0
```

同一实例上换成 12 万行、数据 25.5MB + 索引 39MB 的小表（缓冲池装得下）：批量插 2 万行后 `Ibuf` 仍是 `size 1, 0 merges`，普通索引物理读 +57 页、唯一索引 +67 页——**页常驻时两者没有差别，change buffer 也无事可做**。

## 失败与降级

- 起容器失败最常见的原因是端口被占：`ERROR 2003 (HY000): Can't connect to MySQL server on '127.0.0.1:33xxx'`。换 `PORT=34199` 再跑。
- 语言环境导致 `mysql` 客户端不在 `PATH`：脚本已尝试加载 `/Applications/Docker.app/Contents/Resources/bin`。
- 只用 `SELECT` / `INSERT`，不改全局参数；唯一改动的会话变量是 `optimizer_switch` 与 `cte_max_recursion_depth`，都在会话内生效、退出即失效。若在别的实例上手工执行，`SET GLOBAL innodb_change_buffering='none'` 记得改回 `all`。
