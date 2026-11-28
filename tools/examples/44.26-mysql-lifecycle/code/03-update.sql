-- 只做一次 update，其余观察交给 run.sh（避免把 InnoDB Status 的巨量输出灌进 SQL 文件）
USE lifecycle;
UPDATE t SET c=c+1 WHERE id=2;
