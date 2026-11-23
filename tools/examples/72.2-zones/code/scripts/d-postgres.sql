\pset pager off
SET client_min_messages TO warning;
\echo '===== D0 环境 ====='
SELECT version();
SELECT current_setting('TimeZone') AS session_timezone;
SELECT current_setting('TimeZone') AS tz_used_by_date_trunc;

\echo '===== D1 同一字面量 2026-01-15 09:00，在三个 session 时区分别插入 timestamptz 与 timestamp ====='
DROP TABLE IF EXISTS ev;
CREATE TABLE ev (
  id       int PRIMARY KEY,
  note     text,
  ts_tz    timestamptz,
  ts_plain timestamp
);
SET TIME ZONE 'UTC';
INSERT INTO ev VALUES (1, 'UTC 会话插入', TIMESTAMP '2026-01-15 09:00:00', TIMESTAMP '2026-01-15 09:00:00');
SET TIME ZONE 'America/New_York';
INSERT INTO ev VALUES (2, 'NY 会话插入', TIMESTAMP '2026-01-15 09:00:00', TIMESTAMP '2026-01-15 09:00:00');
SET TIME ZONE 'Asia/Shanghai';
INSERT INTO ev VALUES (3, 'SH 会话插入', TIMESTAMP '2026-01-15 09:00:00', TIMESTAMP '2026-01-15 09:00:00');

\echo '--- 同一批行，在不同 SET TIME ZONE 下显示 ---'
SET TIME ZONE 'UTC';
SELECT id, note, ts_tz, ts_plain FROM ev ORDER BY id;
SET TIME ZONE 'America/New_York';
SELECT id, note, ts_tz, ts_plain FROM ev ORDER BY id;
SET TIME ZONE 'Asia/Shanghai';
SELECT id, note, ts_tz, ts_plain FROM ev ORDER BY id;
SET TIME ZONE 'Europe/Berlin';
SELECT id, note, ts_tz, ts_plain FROM ev ORDER BY id;

\echo '===== D2 timestamptz 的真实存储：AT TIME ZONE 固定到 UTC ====='
SET TIME ZONE 'Asia/Shanghai';
SELECT id, ts_tz AT TIME ZONE 'UTC' AS stored_utc, ts_tz AT TIME ZONE 'America/New_York' AS in_ny
FROM ev ORDER BY id;

\echo '===== D3 DST gap：在 NY 会话插入 2026-03-08 02:30（该墙钟不存在） ====='
DROP TABLE IF EXISTS dst;
CREATE TABLE dst (id int PRIMARY KEY, note text, wall timestamp, as_tz timestamptz);
SET TIME ZONE 'America/New_York';
INSERT INTO dst VALUES (1, 'gap 02:30', TIMESTAMP '2026-03-08 02:30:00', TIMESTAMP '2026-03-08 02:30:00');
INSERT INTO dst VALUES (2, 'gap 02:00', TIMESTAMP '2026-03-08 02:00:00', TIMESTAMP '2026-03-08 02:00:00');
INSERT INTO dst VALUES (3, 'overlap 01:30', TIMESTAMP '2026-11-01 01:30:00', TIMESTAMP '2026-11-01 01:30:00');
SET TIME ZONE 'UTC';
SELECT id, note, wall, as_tz, as_tz AT TIME ZONE 'America/New_York' AS back_to_ny FROM dst ORDER BY id;
SET TIME ZONE 'America/New_York';
SELECT id, note, wall, as_tz, as_tz AT TIME ZONE 'America/New_York' AS back_to_ny FROM dst ORDER BY id;

\echo '===== D3b 同一字符串 cast 成 timestamptz，结果由 session 时区决定 ====='
SET TIME ZONE 'UTC';
SELECT '2026-03-08 02:30:00'::timestamptz AS as_utc_session;
SET TIME ZONE 'America/New_York';
SELECT '2026-03-08 02:30:00'::timestamptz AS as_ny_session;

\echo '===== D4 date_trunc(day) 在 session 时区下分桶：同一批订单，三种 session ====='
DROP TABLE IF EXISTS orders;
CREATE TABLE orders (id int PRIMARY KEY, ts timestamptz);
INSERT INTO orders VALUES
 (1, TIMESTAMP '2026-10-31 22:00:00+00'),
 (2, TIMESTAMP '2026-11-01 03:59:00+00'),
 (3, TIMESTAMP '2026-11-01 04:30:00+00'),
 (4, TIMESTAMP '2026-11-01 05:30:00+00'),
 (5, TIMESTAMP '2026-11-01 06:30:00+00'),
 (6, TIMESTAMP '2026-11-01 17:00:00+00');
SET TIME ZONE 'UTC';
SELECT date_trunc('day', ts) AS bucket, count(*) FROM orders GROUP BY 1 ORDER BY 1;
SET TIME ZONE 'Asia/Shanghai';
SELECT date_trunc('day', ts) AS bucket, count(*) FROM orders GROUP BY 1 ORDER BY 1;
SET TIME ZONE 'America/New_York';
SELECT date_trunc('day', ts) AS bucket, count(*) FROM orders GROUP BY 1 ORDER BY 1;

\echo '===== D5 显式业务时区分桶：AT TIME ZONE 把 timestamptz 变成"不带时区的本地墙钟" ====='
SELECT date_trunc('day', ts AT TIME ZONE 'Asia/Shanghai') AS sh_bucket, count(*) AS sh_cnt
FROM orders GROUP BY 1 ORDER BY 1;
SELECT date_trunc('day', ts AT TIME ZONE 'America/New_York') AS ny_bucket, count(*) AS ny_cnt
FROM orders GROUP BY 1 ORDER BY 1;
SELECT pg_typeof(ts AT TIME ZONE 'Asia/Shanghai') AS type_of_local_wallclock FROM orders LIMIT 1;

\echo '===== D6 DST 当天到底几个小时（用 interval 算术） ====='
SELECT ('2026-03-09 00:00'::timestamp AT TIME ZONE 'America/New_York')
     - ('2026-03-08 00:00'::timestamp AT TIME ZONE 'America/New_York') AS spring_day,
       ('2026-11-02 00:00'::timestamp AT TIME ZONE 'America/New_York')
     - ('2026-11-01 00:00'::timestamp AT TIME ZONE 'America/New_York') AS fall_day;

\echo '===== D7 pg_timezone_abbrevs：缩写不是唯一键 ====='
SELECT abbrev, utc_offset, is_dst FROM pg_timezone_abbrevs WHERE abbrev IN ('CST','IST','BST','EST') ORDER BY abbrev;
