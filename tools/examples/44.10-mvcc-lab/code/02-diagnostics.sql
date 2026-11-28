-- 长事务与 undo 的两个观测点
SELECT trx_id, trx_state, trx_started, trx_isolation_level,
       trx_rows_modified, trx_mysql_thread_id
FROM information_schema.innodb_trx;

SHOW ENGINE INNODB STATUS;
-- 只看 History list length 这一行
