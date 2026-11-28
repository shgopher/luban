-- MySQL 8.0.46：10,000 行驱动表 + 100,000 行被驱动表
drop database if exists joinlab;
create database joinlab;
use joinlab;
set cte_max_recursion_depth = 200000;

create table t1 (id int primary key, a int, b int, key(a)) engine=innodb;
create table t2 (id int primary key, a int, b int, key(a)) engine=innodb;
insert into t1 with recursive s as (select 1 n union all select n+1 from s where n<10000)
  select n, n, n from s;
insert into t2 with recursive s as (select 1 n union all select n+1 from s where n<100000)
  select n, n, n from s;

-- a 列逆序，制造“按二级索引范围查、按主键回表”的随机 IO
create table t3 (id int primary key, a int, b int, key(a)) engine=innodb;
insert into t3 select id, 10001-a, b from t1;

-- 44 讲 left join 例子：a.f1 有索引，b.f1 无索引，两表各 6 行、4 行匹配
create table la(f1 int, f2 int, index(f1)) engine=innodb;
create table lb(f1 int, f2 int) engine=innodb;
insert into la values(1,1),(2,2),(3,3),(4,4),(5,5),(6,6);
insert into lb values(3,3),(4,4),(5,5),(6,6),(7,7),(8,8);

analyze table t1, t2, t3, la, lb;

select (select count(*) from t1) t1_rows, (select count(*) from t2) t2_rows,
       (select count(*) from t3) t3_rows;
