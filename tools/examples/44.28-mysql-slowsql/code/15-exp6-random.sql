-- 实验 6：随机取 3 条，三种写法（原文第 17 讲）
USE lab;

SELECT '--- 6.1 ORDER BY RAND()：Using temporary + Using filesort ---' AS step;
EXPLAIN SELECT word FROM words ORDER BY RAND() LIMIT 3;
EXPLAIN ANALYZE SELECT word FROM words ORDER BY RAND() LIMIT 3;

SELECT '--- 6.2 随机算法 1：max/min 定位，扫 1 行（有空洞时概率不均）---' AS step;
SELECT COUNT(*) AS c, MIN(id) AS lo, MAX(id) AS hi,
       MAX(id) - MIN(id) + 1 AS span,
       MAX(id) - MIN(id) + 1 - COUNT(*) AS holes
FROM words;

SELECT '--- 6.3 随机算法 1 的实际语句 ---' AS step;
SELECT MAX(id), MIN(id) INTO @M, @N FROM words;
SET @X = FLOOR((@M - @N + 1) * RAND() + @N);
SELECT @M AS M, @N AS N, @X AS X;
EXPLAIN ANALYZE SELECT id, word FROM words WHERE id >= @X LIMIT 1;

SELECT '--- 6.4 随机算法 2：先 count(*) 再 limit Y,1（严格均匀，但扫 C+Y+1 行）---' AS step;
SELECT COUNT(*) INTO @C FROM words;
SET @Y = FLOOR(@C * RAND());
SET @sql = CONCAT('SELECT id, word FROM words LIMIT ', @Y, ',1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT '--- 6.5 算法 3 的扫描行数上界：C + (Y1+1) + (Y2+1) + (Y3+1) ---' AS step;
SELECT @C AS total_rows;
