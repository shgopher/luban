-- 04-copy.sql：复制一张表 —— 逻辑方式（同实例，CREATE TABLE LIKE + INSERT ... SELECT）
-- 另外三条路线（OUTFILE/LOAD DATA、mysqldump、可传输表空间）在 run.sh 里跑并计时
SET SESSION cte_max_recursion_depth = 200000;
USE sp;

DROP TABLE IF EXISTS src;
CREATE TABLE src (
  id  INT PRIMARY KEY,
  a   INT,
  pad CHAR(100),
  KEY (a)
) ENGINE = InnoDB;

-- 5 万行，约 17MB 表空间：够看出四条路线的量级差
INSERT INTO src
SELECT n, n % 1000, REPEAT('y', 100)
FROM (WITH RECURSIVE s AS (SELECT 1 AS n UNION ALL SELECT n + 1 FROM s WHERE n < 50000) SELECT n FROM s) x;
ANALYZE TABLE src;

SELECT '== 源表规模 ==' AS step;
SELECT table_rows AS rows_in_src,
       ROUND((data_length + index_length) / 1024 / 1024, 2) AS total_mb
FROM information_schema.tables
WHERE table_schema = 'sp' AND table_name = 'src';

SELECT '== 路线一：CREATE TABLE LIKE + INSERT ... SELECT（结构+数据，同实例最快） ==' AS step;
DROP TABLE IF EXISTS dst_like;
CREATE TABLE dst_like LIKE src;
INSERT INTO dst_like SELECT * FROM src;
SELECT COUNT(*) AS rows_in_dst_like FROM dst_like;
