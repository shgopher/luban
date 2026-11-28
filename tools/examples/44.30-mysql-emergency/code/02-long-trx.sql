-- 长事务：改 20 万行后不提交（用于观察 innodb_trx / data_locks / kill 回滚）
BEGIN;
UPDATE luban.t SET c = c + 1 WHERE id <= 200000;
SELECT SLEEP(300);
