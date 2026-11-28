-- 打开表输出的慢日志（只为实验，生产别把 long_query_time 设 0）
SET GLOBAL log_output = 'TABLE';
SET GLOBAL slow_query_log = ON;
SET GLOBAL long_query_time = 0;
USE lifecycle;
