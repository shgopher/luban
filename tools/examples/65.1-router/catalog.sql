\pset pager off
\echo '===== 阶段 A：未建租户 schema（只有系统表）====='
select count(*) as pg_class_rows from pg_class;
select count(*) as schemas from pg_namespace;
select count(*) as info_schema_tables from information_schema.tables;
\echo '-- A1 information_schema.tables 全量扫描的代价'
explain (analyze, buffers, format text) select count(*) from information_schema.tables;
\echo '-- A2 精确到某个 schema 的元数据点查'
explain (analyze, buffers, format text) select table_name from information_schema.tables where table_schema = 't_0001';
\echo '-- A3 单 schema 的 catalog 点查'
explain (analyze, format text) select relname from pg_class where relnamespace = to_regnamespace('t_0001');
\echo '-- A4 库大小'
select pg_size_pretty(pg_database_size(current_database())) as db_size;

\echo '===== 阶段 B：建 3000 个租户 schema，每个 3 张表 ====='
\timing on
DO $$
DECLARE i int;
BEGIN
  FOR i IN 0..2999 LOOP
    EXECUTE format('CREATE SCHEMA t_%s', lpad(i::text, 4, '0'));
    EXECUTE format('CREATE TABLE t_%s.orders (id bigint primary key, amount numeric(12,2), created_at timestamptz default now())', lpad(i::text, 4, '0'));
    EXECUTE format('CREATE TABLE t_%s.users (id bigint primary key, email text not null)', lpad(i::text, 4, '0'));
    EXECUTE format('CREATE TABLE t_%s.audit (id bigint primary key, action text null)', lpad(i::text, 4, '0'));
  END LOOP;
END $$;
\timing off

\echo '===== 阶段 C：同样的查询再跑一遍 ====='
select count(*) as pg_class_rows from pg_class;
select count(*) as schemas from pg_namespace;
select count(*) as info_schema_tables from information_schema.tables;
\echo '-- C1 information_schema.tables 全量扫描的代价'
explain (analyze, buffers, format text) select count(*) from information_schema.tables;
\echo '-- C2 精确到某个 schema 的元数据点查'
explain (analyze, buffers, format text) select table_name from information_schema.tables where table_schema = 't_0001';
\echo '-- C3 单 schema 的 catalog 点查'
explain (analyze, format text) select relname from pg_class where relnamespace = to_regnamespace('t_0001');
\echo '-- C4 库大小'
select pg_size_pretty(pg_database_size(current_database())) as db_size;
