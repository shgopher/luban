\pset pager off
\echo '===== 共享表 + tenant_id：200 万行、头部租户占 50% 的病态分布 ====='
\timing on
DROP TABLE IF EXISTS shared_orders;
CREATE TABLE shared_orders (
  tenant_id int not null,
  id bigint not null,
  amount numeric(12,2) not null,
  created_at timestamptz default now(),
  primary key (tenant_id, id)
);
INSERT INTO shared_orders
SELECT CASE WHEN g % 2 = 0 THEN 1 ELSE 1 + (g % 999) END, g, (random()*1000)::numeric(12,2)
FROM generate_series(1, 2000000) g;
\timing off
\echo '-- 表与索引体积'
select pg_size_pretty(pg_total_relation_size('shared_orders')) as total,
       pg_size_pretty(pg_relation_size('shared_orders_pkey')) as pkey;
\echo '-- 每个租户的行数分布（前 3 / 后 3）'
select tenant_id, count(*) as rows from shared_orders group by 1 order by 2 desc limit 3;
select tenant_id, count(*) as rows from shared_orders group by 1 order by 2 asc limit 3;
\echo '-- S1 大租户(1) 点查：走 (tenant_id,id) 主键'
explain (analyze, format text) select * from shared_orders where tenant_id = 1 and id = 2000000;
\echo '-- S2 小租户(7) 点查'
explain (analyze, format text) select * from shared_orders where tenant_id = 7 and id = 1999999;
\echo '-- S3 大租户全量统计（1M 行）'
explain (analyze, format text) select count(*) from shared_orders where tenant_id = 1;
\echo '-- S4 小租户全量统计（约 1000 行）'
explain (analyze, format text) select count(*) from shared_orders where tenant_id = 7;
