#!/usr/bin/env bash
# 实验 7：只查一行也慢——等 MDL、等行锁（原文第 19 讲）
# 需要 3 个并发连接，所以用 shell 起后台会话，而不是单文件 SQL。
set -u
PORT="${1:-33199}"
MYSQL=(mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "=== 7.1 等 MDL 写锁：DDL/lock tables 把 select 堵在表级 ==="
"${MYSQL[@]}" -e "use lab; LOCK TABLES words WRITE; DO SLEEP(10); UNLOCK TABLES;" >/dev/null 2>&1 &
BLOCKER=$!
sleep 1

# 被堵的查询：lock_wait_timeout 设小一点，避免真的等 1 年
"${MYSQL[@]}" -e "use lab; SET SESSION lock_wait_timeout=8; SELECT COUNT(*) FROM words;" \
  >"$TMP/blocked.txt" 2>&1 &
BLOCKED=$!
sleep 1

echo "--- processlist：谁在等 ---"
"${MYSQL[@]}" -t -e "SELECT id, command, time, state, LEFT(info,40) AS info
  FROM information_schema.processlist
 WHERE state LIKE '%metadata lock%' OR info LIKE '%COUNT(*)%';"

echo "--- sys.schema_table_lock_waits：谁堵了谁 ---"
"${MYSQL[@]}" -t -e "SELECT waiting_pid, blocking_pid, object_name, waiting_query
  FROM sys.schema_table_lock_waits;"

wait "$BLOCKED"
echo "--- 被堵语句的最终结果（超时报错 1205）---"
cat "$TMP/blocked.txt"
wait "$BLOCKER" 2>/dev/null

echo
echo "=== 7.2 等行锁：update 不提交，select ... lock in share mode 被堵 ==="
"${MYSQL[@]}" -e "use lab; BEGIN; UPDATE words SET word='locked' WHERE id=1; DO SLEEP(10); COMMIT;" >/dev/null 2>&1 &
BLOCKER=$!
sleep 1

"${MYSQL[@]}" -e "use lab; SET SESSION innodb_lock_wait_timeout=8;
  SELECT * FROM words WHERE id=1 LOCK IN SHARE MODE;" >"$TMP/blocked2.txt" 2>&1 &
BLOCKED=$!
sleep 1

echo "--- sys.innodb_lock_waits：谁持有行锁 ---"
"${MYSQL[@]}" -t -e "SELECT waiting_pid, blocking_pid, waiting_query, blocking_query
  FROM sys.innodb_lock_waits;"

wait "$BLOCKED"
echo "--- 被堵语句的最终结果（超时报错 1205）---"
cat "$TMP/blocked2.txt"
wait "$BLOCKER" 2>/dev/null

echo
echo "=== 7.3 长事务与一致性读：扫 1 行却读了 100 万个 undo 版本（原文数字，本机未复现）==="
echo "本机只验证「锁」这一半；undo 版本链过长的百万次 update 耗时以原文为准。"
