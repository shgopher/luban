# 44.27 示例：InnoDB 表空间、空间回收与在线 DDL

对应节：[44.27 InnoDB 表空间、空间回收与在线 DDL](../../../v2/L4-组件与技术/44-数据库/44.27-InnoDB表空间与在线DDL.md)

一条命令复现正文里的全部数字：`DELETE` 后文件不缩、`OPTIMIZE` 才缩、三档 DDL 的成功与失败、分区裁剪与 `DROP PARTITION`、复制表的四条路线、共享表空间只借不还。

## 跑法

```bash
cd tools/examples/44.27-mysql-space
bash code/run.sh                 # 默认端口 33199
PORT=34199 bash code/run.sh      # 33199 被占用时换一个
```

前置：Docker 可用、能拉 `mysql:8.0`；宿主机有 `mysql` 客户端（复制表那一步可选 `mysqldump`，没有就跳过该路线）。

脚本行为：`docker run --rm` 起一个 `mysql:8.0` 容器 → 等 TCP 就绪 → 依次跑 `code/01-space.sql`、`02-ddl.sql`、`03-partition.sql`、`04-copy.sql`、`05-fragment.sql` → 打印计时与文件字节数 → `trap` 里 `docker rm -f` 清理。**跑完不留容器**。

文件：

| 文件 | 内容 |
|---|---|
| `code/01-space.sql` | 1 万行表 → 删一半 → `OPTIMIZE` → `TRUNCATE`，每步看 `FILE_SIZE` |
| `code/02-ddl.sql` | `INSTANT` / `INPLACE` / `COPY` 五种成功、四种报错 |
| `code/03-partition.sql` | RANGE 分区、`EXPLAIN` 的 `partitions` 列、`DROP PARTITION`、分区主键 |
| `code/04-copy.sql` | 5 万行源表 + `CREATE TABLE LIKE` / `INSERT ... SELECT` |
| `code/05-fragment.sql` | 5 万行两次灌数，只差插入顺序，对比页数 |
| `code/run.sh` | 编排 + 计时 + `OUTFILE`/`LOAD DATA`、`mysqldump`、可传输表空间 |

## 环境

- 镜像 `mysql:8.0`，实测版本 **8.0.46**
- `innodb_page_size=16384`、`innodb_buffer_pool_size=128M`、默认 REPEATABLE-READ
- 宿主：macOS（Apple Silicon）+ Docker Desktop；容器与 mysqld 同机，无网络延迟
- 下面输出是**一次真实运行**（端口 34199，作者写稿时 33199 被占用），仅删掉重复的分隔线和部分 `ANALYZE` 回显

## 期望输出（真实粘贴）

```
== 1. 环境：版本 / 页大小 / 独立表空间 / online log 上限 ==
+---------+-----------+----------------+----------------+-------------+-----------------------+
| version | page_size | file_per_table | online_log_max | sort_buffer | secure_file_priv      |
+---------+-----------+----------------+----------------+-------------+-----------------------+
| 8.0.46  |     16384 |              1 |      134217728 |     1048576 | /var/lib/mysql-files/ |
+---------+-----------+----------------+----------------+-------------+-----------------------+
```

`innodb_online_alter_log_max_size` = 134217728 字节（128MB）是 row log 的上限；`innodb_sort_buffer_size` = 1MB 是建索引时的排序缓冲。

### 删一半：文件一个字节没少

