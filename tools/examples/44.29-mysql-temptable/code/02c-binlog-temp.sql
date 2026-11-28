-- 02c 临时表与 binlog：ROW 格式一条都不记，STATEMENT 格式才记
USE d;

SELECT '--- 02c.1 当前格式' AS step;
SELECT @@binlog_format AS binlog_format;

SELECT '--- 02c.2 ROW 下建/写/删临时表（这些操作不会出现在 binlog 里）' AS step;
CREATE TEMPORARY TABLE t_binlog_probe(id int primary key, c int) ENGINE=InnoDB;
INSERT INTO t_binlog_probe VALUES (1,1);
DROP TEMPORARY TABLE t_binlog_probe;

SELECT '--- 02c.3 切到 STATEMENT 后重做一遍，并留下一张普通表做对照' AS step;
SET GLOBAL binlog_format=STATEMENT;
SET SESSION binlog_format=STATEMENT;
FLUSH BINARY LOGS;
CREATE TABLE t_normal_binlog(id int primary key, c int);
CREATE TEMPORARY TABLE temp_t LIKE t_normal_binlog;
INSERT INTO temp_t VALUES (1,1);
INSERT INTO t_normal_binlog SELECT * FROM temp_t;
DROP TEMPORARY TABLE temp_t;
DROP TABLE t_normal_binlog;
SET SESSION binlog_format=ROW;
SET GLOBAL binlog_format=ROW;
SELECT '切回 ROW' AS step;
