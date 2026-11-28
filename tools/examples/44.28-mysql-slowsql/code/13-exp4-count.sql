-- 实验 4：count(*) / count(1) / count(id) / count(字段) 的真实差别（原文第 14 讲）
USE lab;

SELECT '--- 4.1 count(*)：优化器挑最小的那棵索引树（第一轮冷缓存）---' AS step;
EXPLAIN SELECT COUNT(*) FROM tradelog;
EXPLAIN ANALYZE SELECT COUNT(*) FROM tradelog;

SELECT '--- 4.1b count(*) 第二轮：缓存热了，耗时掉到个位数毫秒 ---' AS step;
EXPLAIN ANALYZE SELECT COUNT(*) FROM tradelog;

SELECT '--- 4.2 count(1)：不取值，按行累加 ---' AS step;
EXPLAIN ANALYZE SELECT COUNT(1) FROM tradelog;

SELECT '--- 4.3 count(id)：要把 id 取出来交给 server 层 ---' AS step;
EXPLAIN ANALYZE SELECT COUNT(id) FROM tradelog;

SELECT '--- 4.4 count(可空字段)：字段没索引，只能扫主键，还要判 NULL ---' AS step;
ALTER TABLE tradelog ADD COLUMN coupon_id INT NULL;
UPDATE tradelog SET coupon_id = IF(id % 10 = 0, NULL, id % 1000);
EXPLAIN ANALYZE SELECT COUNT(coupon_id) FROM tradelog;

SELECT '--- 4.4b 给这个字段建索引后：从主键全表扫描变成索引扫描 ---' AS step;
ALTER TABLE tradelog ADD INDEX idx_coupon (coupon_id);
ANALYZE TABLE tradelog;
EXPLAIN ANALYZE SELECT COUNT(coupon_id) FROM tradelog;

SELECT '--- 4.5 四个结果对照：count(*) 与 count(1) 都是 50000，count(字段) 少 5000 ---' AS step;
SELECT COUNT(*) AS c_star, COUNT(1) AS c_1, COUNT(id) AS c_id, COUNT(coupon_id) AS c_col FROM tradelog;

SELECT '--- 4.6 show table status 的 TABLE_ROWS 与小表真实行数对比 ---' AS step;
SELECT TABLE_NAME, TABLE_ROWS, ROUND(DATA_LENGTH / 1024 / 1024, 1) AS data_mb
FROM information_schema.TABLES WHERE TABLE_SCHEMA = 'lab';

ANALYZE TABLE tradelog;

SELECT '--- 4.7 ANALYZE 之后的 TABLE_ROWS ---' AS step;
SELECT TABLE_NAME, TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_SCHEMA = 'lab';
