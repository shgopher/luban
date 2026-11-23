\pset pager off
\pset footer off
\set ON_ERROR_STOP off
SET max_parallel_workers_per_gather = 0;
\echo '=== 环境'
SELECT version();
SHOW max_parallel_workers_per_gather;

\echo ''
\echo '### C1 float8 vs numeric：100 万行 0.01 累加与累加后乘税率'
DROP TABLE IF EXISTS t_f, t_n;
CREATE TABLE t_f AS SELECT 0.01::float8 AS v FROM generate_series(1, 1000000);
CREATE TABLE t_n AS SELECT 0.01::numeric AS v FROM generate_series(1, 1000000);
SELECT 'float8   sum(v)' AS metric, sum(v)::text AS total FROM t_f
UNION ALL
SELECT 'numeric  sum(v)', sum(v)::text FROM t_n
UNION ALL
SELECT 'float8   sum(v*1.07)', sum(v*1.07)::text FROM t_f
UNION ALL
SELECT 'numeric  sum(v*1.07)', sum(v*1.07)::text FROM t_n
UNION ALL
SELECT 'float8   round(sum(v*1.07),2) 需先转 numeric', round((sum(v*1.07))::numeric, 2)::text FROM t_f
UNION ALL
SELECT 'numeric  round(sum(v*1.07),2)', round(sum(v*1.07), 2)::text FROM t_n
UNION ALL
SELECT 'float8   round(sum(v*1.07)) 单参（half-even）', round(sum(v*1.07))::text FROM t_f;

\echo ''
\echo '### C2 ROUND 的两种语义：numeric 是 half-up，float8 是 half-even'
\echo '-- 单参 round：一边一半（2.5）时的分歧'
SELECT round(2.5::float8) AS f2_5, round(2.5::numeric) AS n2_5,
       round(3.5::float8) AS f3_5, round(3.5::numeric) AS n3_5,
       round((-2.5)::float8) AS fneg2_5, round((-2.5)::numeric) AS nneg2_5;
\echo '-- PostgreSQL 没有 round(double precision, int)：保留 2 位必须先转 numeric'
SELECT round(2.675::float8::numeric, 2) AS f2675_cast, round(2.675::numeric, 2) AS n2675,
       round(1.005::float8::numeric, 2) AS f1005_cast, round(1.005::numeric, 2) AS n1005,
       round(0.125::float8::numeric, 2) AS f0125_cast, round(0.125::numeric, 2) AS n0125;
\echo '-- 2.675 在两个类型里的真实值'
SELECT 2.675::float8::numeric AS f2675_value, 2.675::numeric AS n2675_value,
       2.675::float8::numeric = 2.675::numeric AS same;

\echo ''
\echo '### C3 numeric 乘汇率后的 ROUND：位数不丢，但最终要落到币种 exponent'
SELECT 1234.5678::numeric * 7.3388::numeric AS product,
       pg_typeof(1234.5678::numeric * 7.3388::numeric) AS product_type,
       round(1234.5678::numeric * 7.3388::numeric, 2) AS to_2,
       round(1234.5678::numeric * 7.3388::numeric, 0) AS to_0,
       round(1234.5678::numeric * 7.3388::numeric, 3) AS to_3;

\echo ''
\echo '### C4 numeric(19,4) 的精度、舍入与溢出'
DROP TABLE IF EXISTS amt;
CREATE TABLE amt (id int, a numeric(19,4));
INSERT INTO amt VALUES (1, 0.00005), (2, 999999999999999.9999), (3, 0.000049);
SELECT id, a FROM amt ORDER BY id;
\echo '-- 插入 10^15（超出 numeric(19,4) 整数位 15 位上限）:'
INSERT INTO amt VALUES (4, 1000000000000000);
\echo '-- 插入 10^-5 级别的值（第 5 位小数）:'
INSERT INTO amt VALUES (5, 0.00005);
SELECT id, a FROM amt ORDER BY id;
\echo '-- 每值占用字节：numeric(19,4) 变长，bigint 定长 8'
SELECT pg_column_size(a) AS numeric_bytes FROM amt ORDER BY id;
SELECT pg_column_size(123456789::bigint) AS bigint_bytes;

\echo ''
\echo '### C5 100 万行求和后按币种 exponent 收口：JPY（0 位）vs CNY（2 位）'
SELECT 'JPY 收口' AS what, round(sum(v), 0)::text AS total FROM t_n
UNION ALL
SELECT 'CNY 收口', round(sum(v), 2)::text FROM t_n;

\echo ''
\echo '### C6 累加误差的分布：float8 单行舍入前的原始和 vs 精确和'
SELECT sum(v) - 10000::float8 AS float8_abs_error FROM t_f;
SELECT sum(v) - 10000::numeric AS numeric_abs_error FROM t_n;

\echo ''
\echo '### C6b 订单金额 numeric(19,4) × 汇率，写回列时隐式按 half-up 落 4 位'
DROP TABLE IF EXISTS fx_order;
CREATE TABLE fx_order (id int, amount numeric(19,4), rate numeric(18,8), amount_quote numeric(19,4));
INSERT INTO fx_order (id, amount, rate) VALUES (1, 1234.5678, 7.3388);
UPDATE fx_order SET amount_quote = amount * rate;
SELECT id, amount, rate, amount * rate AS exact_product, amount_quote,
       amount_quote - round(amount * rate, 4) AS diff_vs_exact FROM fx_order;

\echo ''
\echo '### C7 最大余数法分摊（SQL 版）：10000 分给 3 人'
WITH w AS (
  SELECT * FROM (VALUES (1, 1::numeric), (2, 1::numeric), (3, 1::numeric)) AS t(uid, weight)
), base AS (
  SELECT uid, weight,
         floor(10000 * weight / sum(weight) OVER ())::bigint AS share,
         (10000 * weight / sum(weight) OVER ()) - floor(10000 * weight / sum(weight) OVER ()) AS frac
  FROM w
), leftover AS (
  SELECT 10000 - sum(share) AS n FROM base
), ranked AS (
  SELECT uid, share, row_number() OVER (ORDER BY frac DESC, uid ASC) AS rn FROM base
)
SELECT uid, share + CASE WHEN rn <= (SELECT n FROM leftover) THEN 1 ELSE 0 END AS paid,
       sum(share + CASE WHEN rn <= (SELECT n FROM leftover) THEN 1 ELSE 0 END) OVER () AS total
FROM ranked ORDER BY uid;

\echo ''
\echo '=== 收尾'
