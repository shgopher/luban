-- 02-ddl.sql：在线 DDL 三档算法（INSTANT / INPLACE / COPY）的成功与失败
-- 用 mysql --force 跑，让预期失败的语句不影响后面的实验
USE sp;

DROP TABLE IF EXISTS ddl_t;
CREATE TABLE ddl_t (
  id  INT PRIMARY KEY,
  a   INT,
  pad CHAR(200)
) ENGINE = InnoDB;
SET SESSION cte_max_recursion_depth = 20000;
INSERT INTO ddl_t
SELECT n, n % 1000, REPEAT('y', 200)
FROM (WITH RECURSIVE s AS (SELECT 1 AS n UNION ALL SELECT n + 1 FROM s WHERE n < 5000) SELECT n FROM s) x;

SELECT '== 1. INSTANT 加列：只改数据字典，不碰数据页 ==' AS step;
ALTER TABLE ddl_t ADD COLUMN c1 INT, ALGORITHM = INSTANT;
-- 8.0.29 起支持加在任意位置；8.0.12~8.0.28 只能加在最后一列
ALTER TABLE ddl_t ADD COLUMN c3 INT AFTER a, ALGORITHM = INSTANT;
ALTER TABLE ddl_t ADD COLUMN c2 VARCHAR(10) DEFAULT 'x', ALGORITHM = INSTANT;
SHOW CREATE TABLE ddl_t;

SELECT '== 2. INSTANT 加索引：不支持，只能 INPLACE ==' AS step;
-- 预期失败：ERROR 1845 (0A000): ALGORITHM=INSTANT is not supported for this operation.
ALTER TABLE ddl_t ADD INDEX idx_c1 (c1), ALGORITHM = INSTANT;

SELECT '== 3. INSTANT 不能带 LOCK=NONE ==' AS step;
-- 预期失败：ERROR 1221 (HY000): Incorrect usage of ALGORITHM=INSTANT and LOCK=NONE/SHARED/EXCLUSIVE
ALTER TABLE ddl_t ADD COLUMN c4 INT, ALGORITHM = INSTANT, LOCK = NONE;

SELECT '== 4. INPLACE + LOCK=NONE 加索引：不锁 DML ==' AS step;
ALTER TABLE ddl_t ADD INDEX idx_a (a), ALGORITHM = INPLACE, LOCK = NONE;

SELECT '== 5. 重建表收空间：INPLACE 边拷边记 row log ==' AS step;
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces WHERE name = 'sp/ddl_t';
ALTER TABLE ddl_t ENGINE = InnoDB, ALGORITHM = INPLACE, LOCK = NONE;
SELECT name, file_size, ROUND(file_size / 16384) AS pages
FROM information_schema.innodb_tablespaces WHERE name = 'sp/ddl_t';

SELECT '== 6. 改字符集：只能 COPY（建临时表逐行拷，锁写） ==' AS step;
-- 预期失败：ERROR 1846 (0A000): ALGORITHM=INPLACE is not supported.
--           Reason: Cannot change column type INPLACE. Try ALGORITHM=COPY.
ALTER TABLE ddl_t MODIFY pad VARCHAR(200) CHARACTER SET gbk, ALGORITHM = INPLACE;

SELECT '== 7. 全文索引：inplace 但非 online，LOCK=NONE 被拒 ==' AS step;
-- 预期失败：ERROR 1846 (0A000): LOCK=NONE is not supported.
--           Reason: Fulltext index creation requires a lock. Try LOCK=SHARED.
ALTER TABLE ddl_t ADD FULLTEXT KEY ft_pad (pad), ALGORITHM = INPLACE, LOCK = NONE;
-- 改成 LOCK=SHARED 能执行，但执行期间表只允许读（这就是"inplace 但不 online"）
ALTER TABLE ddl_t ADD FULLTEXT KEY ft_pad (pad), ALGORITHM = INPLACE, LOCK = SHARED;

-- 附带现象：表上有了 FULLTEXT 索引之后，再做 ENGINE=InnoDB 的 INPLACE 重建会被拒
SELECT '== 8. 有全文索引后 INPLACE 重建被拒（8.0.46 实测） ==' AS step;
-- 预期失败：ERROR 1846 (0A000): ALGORITHM=INPLACE is not supported.
--           Reason: InnoDB presently supports one FULLTEXT index creation at a time. Try ALGORITHM=COPY.
ALTER TABLE ddl_t ENGINE = InnoDB, ALGORITHM = INPLACE, LOCK = NONE;
