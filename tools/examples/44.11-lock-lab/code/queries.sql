-- 判读锁现场的三条查询（MySQL 8.0：performance_schema.data_locks 取代了 5.7 的 information_schema.innodb_locks）
-- ① 当前所有行锁：INDEX_NAME / LOCK_MODE / LOCK_STATUS / LOCK_DATA
SELECT ENGINE_TRANSACTION_ID AS trx, OBJECT_NAME AS tbl, INDEX_NAME AS idx,
       LOCK_TYPE, LOCK_MODE, LOCK_STATUS, LOCK_DATA
FROM performance_schema.data_locks
ORDER BY INDEX_NAME, LOCK_DATA;

-- ② 谁在等谁
SELECT r.ENGINE_TRANSACTION_ID AS req_trx, r.INDEX_NAME, r.LOCK_MODE AS req_mode,
       b.ENGINE_TRANSACTION_ID AS blk_trx, b.LOCK_MODE AS blk_mode, b.LOCK_DATA AS blk_data
FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks r ON w.REQUESTING_ENGINE_LOCK_ID = r.ENGINE_LOCK_ID
JOIN performance_schema.data_locks b ON w.BLOCKING_ENGINE_LOCK_ID = b.ENGINE_LOCK_ID;

-- ③ 最近一次死锁的现场（LATEST DETECTED DEADLOCK 段）
SHOW ENGINE INNODB STATUS\G
