-- 06 重启之后：Memory 表数据清空、表结构还在；binlog 里多了一条"清空内存表"
USE d;

SELECT '--- 06.1 重启后：Memory 表 0 行，表结构仍是 ENGINE=MEMORY' AS step;
SELECT count(*) AS mem_rows_after FROM t_mem;
SHOW CREATE TABLE t_mem;

SELECT '--- 06.2 重启后：InnoDB 表一行不少' AS step;
SELECT count(*) AS inn_rows_after FROM t_inn;

SELECT '--- 06.3 重启时服务端自己往 binlog 写了一条清空语句（双 M 结构会传到主库）' AS step;
SHOW MASTER STATUS;
