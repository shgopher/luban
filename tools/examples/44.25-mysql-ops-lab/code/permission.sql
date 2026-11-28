-- 44.25 配套实验 2：grant 同时改磁盘和内存；直接 DML 权限表才需要 flush
-- 注意：这些操作会改 mysql.user，只能在一次性容器里跑
DROP DATABASE IF EXISTS lab4425_perm;
CREATE DATABASE lab4425_perm;

DROP USER IF EXISTS 'ub'@'%';
CREATE USER 'ub'@'%' IDENTIFIED WITH caching_sha2_password BY 'pb';
GRANT SELECT ON lab4425_perm.* TO 'ub'@'%';

SELECT '1. grant 之后无需 flush，磁盘和内存都已更新' AS step;
SHOW GRANTS FOR 'ub'@'%';

DELETE FROM mysql.user WHERE user = 'ub';

SELECT '2. 磁盘行已删：此时 GRANT 失败（1410）' AS step;
GRANT SELECT ON lab4425_perm.* TO 'ub'@'%';

SELECT '3. 此时 CREATE USER 也失败（1396，内存认为用户还在）' AS step;
CREATE USER 'ub'@'%' IDENTIFIED WITH caching_sha2_password BY 'pb';

FLUSH PRIVILEGES;

SELECT '4. flush 之后内存与磁盘一致，可以重新建用户' AS step;
DROP USER IF EXISTS 'ub'@'%';
CREATE USER 'ub'@'%' IDENTIFIED WITH caching_sha2_password BY 'pb';
GRANT SELECT ON lab4425_perm.* TO 'ub'@'%';
SHOW GRANTS FOR 'ub'@'%';
