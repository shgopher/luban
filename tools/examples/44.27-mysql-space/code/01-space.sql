-- 01-space.sql：DELETE 之后文件为什么不缩，OPTIMIZE 缩在哪里
-- 环境：MySQL 8.0.46，innodb_page_size=16384，innodb_buffer_pool_size=128M
SET SESSION cte_max_recursion_depth = 20000;

CREATE DATABASE IF NOT EXISTS sp;
USE sp;

DROP TABLE IF EXISTS t;
CREATE TABLE t (
  id  INT PRIMARY KEY,
  a   INT,
  pad CHAR(200),          -- 撑大每行，让页数和文件大小看得见
  KEY (a)
) ENGINE = InnoDB;

-- 10000 行，按主键递增插入 → 索引紧凑，初始没有页分裂空洞
INSERT INTO t
SELECT n, n % 1000, REPEAT('x', 200)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 1 AS n UNION ALL SELECT n + 1 FROM seq WHERE n < 10000
  ) SELECT n FROM seq
) s;
ANALYZE TABLE t;

SELECT '== 初始：10000 行 ==' AS step;
SELECT table_rows,
       ROUND(data_length / 1024 / 1024, 2) AS data_mb,
       ROUND(index_length / 1024 / 1024, 2) AS idx_mb,
       ROUND(data_free / 1024 / 1024, 2) AS free_mb
FROM information_schema.tables
WHERE table_schema = 'sp' AND table_name = 't';
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces
WHERE name = 'sp/t';

-- 删掉一半：只把记录标记为可复用，页和文件都不动
DELETE FROM t WHERE id % 2 = 0;
ANALYZE TABLE t;

SELECT '== DELETE 一半后 ==' AS step;
SELECT COUNT(*) AS rows_left FROM t;
SELECT table_rows,
       ROUND(data_length / 1024 / 1024, 2) AS data_mb,
       ROUND(data_free / 1024 / 1024, 2) AS free_mb
FROM information_schema.tables
WHERE table_schema = 'sp' AND table_name = 't';
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces
WHERE name = 'sp/t';

-- 重建表：OPTIMIZE TABLE 收到 InnoDB 上就是 recreate + analyze
OPTIMIZE TABLE t;

SELECT '== OPTIMIZE 之后 ==' AS step;
SELECT table_rows,
       ROUND(data_length / 1024 / 1024, 2) AS data_mb,
       ROUND(data_free / 1024 / 1024, 2) AS free_mb
FROM information_schema.tables
WHERE table_schema = 'sp' AND table_name = 't';
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces
WHERE name = 'sp/t';

-- TRUNCATE = DROP + CREATE：整文件回收，代价是几毫秒级的元数据操作
SELECT '== TRUNCATE 之后 ==' AS step;
TRUNCATE TABLE t;
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces
WHERE name = 'sp/t';
