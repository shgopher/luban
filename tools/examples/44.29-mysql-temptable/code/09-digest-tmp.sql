-- 09 怎么定位"哪条 SQL 在建临时表"：performance_schema 按 digest 聚合
SELECT DIGEST_TEXT,
       COUNT_STAR AS exec_count,
       SUM_CREATED_TMP_TABLES AS tmp_tables,
       SUM_CREATED_TMP_DISK_TABLES AS disk_tables,
       SUM_SELECT_SCAN AS full_scans
FROM performance_schema.events_statements_summary_by_digest
WHERE SUM_CREATED_TMP_TABLES > 0
  AND SCHEMA_NAME = 'd'
ORDER BY SUM_CREATED_TMP_DISK_TABLES DESC, COUNT_STAR DESC
LIMIT 5;
