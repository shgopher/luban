-- 03-partition.sql：分区表的两层视角、分区裁剪、DROP PARTITION
-- 环境：MySQL 8.0.46
SET SESSION cte_max_recursion_depth = 5000;
USE sp;

DROP TABLE IF EXISTS pt;
CREATE TABLE pt (
  ftime DATETIME NOT NULL,
  c     INT,
  KEY (ftime)
) ENGINE = InnoDB
PARTITION BY RANGE (YEAR(ftime)) (
  PARTITION p2023 VALUES LESS THAN (2024),
  PARTITION p2024 VALUES LESS THAN (2025),
  PARTITION p2025 VALUES LESS THAN (2026),
  PARTITION pmax  VALUES LESS THAN MAXVALUE
);

-- 2023-01-01 起 1095 天，正好铺满 2023/2024/2025 三个分区
INSERT INTO pt
SELECT DATE_ADD('2023-01-01', INTERVAL n DAY), n
FROM (WITH RECURSIVE s AS (SELECT 0 AS n UNION ALL SELECT n + 1 FROM s WHERE n < 1094) SELECT n FROM s) x;
ANALYZE TABLE pt;

SELECT '== 每个分区一个 .ibd，一个分区一张"引擎表" ==' AS step;
SELECT partition_name, table_rows
FROM information_schema.partitions
WHERE table_schema = 'sp' AND table_name = 'pt'
ORDER BY partition_ordinal_position;

SELECT '== 分区裁剪：看 EXPLAIN 的 partitions 列 ==' AS step;
EXPLAIN SELECT * FROM pt WHERE ftime = '2024-04-01';              -- p2024
EXPLAIN SELECT * FROM pt WHERE ftime BETWEEN '2024-01-01' AND '2024-12-31'; -- p2024
EXPLAIN SELECT * FROM pt WHERE ftime >= '2024-04-01';             -- p2024,p2025,pmax
EXPLAIN SELECT * FROM pt WHERE ftime IN ('2023-06-01', '2025-06-01'); -- p2023,p2025
EXPLAIN SELECT * FROM pt WHERE c = 5;                             -- 不带分区键：全部
EXPLAIN SELECT * FROM pt WHERE YEAR(ftime) = 2024;                -- 列上包了函数：实测不裁剪

SELECT '== DROP PARTITION：删文件，不写 undo/redo/binlog 行 ==' AS step;
SELECT COUNT(*) AS rows_before FROM pt;
ALTER TABLE pt DROP PARTITION p2023;
SELECT COUNT(*) AS rows_after FROM pt;
SELECT partition_name, table_rows
FROM information_schema.partitions
WHERE table_schema = 'sp' AND table_name = 'pt'
ORDER BY partition_ordinal_position;

-- 分区键必须进主键：自增 id 想按时间分区，只能建成 (ftime, id)
DROP TABLE IF EXISTS pt2;
CREATE TABLE pt2 (
  id    INT NOT NULL AUTO_INCREMENT,
  ftime DATETIME NOT NULL,
  c     INT,
  PRIMARY KEY (ftime, id),
  KEY id (id)
) ENGINE = InnoDB
PARTITION BY RANGE (YEAR(ftime)) (
  PARTITION p2024 VALUES LESS THAN (2025),
  PARTITION pmax  VALUES LESS THAN MAXVALUE
);
SHOW CREATE TABLE pt2;
