-- 44.7 SQL 语义最小验证（MySQL 8.0.46）
-- 跑法见 README；输出应可逐行对照 expected.txt
DROP DATABASE IF EXISTS sql_semantics;
CREATE DATABASE sql_semantics;
USE sql_semantics;
SET NAMES utf8mb4;

CREATE TABLE t (
  id  INT PRIMARY KEY,
  grp VARCHAR(10) NULL,
  v   INT NULL
) ENGINE=InnoDB;

INSERT INTO t VALUES
  (1,'a',10),(2,'a',NULL),(3,'b',20),
  (4,'b',20),(5,NULL,30),(6,NULL,NULL);

SELECT 'Q1 COUNT 与 NULL' AS '';
SELECT COUNT(*) AS c_star, COUNT(v) AS c_v, COUNT(DISTINCT v) AS c_dv, AVG(v) AS avg_v FROM t;

SELECT 'Q2 GROUP BY 的 NULL 组' AS '';
SELECT grp, COUNT(*) AS n FROM t GROUP BY grp ORDER BY grp;

SELECT 'Q3 HAVING 引用 SELECT 别名' AS '';
SELECT grp, COUNT(*) AS num FROM t GROUP BY grp HAVING num > 1 ORDER BY num DESC;

SELECT 'Q4 LIKE 与 NULL' AS '';
SELECT 'abc' LIKE '%' AS abc_pct, '' LIKE '%' AS empty_pct, NULL LIKE '%' AS null_pct, 'a' LIKE '_' AS one_char;

SELECT 'Q5 CAST 与字符长度' AS '';
SELECT CAST(123.123 AS DECIMAL(8,2)) AS dec_cast,
       CAST(123.9 AS SIGNED) AS signed_cast,
       LENGTH('你好') AS bytes,
       CHAR_LENGTH('你好') AS chars;
