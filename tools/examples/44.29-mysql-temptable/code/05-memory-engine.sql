-- 05 Memory 引擎：堆组织表、hash 索引、varchar 定长、上限、表锁
USE d;

DROP TABLE IF EXISTS t_mem, t_inn, t_mem20, t_mem200;
CREATE TABLE t_mem(id int primary key, c int) ENGINE=Memory;
CREATE TABLE t_inn(id int primary key, c int) ENGINE=InnoDB;
INSERT INTO t_mem VALUES (1,1),(2,2),(3,3),(4,4),(5,5),(6,6),(7,7),(8,8),(9,9),(0,0);
INSERT INTO t_inn VALUES (1,1),(2,2),(3,3),(4,4),(5,5),(6,6),(7,7),(8,8),(9,9),(0,0);

SELECT '--- 05.1 同样的 10 行：Memory 的 0 在最后（数组顺序），InnoDB 的 0 在第一（主键有序）' AS step;
SELECT * FROM t_mem;
SELECT * FROM t_inn;

SELECT '--- 05.2 删 id=5 再插 id=10：10 占用了 5 留下的空位' AS step;
DELETE FROM t_mem WHERE id=5;
INSERT INTO t_mem VALUES (10,10);
SELECT * FROM t_mem;

SELECT '--- 05.3 建表语句与存储引擎' AS step;
SHOW CREATE TABLE t_mem;

SELECT '--- 05.4 hash 主键做范围查询：type=ALL 全表扫描' AS step;
EXPLAIN SELECT * FROM t_mem WHERE id<5;
SELECT * FROM t_mem WHERE id<5;

SELECT '--- 05.5 加 B-Tree 索引后：type=range，结果 0..4 有序' AS step;
ALTER TABLE t_mem ADD INDEX a_btree_index USING BTREE (id);
EXPLAIN SELECT * FROM t_mem WHERE id<5;
SELECT * FROM t_mem WHERE id<5;
SELECT '--- 05.5b force index(primary) 强行走 hash 索引：0 又回到最后' AS step;
SELECT * FROM t_mem FORCE INDEX(primary) WHERE id<5;

SELECT '--- 05.6 varchar(20)/varchar(200) 都存 2 字节字符串：Memory 按定长存，DATA_LENGTH 差 8 倍' AS step;
CREATE TABLE t_mem20(id int primary key, s varchar(20)) ENGINE=Memory;
CREATE TABLE t_mem200(id int primary key, s varchar(200)) ENGINE=Memory;
SET SESSION cte_max_recursion_depth=200000;
INSERT INTO t_mem20 WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<5000) SELECT n,'ab' FROM seq;
INSERT INTO t_mem200 WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<5000) SELECT n,'ab' FROM seq;
SELECT table_name, engine, table_rows, data_length, index_length,
       round(data_length/5000,1) AS bytes_per_row
FROM information_schema.tables
WHERE table_schema='d' AND table_name IN ('t_mem20','t_mem200') ORDER BY table_name;

SELECT '--- 05.7 Memory 表不支持 BLOB/TEXT' AS step;
CREATE TABLE t_mem_text(id int primary key, body text) ENGINE=Memory;

SELECT '--- 05.8 max_heap_table_size=1M 时写满：ERROR 1114 The table is full' AS step;
SET SESSION max_heap_table_size=1048576;
CREATE TABLE t_mem_full(id int primary key, s char(200)) ENGINE=Memory;
INSERT INTO t_mem_full WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<5000) SELECT n, repeat('x',200) FROM seq;

SELECT '--- 05.9 Memory 表的主键自增：自增值不回退，删行后空位由数据复用' AS step;
CREATE TABLE t_mem_ai(id int auto_increment primary key, c int) ENGINE=Memory;
INSERT INTO t_mem_ai(c) VALUES (1),(2),(3);
DELETE FROM t_mem_ai WHERE id=2;
INSERT INTO t_mem_ai(c) VALUES (4);
SELECT * FROM t_mem_ai;
SHOW CREATE TABLE t_mem_ai;

SELECT '--- 05.10 重启前记下 Memory/InnoDB 两表行数' AS step;
INSERT INTO t_mem VALUES (999,999);
SELECT (SELECT count(*) FROM t_mem) AS mem_rows_before,
       (SELECT count(*) FROM t_inn) AS inn_rows_before;
