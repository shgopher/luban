use joinlab;
select '--- MRR 默认 mrr_cost_based=on: 不显示 Using MRR ---' as tag;
set optimizer_switch='mrr=on,mrr_cost_based=on,batched_key_access=off';
explain select * from t3 where a>=1 and a<=1000;

select '--- mrr_cost_based=off: 强制 MRR ---' as tag;
set optimizer_switch='mrr=on,mrr_cost_based=off,batched_key_access=off';
explain select * from t3 where a>=1 and a<=1000;

select '--- BKA 关闭: NLJ, Extra 为空 ---' as tag;
explain select t2.b from t3 straight_join t2 on t3.a=t2.a;

select '--- BKA 打开: Using join buffer (Batched Key Access) ---' as tag;
set optimizer_switch='mrr=on,mrr_cost_based=off,batched_key_access=on';
explain select t2.b from t3 straight_join t2 on t3.a=t2.a;

select '--- count(*)：覆盖索引不回表，BKA 不触发 ---' as tag;
explain select count(*) from t3 straight_join t2 on t3.a=t2.a;
