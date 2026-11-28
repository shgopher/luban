-- 04 内部临时表落盘：tmp_table_size / max_heap_table_size / temptable_max_ram 三个旋钮
USE d;

SELECT '--- 04.1 参数现状：8.0 默认内部临时表引擎是 TempTable，不是 MEMORY' AS step;
SELECT @@version AS version,
       @@internal_tmp_mem_storage_engine AS mem_engine,
       @@tmp_table_size AS tmp_table_size,
       @@max_heap_table_size AS max_heap_table_size,
       @@temptable_max_ram AS temptable_max_ram;

SELECT '--- 04.2 5.7 的 internal_tmp_disk_storage_engine 在 8.0.46 已被移除' AS step;
SELECT @@internal_tmp_disk_storage_engine;

SELECT '--- 04.3 默认 TempTable + tmp_table_size=1024：不落盘（tmp_table_size 不生效）' AS step;
SET SESSION tmp_table_size=1024;
SET SESSION max_heap_table_size=1024;
FLUSH STATUS;
SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null LIMIT 3;
SHOW SESSION STATUS LIKE 'Created_tmp%';

SELECT '--- 04.4 换成 MEMORY 内部临时表 + 1024：Created_tmp_disk_tables=1（落到磁盘临时表）' AS step;
SET SESSION internal_tmp_mem_storage_engine=MEMORY;
SET SESSION tmp_table_size=1024;
SET SESSION max_heap_table_size=1024;
FLUSH STATUS;
SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null LIMIT 3;
SHOW SESSION STATUS LIKE 'Created_tmp%';

SELECT '--- 04.5 MEMORY + 32M：5 万个分组放得下，Created_tmp_disk_tables 回到 0' AS step;
SET SESSION tmp_table_size=33554432;
SET SESSION max_heap_table_size=33554432;
FLUSH STATUS;
SELECT b, count(*) AS c FROM t_big GROUP BY b ORDER BY null LIMIT 3;
SHOW SESSION STATUS LIKE 'Created_tmp%';
