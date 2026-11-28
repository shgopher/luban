-- 05-fragment.sql：同样的数据、同样的行数，插入顺序决定页数
-- 主键递增插入 → 顺序写、无页分裂；ORDER BY RAND() 插入 → 随机写、页分裂
SET SESSION cte_max_recursion_depth = 100000;
USE sp;

DROP TABLE IF EXISTS seq_t;
DROP TABLE IF EXISTS rnd_t;
CREATE TABLE seq_t (id INT PRIMARY KEY, pad CHAR(200)) ENGINE = InnoDB;
CREATE TABLE rnd_t (id INT PRIMARY KEY, pad CHAR(200)) ENGINE = InnoDB;

INSERT INTO seq_t
SELECT n, REPEAT('x', 200)
FROM (WITH RECURSIVE s AS (SELECT 1 AS n UNION ALL SELECT n + 1 FROM s WHERE n < 50000) SELECT n FROM s) x;

INSERT INTO rnd_t
SELECT n, REPEAT('x', 200)
FROM (WITH RECURSIVE s AS (SELECT 1 AS n UNION ALL SELECT n + 1 FROM s WHERE n < 50000) SELECT n FROM s) x
ORDER BY RAND();

ANALYZE TABLE seq_t, rnd_t;

SELECT '== 5 万行、同样的数据，只差插入顺序 ==' AS step;
SELECT name,
       file_size,
       ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces
WHERE name IN ('sp/seq_t', 'sp/rnd_t')
ORDER BY name;

SELECT table_name,
       table_rows,
       ROUND(data_length / 1024 / 1024, 2) AS data_mb,
       ROUND(data_free / 1024 / 1024, 2)   AS free_mb
FROM information_schema.tables
WHERE table_schema = 'sp' AND table_name IN ('seq_t', 'rnd_t')
ORDER BY table_name;