```
| == 初始：10000 行 == |
+------------+---------+--------+---------+
| TABLE_ROWS | data_mb | idx_mb | free_mb |
+------------+---------+--------+---------+
|       9945 |    2.52 |   0.23 |    4.00 |
+------------+---------+--------+---------+
+------+-----------+-------+
| name | file_size | pages |          ← information_schema.INNODB_TABLESPACES
+------+-----------+-------+
| sp/t |  10485760 |   640 |
+------+-----------+-------+

| == DELETE 一半后 == |                  ← 删掉 5000 行
+-----------+
| rows_left |
+-----------+
|      5000 |
+-----------+
+------------+---------+---------+
| TABLE_ROWS | data_mb | free_mb |
+------------+---------+---------+
|       4896 |    2.52 |    4.00 |
+------------+---------+---------+
+------+-----------+-------+
| name | file_size | pages |
+------+-----------+-------+
| sp/t |  10485760 |   640 |            ← 640 页，字节数一个没变
+------+-----------+-------+

| == OPTIMIZE 之后 == |
+------------+---------+---------+
| TABLE_ROWS | data_mb | free_mb |
+------------+---------+---------+
|       4896 |    2.52 |    4.00 |
+------------+---------+---------+
+------+-----------+-------+
| name | file_size | pages |
+------+-----------+-------+
| sp/t |   7340032 |   448 |            ← -30%
+------+-----------+-------+

| == TRUNCATE 之后 == |
+------+-----------+-------+
| name | file_size | pages |
+------+-----------+-------+
| sp/t |    131072 |     8 |
+------+-----------+-------+
-- TRUNCATE 之后容器里 .ibd 的字节数 --
-rw-r----- 1 mysql mysql 131072 Nov 28 05:48 t.ibd
```

`OPTIMIZE TABLE` 的回显说明它在 InnoDB 上不是"原地整理"：

```
| sp.t | optimize | note | Table does not support optimize, doing recreate + analyze instead |
```

### 插入顺序决定页数

```
| == 5 万行、同样的数据，只差插入顺序 == |
+----------+-----------+-------+
| name     | file_size | pages |
+----------+-----------+-------+
| sp/rnd_t |  28311552 |  1728 |     ← ORDER BY RAND() 打乱后插入
| sp/seq_t |  20971520 |  1280 |     ← 主键递增插入
+----------+-----------+-------+
+------------+------------+---------+---------+
| TABLE_NAME | TABLE_ROWS | data_mb | free_mb |
+------------+------------+---------+---------+
| rnd_t      |      49743 |   19.52 |    4.00 |
| seq_t      |      49302 |   12.52 |    4.00 |
+------------+------------+---------+---------+
```

同样的行数、同样的行宽，随机插入多占 35% 的页（1728 vs 1280）——这部分就是页分裂留下的半空页。

### 在线 DDL：五个成功、五个报错

```
| == 5. 重建表收空间：INPLACE 边拷边记 row log == |
| sp/ddl_t |   9437184 |   576 |      ← 重建前
| sp/ddl_t |   7340032 |   448 |      ← 重建后

ERROR 1845 (0A000) at line 25: ALGORITHM=INSTANT is not supported for this operation. Try ALGORITHM=COPY/INPLACE.
ERROR 1221 (HY000) at line 29: Incorrect usage of ALGORITHM=INSTANT and LOCK=NONE/SHARED/EXCLUSIVE
ERROR 1846 (0A000) at line 44: ALGORITHM=INPLACE is not supported. Reason: Cannot change column type INPLACE. Try ALGORITHM=COPY.
ERROR 1846 (0A000) at line 49: LOCK=NONE is not supported. Reason: Fulltext index creation requires a lock. Try LOCK=SHARED.
ERROR 1846 (0A000) at line 57: ALGORITHM=INPLACE is not supported. Reason: InnoDB presently supports one FULLTEXT index creation at a time. Try ALGORITHM=COPY.
```

最后一条是附带现象：表上已有 `FULLTEXT` 索引后，再做 `ENGINE=InnoDB, ALGORITHM=INPLACE` 会被拒。`02-ddl.sql` 用 `mysql --force` 执行，报错不会中断后面的实验。

### 分区表：裁剪与删文件

