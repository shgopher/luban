use joinlab;
select '--- NLJ: 被驱动表 t2.a 有索引 ---' as tag;
explain select count(*) from t1 straight_join t2 on t1.a=t2.a;

select '--- 无索引: 8.0.46 走 hash join ---' as tag;
explain select count(*) from t1 straight_join t2 on t1.a=t2.b;

select '--- straight_join 反过来: t2(100k) 驱动 ---' as tag;
explain select count(*) from t2 straight_join t1 on t1.a=t2.a;

select '--- 关掉 join buffer: 退回 Simple NLJ ---' as tag;
set optimizer_switch='block_nested_loop=off';
explain select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;
set optimizer_switch='block_nested_loop=on';
