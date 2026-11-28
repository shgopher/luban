-- 01 准备数据：t1 是原文用的 1000 行小表，t_big 用来把内部临时表撑到磁盘
DROP DATABASE IF EXISTS d;
CREATE DATABASE d;
USE d;

CREATE TABLE t1(id int primary key, a int, b int, index(a)) ENGINE=InnoDB;

delimiter ;;
create procedure idata()
begin
  declare i int;
  set i=1;
  while(i<=1000)do
    insert into t1 values(i, i, i);
    set i=i+1;
  end while;
end;;
delimiter ;
call idata();
drop procedure idata;

-- 10 万行、5 万个不同分组值：分组结果的体积远大于任何合理的 tmp_table_size
CREATE TABLE t_big(id int primary key, a int, b varchar(50), index(a)) ENGINE=InnoDB;
set session cte_max_recursion_depth=200000;
INSERT INTO t_big
WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<100000)
SELECT n, n%997, concat('k', n%50000) FROM seq;

SELECT (SELECT count(*) FROM t1) AS t1_rows, (SELECT count(*) FROM t_big) AS t_big_rows;
