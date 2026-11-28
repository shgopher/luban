#!/usr/bin/env bash
# 44.10 事务隔离实验：RR / RC / 一致性快照 / 幻读 / 长事务
# 用法：bash code/run.sh     （默认端口 33199，可用 PORT=xxxx 覆盖）
set -euo pipefail
PORT=${PORT:-33199}
NAME="mysqllab-$PORT"
DIR="$(cd "$(dirname "$0")" && pwd)"
MYSQL="docker exec -i $NAME mysql -uroot --batch --raw --unbuffered --skip-column-names lab"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  rm -f /tmp/mvcc-a.in /tmp/mvcc-b.in /tmp/mvcc-a.out /tmp/mvcc-b.out
}
trap cleanup EXIT

docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" -e MYSQL_ALLOW_EMPTY_PASSWORD=yes \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 --innodb-buffer-pool-size=128M >/dev/null

# 等“正式 server”，不是初始化期间的临时 server：临时 server 不监听 TCP（port 0）
for i in $(seq 1 240); do
  docker exec "$NAME" mysql -h127.0.0.1 -P3306 -uroot -e 'select 1' >/dev/null 2>&1 && break
  sleep 1
done
echo "MySQL $(docker exec "$NAME" mysql -uroot -N -e 'select version()')  port=$PORT"

docker exec -i "$NAME" mysql -uroot < "$DIR/01-init.sql"

echo
echo "===== 0. 事务控制：连续 BEGIN / 遇错不回滚 / SAVEPOINT ====="
docker exec -i "$NAME" mysql -uroot --table --force < "$DIR/03-tx-control.sql" 2>&1

rm -f /tmp/mvcc-a.in /tmp/mvcc-b.in /tmp/mvcc-a.out /tmp/mvcc-b.out
mkfifo /tmp/mvcc-a.in /tmp/mvcc-b.in
($MYSQL < /tmp/mvcc-a.in > /tmp/mvcc-a.out 2>&1) &
($MYSQL < /tmp/mvcc-b.in > /tmp/mvcc-b.out 2>&1) &
exec 3>/tmp/mvcc-a.in
exec 4>/tmp/mvcc-b.in
n=0
run() { # sess out-file fd sql
  local s=$1 f=$2 fd=$3 sql=$4
  n=$((n+1)); local tag="@SYNC${s}${n}"
  printf '%s;\n' "$sql" >&$fd
  printf "select '%s';\n" "$tag" >&$fd
  for i in $(seq 1 300); do
    grep -q "$tag" "$f" 2>/dev/null && return 0
    sleep 0.1
  done
  echo "TIMEOUT $tag" >&2; exit 1
}
a() { run A /tmp/mvcc-a.out 3 "$1"; }
b() { run B /tmp/mvcc-b.out 4 "$1"; }
hll() { docker exec "$NAME" mysql -uroot -N lab -e "show engine innodb status" \
        | sed 's/\\n/\n/g' | grep -i 'History list length'; }

a "SELECT 'CONN_A' AS step, CONNECTION_ID()"
CONN_A=$(grep '^CONN_A' /tmp/mvcc-a.out | awk -F'\t' '{print $2}')
echo "会话 A 的 thread_id = $CONN_A"

echo
echo "===== 1. RR：快照读 vs 当前读 ====="
a "SET SESSION transaction_isolation='REPEATABLE-READ'"
a "SELECT 'RR-1 A begin' AS step; BEGIN"
a "SELECT 'RR-2 A 快照读' AS step, k FROM t WHERE id=1"
b "SELECT 'RR-3 B 更新并提交' AS step; UPDATE t SET k=k+1 WHERE id=1"
a "SELECT 'RR-4 A 再快照读' AS step, k FROM t WHERE id=1"
a "SELECT 'RR-5 A 当前读 for update' AS step, k FROM t WHERE id=1 FOR UPDATE"
a "SELECT 'RR-6 A commit' AS step; COMMIT"

echo
echo "===== 2. RC：同一条 SQL 在下一个语句看到新值 ====="
a "SET SESSION transaction_isolation='READ-COMMITTED'"
a "SELECT 'RC-1 A begin' AS step; BEGIN"
a "SELECT 'RC-2 A 读' AS step, k FROM t WHERE id=1"
b "SELECT 'RC-3 B 更新并提交' AS step; UPDATE t SET k=k+1 WHERE id=1"
a "SELECT 'RC-4 A 再读' AS step, k FROM t WHERE id=1"
a "SELECT 'RC-5 A commit' AS step; COMMIT"

echo
echo "===== 3. 快照在 begin 后第一条语句才建立 ====="
a "SET SESSION transaction_isolation='REPEATABLE-READ'"
a "SELECT 'CS-1 A with consistent snapshot' AS step; START TRANSACTION WITH CONSISTENT SNAPSHOT"
a "SELECT 'CS-2 A 读' AS step, k FROM t WHERE id=1"
b "SELECT 'CS-3 B 更新并提交' AS step; UPDATE t SET k=k+1 WHERE id=1"
a "SELECT 'CS-4 A 再读' AS step, k FROM t WHERE id=1"
a "SELECT 'CS-5 A commit' AS step; COMMIT"

echo
echo "===== 4. 幻读：快照读不幻读，当前读幻读 ====="
a "SELECT 'PH-1 A begin' AS step; BEGIN"
a "SELECT 'PH-2 A 快照读 count' AS step, COUNT(*) FROM t2 WHERE id>=1"
b "SELECT 'PH-3 B 插入 id=4' AS step; INSERT INTO t2 VALUES (4,4)"
a "SELECT 'PH-4 A 再快照读 count' AS step, COUNT(*) FROM t2 WHERE id>=1"
a "SELECT 'PH-5 A 当前读 count for update' AS step, COUNT(*) FROM t2 WHERE id>=1 FOR UPDATE"
a "SELECT 'PH-6 A commit' AS step; COMMIT"

echo
echo "===== 5. 长事务：innodb_trx 与 History list length ====="
echo "--- 5.1 基线 ---"; hll
a "SELECT 'LT-1 A begin' AS step; BEGIN"
a "SELECT 'LT-2 A 建视图' AS step, SUM(k) FROM big"
docker exec "$NAME" mysql -uroot -t lab \
  -e "SELECT trx_id, trx_state, trx_started, trx_isolation_level, trx_rows_modified \
      FROM information_schema.innodb_trx WHERE trx_mysql_thread_id = $CONN_A"
echo "--- 5.2 B 用 200 个独立事务更新 200 行，A 的旧视图还在 ---"
for i in $(seq 1 200); do echo "UPDATE big SET k=k+1 WHERE id=$i;"; done \
  | docker exec -i "$NAME" mysql -uroot lab
hll
a "SELECT 'LT-3 A 仍读旧快照 sum(k)' AS step, SUM(k) FROM big"
a "SELECT 'LT-4 A commit' AS step; COMMIT"
sleep 2
echo "--- 5.3 A 提交 2s 后 ---"; hll

echo
echo "===== A 会话输出 ====="
grep -v '^@SYNC' /tmp/mvcc-a.out
echo "===== B 会话输出 ====="
grep -v '^@SYNC' /tmp/mvcc-b.out
exec 3>&-; exec 4>&-
