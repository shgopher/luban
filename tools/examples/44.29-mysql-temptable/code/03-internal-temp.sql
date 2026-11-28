-- 03 内部临时表：UNION、GROUP BY、order by null、SQL_BIG_RESULT、落盘判据
USE d;

SELECT '--- 03.1 UNION：第 3 行 UNION RESULT 带 Using temporary（临时表去重）' AS step;
EXPLAIN (SELECT 1000 AS f) UNION (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);
(SELECT 1000 AS f) UNION (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);

SELECT '--- 03.2 UNION ALL：没有 UNION RESULT 行，Extra 里没有 Using temporary' AS step;
EXPLAIN (SELECT 1000 AS f) UNION ALL (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);
(SELECT 1000 AS f) UNION ALL (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);

SELECT '--- 03.3 计数对照：UNION 建 1 个内部临时表，UNION ALL 建 0 个' AS step;
FLUSH STATUS;
(SELECT 1000 AS f) UNION (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);
SHOW SESSION STATUS LIKE 'Created_tmp%';
FLUSH STATUS;
(SELECT 1000 AS f) UNION ALL (SELECT id FROM t1 ORDER BY id DESC LIMIT 2);
SHOW SESSION STATUS LIKE 'Created_tmp%';

SELECT '--- 03.4 group by 无索引列：Using index; Using temporary（8.0 无 filesort）' AS step;
EXPLAIN SELECT id%10 AS m, count(*) AS c FROM t1 GROUP BY m;
SELECT id%10 AS m, count(*) AS c FROM t1 GROUP BY m;

SELECT '--- 03.5 order by null：跳过最后排序，结果按扫描顺序给出（0 落最后一行）' AS step;
EXPLAIN SELECT id%10 AS m, count(*) AS c FROM t1 GROUP BY m ORDER BY null;
SELECT id%10 AS m, count(*) AS c FROM t1 GROUP BY m ORDER BY null;

SELECT '--- 03.6 生成列 + 索引：Extra 只剩 Using index，临时表和文件排序都没了' AS step;
ALTER TABLE t1 ADD COLUMN z int GENERATED ALWAYS AS (id % 100), ADD INDEX(z);
EXPLAIN SELECT z, count(*) AS c FROM t1 GROUP BY z;

SELECT '--- 03.7 SQL_BIG_RESULT：不用临时表，直接排序数组（Using filesort）' AS step;
EXPLAIN SELECT SQL_BIG_RESULT id%100 AS m, count(*) AS c FROM t1 GROUP BY m;

SELECT '--- 03.8 distinct 与 group by：b 无索引，两条语句的 Extra 完全一样' AS step;
EXPLAIN SELECT DISTINCT b FROM t1;
EXPLAIN SELECT b FROM t1 GROUP BY b ORDER BY null;
