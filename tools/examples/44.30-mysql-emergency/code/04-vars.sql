-- 本节涉及的关键参数（本实验环境实际值；max_connections 由启动参数设为 200）
SELECT VARIABLE_NAME, VARIABLE_VALUE
FROM performance_schema.global_variables
WHERE VARIABLE_NAME IN (
  'net_buffer_length', 'max_connections', 'wait_timeout',
  'innodb_thread_concurrency', 'innodb_buffer_pool_size',
  'innodb_old_blocks_time', 'sort_buffer_size', 'join_buffer_size',
  'read_buffer_size', 'tmp_table_size'
)
ORDER BY VARIABLE_NAME;
