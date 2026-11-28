-- 实验 1：插入顺序决定叶子页数（44.8「主键怎么选」）
CREATE DATABASE IF NOT EXISTS idxlab;
USE idxlab;
SET SESSION cte_max_recursion_depth = 100000;

DROP TABLE IF EXISTS seq_t, rand_t, nums;
CREATE TABLE nums  (n INT PRIMARY KEY) ENGINE=InnoDB;
CREATE TABLE seq_t (id INT PRIMARY KEY, pad CHAR(200) NOT NULL DEFAULT '') ENGINE=InnoDB;
CREATE TABLE rand_t(id INT PRIMARY KEY, pad CHAR(200) NOT NULL DEFAULT '') ENGINE=InnoDB;

INSERT INTO nums
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 50000)
SELECT n FROM c;

-- 递增插入：每次都是追加
INSERT INTO seq_t SELECT n, REPEAT('x',200) FROM nums ORDER BY n;
-- 随机顺序插入：每行落到树的随机位置
INSERT INTO rand_t SELECT n, REPEAT('x',200) FROM nums ORDER BY RAND(42);

ANALYZE TABLE seq_t, rand_t;

SELECT table_name, stat_name, stat_value
FROM mysql.innodb_index_stats
WHERE database_name = 'idxlab' AND table_name IN ('seq_t','rand_t')
  AND index_name = 'PRIMARY' AND stat_name IN ('n_leaf_pages','size')
ORDER BY table_name, stat_name;

SELECT table_name, table_rows, ROUND(data_length/1024/1024,1) AS data_mb
FROM information_schema.tables
WHERE table_schema = 'idxlab' AND table_name IN ('seq_t','rand_t');
