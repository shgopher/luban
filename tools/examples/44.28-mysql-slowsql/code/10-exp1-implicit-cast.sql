-- 实验 1：隐式类型转换把索引打掉（原文案例二）
-- tradeid 是 varchar(32)，传数字 110717 时 MySQL 会把「字段」转成数字比较：
--   CAST(tradeid AS signed int) = 110717
-- 等于在索引列上套了函数 → 放弃树搜索 → 全表扫描
USE lab;

SELECT '--- 1.1 传数字：type=ALL，全表扫描 ---' AS step;
EXPLAIN SELECT * FROM tradelog WHERE tradeid = 110717;
EXPLAIN ANALYZE SELECT * FROM tradelog WHERE tradeid = 110717;
SHOW WARNINGS;

SELECT '--- 1.2 传字符串：走索引，只扫 1 行 ---' AS step;
EXPLAIN SELECT * FROM tradelog WHERE tradeid = '110717';
EXPLAIN ANALYZE SELECT * FROM tradelog WHERE tradeid = '110717';

SELECT '--- 1.3 反过来不成立：int 列传字符串，常量被转成数字，索引照走 ---' AS step;
EXPLAIN SELECT * FROM tradelog WHERE id = '700';
