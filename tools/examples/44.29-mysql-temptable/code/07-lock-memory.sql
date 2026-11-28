-- 07a Memory 表的表锁：B 更新另一行也要等 A 的 update 语句结束
USE d;
SET @t0 = NOW(6);
UPDATE t_mem SET c=c+1 WHERE id=2;
SELECT TIMESTAMPDIFF(MICROSECOND, @t0, NOW(6))/1000000 AS memory_wait_seconds;
