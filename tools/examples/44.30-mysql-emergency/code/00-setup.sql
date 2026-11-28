-- 44.30 应急手册：实验数据准备（MySQL 8.0）
-- 表 t：20 万行小行，用于长事务、锁与巡检
-- 表 big：20 万行 × 300 字节，约 60MB 结果集，用于 Sending to client
SET SESSION sql_log_bin = 0;
CREATE DATABASE IF NOT EXISTS luban;

DROP TABLE IF EXISTS luban.t;
CREATE TABLE luban.t (
  id  INT PRIMARY KEY,
  c   INT NOT NULL DEFAULT 0,
  pad VARCHAR(64) NOT NULL DEFAULT ''
) ENGINE=InnoDB;

SET SESSION cte_max_recursion_depth = 1000000;
INSERT INTO luban.t (id, c, pad)
SELECT n, 0, 'x'
FROM (
  WITH RECURSIVE seq AS (
    SELECT 1 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 200000
  )
  SELECT n FROM seq
) s;

DROP TABLE IF EXISTS luban.big;
CREATE TABLE luban.big (
  id  INT PRIMARY KEY,
  pad VARCHAR(400) NOT NULL
) ENGINE=InnoDB;

INSERT INTO luban.big
SELECT n, REPEAT('x', 300)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 1 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 50000
  )
  SELECT n FROM seq
) s;

INSERT INTO luban.big
SELECT n, REPEAT('x', 300)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 50001 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 100000
  )
  SELECT n FROM seq
) s;

INSERT INTO luban.big
SELECT n, REPEAT('x', 300)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 100001 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 150000
  )
  SELECT n FROM seq
) s;

INSERT INTO luban.big
SELECT n, REPEAT('x', 300)
FROM (
  WITH RECURSIVE seq AS (
    SELECT 150001 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 200000
  )
  SELECT n FROM seq
) s;
