-- 实验 2：覆盖索引（Using index）与索引下推（Using index condition）
CREATE DATABASE IF NOT EXISTS idxlab;
USE idxlab;
SET SESSION cte_max_recursion_depth = 200000;

DROP TABLE IF EXISTS tuser;
CREATE TABLE tuser (
  id INT PRIMARY KEY,
  name VARCHAR(32),
  age INT,
  ismale TINYINT(1),
  KEY idx_name_age (name, age)
) ENGINE=InnoDB;

-- 每 10 行一个「张」，age 在 0..49 循环，ismale 与 id 的十位相关
INSERT INTO tuser
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 100000)
SELECT n,
       CASE WHEN n % 10 = 0 THEN CONCAT('张', n) ELSE CONCAT('李', n) END,
       n % 50,
       (n DIV 10) % 2
FROM c;
ANALYZE TABLE tuser;

-- 谓词相同，只差选择列表：id 在索引里（覆盖），* 要回表
EXPLAIN SELECT id FROM tuser WHERE name LIKE '张%' AND age = 10;
EXPLAIN SELECT *  FROM tuser WHERE name LIKE '张%' AND age = 10;
EXPLAIN ANALYZE SELECT id FROM tuser WHERE name LIKE '张%' AND age = 10;
EXPLAIN ANALYZE SELECT *  FROM tuser WHERE name LIKE '张%' AND age = 10;

-- 同一条语句，ICP 开 / 关
EXPLAIN SELECT * FROM tuser WHERE name LIKE '张%' AND age = 10 AND ismale = 1;
EXPLAIN ANALYZE SELECT * FROM tuser WHERE name LIKE '张%' AND age = 10 AND ismale = 1;
SET optimizer_switch = 'index_condition_pushdown=off';
EXPLAIN SELECT * FROM tuser WHERE name LIKE '张%' AND age = 10 AND ismale = 1;
EXPLAIN ANALYZE SELECT * FROM tuser WHERE name LIKE '张%' AND age = 10 AND ismale = 1;
SET optimizer_switch = 'index_condition_pushdown=on';
