-- 44.13 复制实验用到的探测语句（run.sh 逐条调用，也可手工黏贴）
-- 连接：mysql -h127.0.0.1 -P33199 -uroot --protocol=TCP

-- 1) 并行复制与依赖跟踪：默认值先看，再谈调优
SHOW VARIABLES LIKE 'replica_parallel%';
SHOW VARIABLES LIKE 'slave_parallel%';
SHOW VARIABLES LIKE 'binlog_transaction_dependency_tracking';

-- 2) 半同步模块：mysql:8.0 镜像默认不装，变量列表为空
SHOW VARIABLES LIKE 'rpl_semi_sync%';
INSTALL PLUGIN rpl_semi_sync_source SONAME 'semisync_source.so';
INSTALL PLUGIN rpl_semi_sync_replica SONAME 'semisync_replica.so';
SHOW VARIABLES LIKE 'rpl_semi_sync%';

-- 3) 位点与 GTID 集合：CHANGE MASTER TO 要填的就是这两列
SHOW MASTER STATUS;
SHOW BINARY LOGS;
SELECT @@gtid_mode, @@gtid_executed;

-- 4) 探测语句的代价：select 1 不进存储引擎
TRUNCATE performance_schema.events_statements_history;
SELECT 1 AS probe;
SELECT COUNT(*) FROM repl_lab.big;
SELECT SQL_TEXT, ROUND(TIMER_WAIT / 1000000000, 3) AS ms
FROM performance_schema.events_statements_history
WHERE SQL_TEXT IS NOT NULL
ORDER BY TIMER_START DESC LIMIT 3;

-- 5) IO 内部统计：判「真的慢」而不是「探针没超时」
SELECT EVENT_NAME, COUNT_STAR, ROUND(MAX_TIMER_WAIT / 1000000000, 3) AS max_ms
FROM performance_schema.file_summary_by_event_name
WHERE EVENT_NAME IN ('wait/io/file/innodb/innodb_log_file', 'wait/io/file/sql/binlog');

-- 6) 只读语义：read_only 挡普通用户、不挡 SUPER；super_read_only 连 SUPER 一起挡
SET GLOBAL read_only = ON;
-- 普通账号：ERROR 1290 (--read-only)
-- root(SUPER)：写入成功
SET GLOBAL super_read_only = ON;
-- root(SUPER)：ERROR 1290 (--super-read-only)
SET GLOBAL super_read_only = OFF;
SET GLOBAL read_only = OFF;