```
| == 每个分区一个 .ibd，一个分区一张"引擎表" == |
+----------------+------------+
| PARTITION_NAME | TABLE_ROWS |
+----------------+------------+
| p2023          |        365 |
| p2024          |        366 |
| p2025          |        364 |
| pmax           |          0 |
+----------------+------------+

EXPLAIN ... WHERE ftime = '2024-04-01'                 → partitions: p2024
EXPLAIN ... WHERE ftime >= '2024-04-01'                → partitions: p2024,p2025,pmax
EXPLAIN ... WHERE ftime IN ('2023-06-01','2025-06-01') → partitions: p2023,p2025
EXPLAIN ... WHERE c = 5                                → partitions: p2023,p2024,p2025,pmax
EXPLAIN ... WHERE YEAR(ftime) = 2024                   → partitions: p2023,p2024,p2025,pmax   ← 不裁剪

| == DROP PARTITION：删文件，不写 undo/redo/binlog 行 == |
| rows_before | 1095 |   →   | rows_after | 730 |
-- 每个分区一个独立表空间文件 --
-rw-r----- 1 mysql mysql 131072 pt#p#p2024.ibd
-rw-r----- 1 mysql mysql 131072 pt#p#p2025.ibd
-rw-r----- 1 mysql mysql 131072 pt#p#pmax.ibd      ← p2023 的文件已消失
```

### 复制一张表：四条路线，5 万行 / 17MB

```
== 源表规模 ==
+-------------+----------+
| rows_in_src | total_mb |
+-------------+----------+
|       49725 |     9.03 |
+-------------+----------+

-- 路线一计时：CREATE TABLE LIKE + INSERT ... SELECT --
耗时 2715 ms，行数 50000
-- SELECT ... INTO OUTFILE + LOAD DATA（在容器内执行）--
导出耗时 882 ms
导入耗时 2626 ms，行数 50000
-- mysqldump | mysql --
耗时 3138 ms，行数 50000
-- 可传输表空间：cp .ibd，物理拷贝 --
耗时 2791 ms，行数 50000
-- import 不会删掉 .cfg，它会留在库目录里 --
-rw-r----- 1 mysql mysql      843 dst_ts.cfg
-rw-r----- 1 mysql mysql 17825792 dst_ts.ibd     ← 与 src.ibd 字节数完全相同
```

`rows_in_src` 是 `information_schema` 的估算值（49725），实际 50000 行；`total_mb` 9.03 是"存数据的页"，`.ibd` 实际 17MB，差额是空闲的区与页。

### 共享表空间：删表也不缩

```
-rw-r----- 1 mysql mysql 12582912 /var/lib/mysql/ibdata1     ← 空实例
-- 灌进 10 万行后 --
-rw-r----- 1 mysql mysql 79691776 /var/lib/mysql/ibdata1     ← 12MB → 76MB
-- DROP TABLE 之后 --
-rw-r----- 1 mysql mysql 79691776 /var/lib/mysql/ibdata1     ← 76MB，一个字节没退
```

## 数字怎么浮动

| 数字 | 稳定性 | 说明 |
|---|---|---|
| `file_size` 10485760 / 7340032 / 131072 | **稳定** | 页面组织由数据量和主键顺序决定，同一份数据重复跑结果一致 |
| 三个 `DATA_*` 列（2.52 / 4.00 MB） | 稳定 | 取整到 0.01MB，`ANALYZE` 之后基本不变 |
| 四条复制路线的耗时 | **会漂 2–20 倍** | 同一台机器上三轮：`LIKE` 1.7s / 2.7s / 41.6s，`mysqldump` 3.1s / 8.2s / 49.9s，`LOAD DATA` 2.6s / 4.8s / 70.1s。第三轮是宿主机上同时跑着 7 个 MySQL 容器做 IO 密集实验时测的。**只看相对次序，绝对数别抄** |
| `rows_in_src` | 会漂 | `information_schema` 的行数是采样估算，±2% 正常 |
| `file_size` 20971520 / 28311552 | **稳定** | 独立两次运行（不同端口、不同宿主机负载）字节数完全一致 |
| `ERROR 1845/1221/1846` 的报错文本 | 稳定 | 8.0.46 实测；版本不同措辞可能微调，错误码一致 |
| `YEAR(ftime)=2024` 不裁剪 | 8.0.46 实测结论 | 换版本前用 `EXPLAIN` 复核一次 |

## 清理

`run.sh` 结束会自动 `docker rm -f`。手工确认：

```bash
docker ps -a --format '{{.Names}}' | grep mysqlex-4427 || echo "clean"
```
