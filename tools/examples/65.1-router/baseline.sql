\pset pager off
CREATE SCHEMA t_0042;
CREATE TABLE t_0042.orders (id bigint primary key, amount numeric(12,2), created_at timestamptz default now());
CREATE TABLE t_0042.users (id bigint primary key, email text not null);
CREATE TABLE t_0042.audit (id bigint primary key, action text);
\echo '===== 对照：只有一个租户 schema 的库（同样 3 张表）====='
select count(*) as info_schema_tables from information_schema.tables;
\echo 'B1 information_schema.tables 全量扫描'
explain (analyze, format text) select count(*) from information_schema.tables;
\echo 'B2 information_schema.tables 精确到单个 schema'
explain (analyze, format text) select table_name from information_schema.tables where table_schema = 't_0042';
\echo 'B3 pg_class 按 schema 点查'
explain (analyze, format text) select relname from pg_class where relnamespace = to_regnamespace('t_0042');
\echo 'B4 单租户业务查询（空表 count）'
explain (analyze, format text) select count(*) from t_0042.orders;
