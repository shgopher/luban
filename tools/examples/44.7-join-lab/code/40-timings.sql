use joinlab;
set profiling=1;
set profiling_history_size=100;
set optimizer_switch='hash_join=on,block_nested_loop=on,batched_key_access=off,mrr=on,mrr_cost_based=on';
set session join_buffer_size=262144;

select '--- NLJ: t1(10k) 驱动 t2(100k), t2.a 有索引, 3 次 ---' as tag;
select count(*) from t1 straight_join t2 on t1.a=t2.a;
select count(*) from t1 straight_join t2 on t1.a=t2.a;
select count(*) from t1 straight_join t2 on t1.a=t2.a;

select '--- hash join: t1 驱动 t2, t2.b 无索引, 3 次 ---' as tag;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;

select '--- 大表驱动: t2(100k) 驱动 t1(10k), NLJ, 3 次 ---' as tag;
select count(*) from t2 straight_join t1 on t1.a=t2.a;
select count(*) from t2 straight_join t1 on t1.a=t2.a;
select count(*) from t2 straight_join t1 on t1.a=t2.a;

select '--- Simple NLJ: 200 x 100k, 关 join buffer, 3 次 ---' as tag;
set optimizer_switch='block_nested_loop=off';
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;

select '--- hash join: 同样 200 x 100k, 3 次 ---' as tag;
set optimizer_switch='block_nested_loop=on';
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.b where t1.id<=200;

select '--- NLJ: 同样 200 x 100k, 3 次 ---' as tag;
select count(*) from t1 straight_join t2 on t1.a=t2.a where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.a where t1.id<=200;
select count(*) from t1 straight_join t2 on t1.a=t2.a where t1.id<=200;

select '--- join_buffer_size=128K, hash join, 3 次 ---' as tag;
set session join_buffer_size=131072;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;

select '--- join_buffer_size=4M, hash join, 3 次 ---' as tag;
set session join_buffer_size=4194304;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;
select count(*) from t1 straight_join t2 on t1.a=t2.b;

show profiles;
