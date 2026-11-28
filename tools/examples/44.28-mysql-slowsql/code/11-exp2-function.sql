-- 实验 2：函数操作破坏索引有序性（原文案例一）
-- month(t_modified)=7 让 B+ 树在第一层就无从下手：同一个 7 月散落在三年里
USE lab;

SELECT '--- 2.1 month(t_modified)=7：type=index，全索引扫描 ---' AS step;
EXPLAIN SELECT COUNT(*) FROM tradelog WHERE MONTH(t_modified) = 7;
EXPLAIN ANALYZE SELECT COUNT(*) FROM tradelog WHERE MONTH(t_modified) = 7;
SHOW WARNINGS;

SELECT '--- 2.2 改写成按字段本身的范围查询：type=range ---' AS step;
EXPLAIN SELECT COUNT(*) FROM tradelog
 WHERE (t_modified >= '2016-07-01' AND t_modified < '2016-08-01')
    OR (t_modified >= '2017-07-01' AND t_modified < '2017-08-01')
    OR (t_modified >= '2018-07-01' AND t_modified < '2018-08-01');
EXPLAIN ANALYZE SELECT COUNT(*) FROM tradelog
 WHERE (t_modified >= '2016-07-01' AND t_modified < '2016-08-01')
    OR (t_modified >= '2017-07-01' AND t_modified < '2017-08-01')
    OR (t_modified >= '2018-07-01' AND t_modified < '2018-08-01');

SELECT '--- 2.3 两种写法结果必须一致（这是改写的前提）---' AS step;
SELECT (SELECT COUNT(*) FROM tradelog WHERE MONTH(t_modified) = 7) AS by_func,
       (SELECT COUNT(*) FROM tradelog
         WHERE (t_modified >= '2016-07-01' AND t_modified < '2016-08-01')
            OR (t_modified >= '2017-07-01' AND t_modified < '2017-08-01')
            OR (t_modified >= '2018-07-01' AND t_modified < '2018-08-01')) AS by_range;

-- 2.4 不改变有序性的函数也不走索引：id+1=10000 要手写成 id=10000-1
SELECT '--- 2.4 加 1 不改变有序性，优化器仍然不用索引 ---' AS step;
EXPLAIN SELECT * FROM tradelog WHERE id + 1 = 10001;
EXPLAIN SELECT * FROM tradelog WHERE id = 10000;
