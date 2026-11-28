-- 实验 3：前缀索引到底该取多长（原文第 11 讲）
-- 身份证号前 6 位是地址码，本例只有 20 个县 → 前缀 6 位的区分度极差
USE lab;

SELECT '--- 3.1 全列基数与各前缀长度的基数（区分度 = Ln / L）---' AS step;
SELECT COUNT(DISTINCT id_card) AS L,
       COUNT(DISTINCT LEFT(id_card, 4))  AS L4,
       COUNT(DISTINCT LEFT(id_card, 5))  AS L5,
       COUNT(DISTINCT LEFT(id_card, 6))  AS L6,
       COUNT(DISTINCT LEFT(id_card, 7))  AS L7,
       COUNT(DISTINCT LEFT(id_card, 8))  AS L8,
       COUNT(DISTINCT LEFT(id_card, 10)) AS L10,
       COUNT(DISTINCT LEFT(id_card, 12)) AS L12,
       COUNT(DISTINCT LEFT(id_card, 14)) AS L14,
       COUNT(DISTINCT LEFT(id_card, 17)) AS L17
FROM idcard;

SELECT '--- 3.2 区分度比值：低于 95% 就是"损失超过 5%" ---' AS step;
SELECT ROUND(COUNT(DISTINCT LEFT(id_card, 6))  / COUNT(DISTINCT id_card), 4) AS r6,
       ROUND(COUNT(DISTINCT LEFT(id_card, 8))  / COUNT(DISTINCT id_card), 4) AS r8,
       ROUND(COUNT(DISTINCT LEFT(id_card, 10)) / COUNT(DISTINCT id_card), 4) AS r10,
       ROUND(COUNT(DISTINCT LEFT(id_card, 12)) / COUNT(DISTINCT id_card), 4) AS r12,
       ROUND(COUNT(DISTINCT LEFT(id_card, 14)) / COUNT(DISTINCT id_card), 4) AS r14,
       ROUND(COUNT(DISTINCT LEFT(id_card, 17)) / COUNT(DISTINCT id_card), 4) AS r17
FROM idcard;

-- 3.3 建 email(6) 式的前缀索引，对比回表次数
ALTER TABLE idcard ADD INDEX idx_card6 (id_card(6));

SELECT '--- 3.3 同一个等值查询：全字段索引 vs 前缀索引，看 rows ---' AS step;
EXPLAIN SELECT * FROM idcard FORCE INDEX (idx_card)  WHERE id_card = '330100197108042000';
EXPLAIN SELECT * FROM idcard FORCE INDEX (idx_card6) WHERE id_card = '330100197108042000';

SELECT '--- 3.4 覆盖索引：全字段索引能 Using index，前缀索引永远不行 ---' AS step;
EXPLAIN SELECT id, id_card FROM idcard FORCE INDEX (idx_card)  WHERE id_card = '330100197108042000';
EXPLAIN SELECT id, id_card FROM idcard FORCE INDEX (idx_card6) WHERE id_card = '330100197108042000';

-- 3.5 倒序存储：把地址码挪到末尾，前缀就变成顺序号
SELECT '--- 3.5 REVERSE 后的前缀区分度（后 6 位没有地址码的重复性）---' AS step;
SELECT ROUND(COUNT(DISTINCT LEFT(REVERSE(id_card), 6)) / COUNT(DISTINCT id_card), 4) AS rev_r6,
       ROUND(COUNT(DISTINCT LEFT(REVERSE(id_card), 4)) / COUNT(DISTINCT id_card), 4) AS rev_r4
FROM idcard;

-- 3.6 哈希列：4 字节索引，等值查询平均扫 1 行
ALTER TABLE idcard ADD COLUMN id_card_crc INT UNSIGNED, ADD INDEX idx_crc (id_card_crc);
UPDATE idcard SET id_card_crc = CRC32(id_card);

SELECT '--- 3.6 哈希列：rows=1，但要带上原值判断防冲突 ---' AS step;
EXPLAIN SELECT * FROM idcard
 WHERE id_card_crc = CRC32('330100197108042000') AND id_card = '330100197108042000';
SELECT COUNT(*) AS crc_rows FROM idcard WHERE id_card_crc = CRC32('330100197108042000');

-- 3.7 刚回填的新列，统计信息按「整列 NULL」算：rows 估成整表，索引等于白建
SELECT '--- 3.7 刚回填完的新列：rows 估成整表（49888）---' AS step;
EXPLAIN SELECT * FROM idcard FORCE INDEX (idx_crc)
 WHERE id_card_crc = CRC32('330100197108042000') AND id_card = '330100197108042000';

ANALYZE TABLE idcard;

SELECT '--- 3.8 ANALYZE 之后：rows 回到 1 ---' AS step;
EXPLAIN SELECT * FROM idcard FORCE INDEX (idx_crc)
 WHERE id_card_crc = CRC32('330100197108042000') AND id_card = '330100197108042000';
