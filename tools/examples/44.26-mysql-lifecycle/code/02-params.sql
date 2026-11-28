-- 与"丢不丢数据"直接相关的参数：默认值就是 8.0 的出厂值
SELECT @@version AS version,
       @@transaction_isolation AS isolation,
       @@innodb_page_size AS page_size,
       @@innodb_buffer_pool_size/1024/1024 AS bp_mb;

SHOW VARIABLES WHERE Variable_name IN (
  'innodb_flush_log_at_trx_commit',
  'sync_binlog',
  'innodb_flush_neighbors',
  'innodb_io_capacity',
  'innodb_io_capacity_max',
  'innodb_max_dirty_pages_pct',
  'innodb_max_dirty_pages_pct_lwm',
  'innodb_lru_scan_depth',
  'innodb_log_buffer_size',
  'innodb_redo_log_capacity',
  'binlog_cache_size',
  'binlog_group_commit_sync_delay',
  'binlog_group_commit_sync_no_delay_count',
  'binlog_order_commits',
  'binlog_checksum',
  'binlog_format',
  'log_bin',
  'wait_timeout'
);
