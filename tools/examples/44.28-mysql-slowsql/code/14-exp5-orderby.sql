-- 实验 5：order by 的三级跳：filesort → 消除排序 → 覆盖索引（原文第 16 讲）
USE lab;

ALTER TABLE tcity ADD INDEX idx_city (city);

SELECT '--- 5.1 只有 city 索引：Extra 出现 Using filesort ---' AS step;
EXPLAIN SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;
EXPLAIN ANALYZE SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;

SELECT '--- 5.2 sort_buffer 放不下：排序数据溢出到磁盘（临时文件）---' AS step;
SET SESSION sort_buffer_size = 32768;
SET SESSION optimizer_trace = 'enabled=on';
SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 100;
SELECT JSON_EXTRACT(trace, '$**.filesort_summary') AS filesort_summary
FROM information_schema.OPTIMIZER_TRACE;
SET SESSION optimizer_trace = 'enabled=off';

SELECT '--- 5.3 8.0.46：max_length_for_sort_data 已废弃，设了也不切 rowid 排序 ---' AS step;
SET SESSION max_length_for_sort_data = 16;
SHOW WARNINGS;
SET SESSION sort_buffer_size = 262144;
SET SESSION optimizer_trace = 'enabled=on';
SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 100;
SELECT JSON_EXTRACT(trace, '$**.filesort_summary') AS with_deprecated_param
FROM information_schema.OPTIMIZER_TRACE;
SET SESSION optimizer_trace = 'enabled=off';

SELECT '--- 5.4 加 (city,name) 联合索引：Using filesort 消失 ---' AS step;
ALTER TABLE tcity ADD INDEX idx_city_name (city, name);
EXPLAIN SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;
EXPLAIN ANALYZE SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;

SELECT '--- 5.5 加 (city,name,age) 联合索引：多出 Using index，彻底不回表 ---' AS step;
ALTER TABLE tcity ADD INDEX idx_city_name_age (city, name, age);
EXPLAIN SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;
EXPLAIN ANALYZE SELECT city, name, age FROM tcity WHERE city = '杭州' ORDER BY name LIMIT 1000;

SELECT '--- 5.6 limit 20000：要排序的行数变大，代价立刻暴露 ---' AS step;
EXPLAIN ANALYZE SELECT city, name, age FROM tcity FORCE INDEX (idx_city)
 WHERE city = '杭州' ORDER BY name LIMIT 20000;
