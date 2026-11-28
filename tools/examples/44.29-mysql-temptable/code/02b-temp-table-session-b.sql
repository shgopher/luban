-- 02b 会话 B：看不到 A 的临时表，同名标识符落到普通表
USE d;

SELECT '--- B: show tables（只有普通表）' AS step;
SHOW TABLES;

SELECT '--- B: show create table t1（看到的是普通表）' AS step;
SHOW CREATE TABLE t1;

SELECT '--- B: select count(*) from t1（1000，A 的临时表对 B 不可见）' AS step;
SELECT count(*) AS normal_t1_rows FROM t1;

SELECT '--- B: 最后一条演示报错：访问只有 A 才有的临时表 t_tmp2 -> ERROR 1146' AS step;
SELECT count(*) FROM t_tmp2;
