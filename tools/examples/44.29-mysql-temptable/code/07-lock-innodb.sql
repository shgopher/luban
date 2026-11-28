-- 07b InnoDB 的行锁：B 更新另一行不受 A 影响
USE d;
SET @t0 = NOW(6);
UPDATE t_inn SET c=c+1 WHERE id=2;
SELECT TIMESTAMPDIFF(MICROSECOND, @t0, NOW(6))/1000000 AS innodb_wait_seconds;
