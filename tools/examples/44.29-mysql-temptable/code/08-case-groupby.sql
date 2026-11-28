-- 08 案例：一条 10 万行的 GROUP BY，从"临时表 + 1133ms"改成"索引 + 97ms"
USE d;

SELECT '--- 08.1 改造前：b 无索引，EXPLAIN ANALYZE 的 actual time' AS step;
EXPLAIN ANALYZE SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null;

SELECT '--- 08.2 只加一个索引' AS step;
ALTER TABLE t_big ADD INDEX idx_b(b);

SELECT '--- 08.3 改造后：Extra 只剩 Using index，临时表消失' AS step;
EXPLAIN SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null;
EXPLAIN ANALYZE SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null;

SELECT '--- 08.4 换个算法：SQL_BIG_RESULT 走排序数组，也不用临时表' AS step;
EXPLAIN ANALYZE SELECT SQL_BIG_RESULT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null;

SELECT '--- 08.5 顺带验证：tmp_table_size 管不到用户建的 Memory 表（只受 max_heap_table_size 限制）' AS step;
SET SESSION tmp_table_size=1024;
SET SESSION max_heap_table_size=16777216;
CREATE TABLE t_mem_cap(id int primary key, s char(200)) ENGINE=Memory;
INSERT INTO t_mem_cap SELECT id, repeat('x',200) FROM t1;
SELECT count(*) AS rows_inserted,
       (SELECT data_length FROM information_schema.tables
        WHERE table_schema='d' AND table_name='t_mem_cap') AS data_length;
