-- 02a 会话 A：建临时表、与普通表同名、看服务端记录的真实临时表名
USE d;

SELECT connection_id() AS a_thread_id, @@tmpdir AS tmpdir;

-- ① 独立名字的临时表
CREATE TEMPORARY TABLE t_tmp(id int primary key, note varchar(20)) ENGINE=InnoDB;
INSERT INTO t_tmp VALUES (100,'temp-only');

-- ② 与普通表 t1 同名的临时表：不需要改普通表，直接建
CREATE TEMPORARY TABLE t1(id int primary key, a int, b int, note varchar(20)) ENGINE=InnoDB;
INSERT INTO t1(id,a,b,note) VALUES (100,100,100,'temp-shadow');

SELECT '--- A: show create table t1（看到的是临时表）' AS step;
SHOW CREATE TABLE t1;

SELECT '--- A: select * from t1（读到的是临时表的 1 行，不是普通表的 1000 行）' AS step;
SELECT * FROM t1;

SELECT '--- A: show tables（临时表不出现，只有普通表 t1）' AS step;
SHOW TABLES;

SELECT '--- A: information_schema.innodb_temp_table_info（服务端视角的真实名字）' AS step;
SELECT table_id, name FROM information_schema.innodb_temp_table_info;

-- ③ 改临时表名：ALTER TABLE 可以，RENAME TABLE 不行（RENAME 按 库名/表名.frm 去磁盘找文件）
SELECT '--- A: alter table t_tmp rename to t_tmp2（可用）' AS step;
ALTER TABLE t_tmp RENAME TO t_tmp2;
SHOW TABLES;

SELECT '--- A: 挂住 6 秒，让会话 B 在临时表存活期间做对照' AS step;
SELECT sleep(6);
