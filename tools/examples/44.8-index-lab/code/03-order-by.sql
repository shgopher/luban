-- 实验 3：ORDER BY 什么时候能用上索引
CREATE DATABASE IF NOT EXISTS idxlab;
USE idxlab;
SET SESSION cte_max_recursion_depth = 20000;

DROP TABLE IF EXISTS t_order;
CREATE TABLE t_order (
  id INT AUTO_INCREMENT PRIMARY KEY,
  a INT NOT NULL,
  b INT NOT NULL,
  KEY idx_ab (a, b)
) ENGINE=InnoDB;

INSERT INTO t_order(a, b)
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 5000)
SELECT n % 100, n % 7 FROM c;
ANALYZE TABLE t_order;

-- 全表按 (a,b) 顺序扫：不回表、不排序
EXPLAIN SELECT a, b FROM t_order ORDER BY a, b LIMIT 10;
-- a 是常量，b 在剩下的记录里有序
EXPLAIN SELECT a, b FROM t_order WHERE a = 3 ORDER BY b LIMIT 10;
-- 只按 b 排：索引里 b 只在 a 相同的前提下有序 → filesort
EXPLAIN SELECT a, b FROM t_order ORDER BY b LIMIT 10;
-- a 是范围：范围之后的 b 不再有序 → filesort
EXPLAIN SELECT a, b FROM t_order WHERE a > 3 ORDER BY b LIMIT 10;
