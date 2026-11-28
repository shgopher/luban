-- 实验 4：change buffer 什么时候才真正被用上
-- 关键条件：二级索引的工作集超过 buffer pool（本实验容器固定 128MB）
CREATE DATABASE IF NOT EXISTS idxlab;
USE idxlab;
SET SESSION cte_max_recursion_depth = 700000;

DROP TABLE IF EXISTS cb_wide;
CREATE TABLE cb_wide (
  id INT AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(200) NOT NULL,
  KEY idx_code (code)
) ENGINE=InnoDB;

-- 主键顺序插入（追加），二级索引键是随机字符串（落到叶子页的随机位置）
INSERT INTO cb_wide(code)
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 600000)
SELECT CONCAT(MD5(n), MD5(n+1), MD5(n+2), MD5(n+3), MD5(n+4), MD5(n+5)) FROM c;

ANALYZE TABLE cb_wide;
SELECT ROUND(data_length/1024/1024,1) AS data_mb,
       ROUND(index_length/1024/1024,1) AS index_mb
FROM information_schema.tables
WHERE table_schema = 'idxlab' AND table_name = 'cb_wide';
