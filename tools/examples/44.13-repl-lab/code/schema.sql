-- 44.13 复制实验：单实例能观测到的复制面
-- 建库 + 50,000 行宽表（binlog 量的实验对象，行数固定，便于复现）

DROP DATABASE IF EXISTS repl_lab;
CREATE DATABASE repl_lab;
USE repl_lab;

CREATE TABLE big (
  id  INT PRIMARY KEY,
  c   INT NOT NULL,
  pad CHAR(60) NOT NULL DEFAULT 'x'
) ENGINE=InnoDB;

SET SESSION cte_max_recursion_depth = 100000;
INSERT INTO big
WITH RECURSIVE seq AS (
  SELECT 1 AS n
  UNION ALL
  SELECT n + 1 FROM seq WHERE n < 50000
)
SELECT n, n, 'x' FROM seq;

SELECT COUNT(*) AS rows_built FROM big;
