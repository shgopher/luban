-- 10 两个微基准：临时表的引擎选择、Memory 表与 InnoDB 的等值查询
USE d;

SELECT '--- 10.1 同样 5 万行、同样 index(b)：Memory 临时表 vs InnoDB 临时表' AS step;
DROP TEMPORARY TABLE IF EXISTS tt_inn, tt_mem;
CREATE TEMPORARY TABLE tt_inn(id int primary key, b varchar(50), index(b)) ENGINE=InnoDB;
SET @t0=NOW(6);
INSERT INTO tt_inn SELECT id, b FROM t_big LIMIT 50000;
SELECT count(*) AS rows_50k,
       round(TIMESTAMPDIFF(MICROSECOND,@t0,NOW(6))/1000000,3) AS innodb_insert_seconds
FROM tt_inn;

CREATE TEMPORARY TABLE tt_mem(id int primary key, b varchar(50), index(b)) ENGINE=Memory;
SET @t0=NOW(6);
INSERT INTO tt_mem SELECT id, b FROM t_big LIMIT 50000;
SELECT count(*) AS rows_50k,
       round(TIMESTAMPDIFF(MICROSECOND,@t0,NOW(6))/1000000,3) AS memory_insert_seconds
FROM tt_mem;

SELECT '--- 10.2 同一张临时表不能在一条语句里打开两次（预期报错 1137）' AS step;
SELECT count(*) FROM tt_mem a JOIN tt_mem b ON a.b=b.b;

SELECT '--- 10.3 2 万次主键等值查询：Memory vs InnoDB（两表都只有 10 行）' AS step;
DROP PROCEDURE IF EXISTS bench_mem;
DROP PROCEDURE IF EXISTS bench_inn;
delimiter //
CREATE PROCEDURE bench_mem(n int)
BEGIN
  DECLARE i int DEFAULT 0; DECLARE v int;
  SET @t0=NOW(6);
  WHILE i<n DO
    SELECT c INTO v FROM t_mem WHERE id=(i%10);
    SET i=i+1;
  END WHILE;
  SELECT 'memory' AS engine, n AS lookups,
         round(TIMESTAMPDIFF(MICROSECOND,@t0,NOW(6))/1000000,3) AS seconds;
END//
CREATE PROCEDURE bench_inn(n int)
BEGIN
  DECLARE i int DEFAULT 0; DECLARE v int;
  SET @t0=NOW(6);
  WHILE i<n DO
    SELECT c INTO v FROM t_inn WHERE id=(i%10);
    SET i=i+1;
  END WHILE;
  SELECT 'innodb' AS engine, n AS lookups,
         round(TIMESTAMPDIFF(MICROSECOND,@t0,NOW(6))/1000000,3) AS seconds;
END//
delimiter ;
CALL bench_mem(20000);
CALL bench_inn(20000);
