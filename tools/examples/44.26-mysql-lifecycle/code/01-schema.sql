-- 建库建表：一张窄表用来做 update 的 binlog 观察，一张宽表用来制造脏页
DROP DATABASE IF EXISTS lifecycle;
CREATE DATABASE lifecycle;
USE lifecycle;

CREATE TABLE t (
  id INT PRIMARY KEY,
  c  INT
) ENGINE=InnoDB;

-- 目标行：id=2, c=0。update 之后 binlog 里会看到 @2=0 -> @2=1
INSERT INTO t VALUES (1,0),(2,0);

CREATE TABLE big (
  id  INT PRIMARY KEY AUTO_INCREMENT,
  c   INT,
  pad CHAR(200)
) ENGINE=InnoDB;

SET SESSION cte_max_recursion_depth = 100000;
INSERT INTO big (c, pad)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n < 20000
)
SELECT n, REPEAT('x',200) FROM seq;
