use joinlab;
select '--- Q1: 条件都在 ON，返回 6 行 ---' as tag;
select * from la left join lb on(la.f1=lb.f1) and (la.f2=lb.f2);
select count(*) as q1_rows from la left join lb on(la.f1=lb.f1) and (la.f2=lb.f2);

select '--- Q2: 一个条件在 WHERE，返回 4 行 ---' as tag;
select * from la left join lb on(la.f1=lb.f1) where (la.f2=lb.f2);
select count(*) as q2_rows from la left join lb on(la.f1=lb.f1) where (la.f2=lb.f2);

select '--- Q2 的 EXPLAIN: b 成了驱动表 ---' as tag;
explain select * from la left join lb on(la.f1=lb.f1) where (la.f2=lb.f2);

select '--- Q1 的 EXPLAIN: a 是驱动表 ---' as tag;
explain select * from la left join lb on(la.f1=lb.f1) and (la.f2=lb.f2);

select '--- Q2 被改写成 inner join（SHOW WARNINGS）---' as tag;
explain select * from la left join lb on(la.f1=lb.f1) where (la.f2=lb.f2);
show warnings;
