-- 值班巡检：一条查询给出「连接 / 执行 / 阻塞 / 事务」四个维度（MySQL 8.0）
SELECT
  (SELECT COUNT(*) FROM information_schema.processlist)                                   AS threads_connected,
  (SELECT @@max_connections)                                                              AS max_connections,
  (SELECT COUNT(*) FROM information_schema.processlist WHERE command = 'Query')            AS threads_running,
  (SELECT COUNT(*) FROM information_schema.processlist WHERE state = 'Sending to client')  AS sending_to_client,
  (SELECT COUNT(*) FROM information_schema.innodb_trx)                                     AS open_trx,
  (SELECT COUNT(*) FROM information_schema.innodb_trx WHERE trx_rows_modified > 10000)     AS big_trx,
  (SELECT COUNT(*) FROM performance_schema.data_locks WHERE lock_status = 'WAITING')       AS lock_waiters;
