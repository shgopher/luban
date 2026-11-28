#!/usr/bin/env bash
# 44.26 一条 SQL 的一生：起一个一次性 MySQL 8.0，跑完四段观察，然后删掉容器。
#   bash code/run.sh          # 默认端口 33199
#   PORT=33199 bash code/run.sh
set -euo pipefail

PORT=${PORT:-33199}
NAME="mysql-lifecycle-$PORT"
HERE=$(cd "$(dirname "$0")" && pwd)

# macOS 上 docker 常在 App 包里；Linux 直接有
if ! command -v docker >/dev/null 2>&1; then
  export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
fi
if [ -z "${DOCKER_HOST:-}" ] && [ -S "$HOME/.docker/run/docker.sock" ]; then
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi
if ! command -v mysql >/dev/null 2>&1; then
  echo "need mysql client in PATH" >&2; exit 1
fi

TMPDIR_LAB=$(mktemp -d)
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  rm -rf "$TMPDIR_LAB"
}
trap cleanup EXIT

my() { mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP "$@"; }

echo "== 0. 起容器（镜像 mysql:8.0，--rm，端口 $PORT）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null

for i in $(seq 1 120); do
  if my -e "select 1" >/dev/null 2>&1; then break; fi
  sleep 1
  if [ "$i" -eq 120 ]; then echo "mysql not ready on $PORT" >&2; exit 1; fi
done

echo
echo "== 1. 灌数（t 两行；big 两万行，用来制造脏页）"
my < "$HERE/01-schema.sql"
my -t -e "USE lifecycle; SELECT COUNT(*) AS rows_t FROM t; SELECT COUNT(*) AS rows_big FROM big;"

echo
echo "== 2. 与持久化有关的参数实际取值"
my -t < "$HERE/02-params.sql"

echo
echo "== 3. 一个 update 在 binlog 里的形态"
my -e "FLUSH BINARY LOGS"
my < "$HERE/03-update.sql"
my -e "FLUSH BINARY LOGS"   # 让刚才那个 binlog 文件封口，便于离线解码
TARGET=$(my -N -B -e "SHOW BINARY LOGS" | awk '{print $1}' | tail -2 | head -1)
echo "-- 目标 binlog 文件：$TARGET"
my -t -e "SHOW BINLOG EVENTS IN '$TARGET'"

echo
echo "== 4. 执行器的计数：rows_examined（全表扫描 2 万行）"
my < "$HERE/04-rows-examined.sql"
# 打开慢日志、执行语句、读慢日志要分三次连接：同一条连接里刚写的慢日志行不一定可见
my -t -e "USE lifecycle; SELECT COUNT(*) AS cnt FROM big WHERE c > 0;"
my -t -e "SELECT rows_examined, rows_sent, ROUND(query_time,4) AS query_time_s,
                  LEFT(sql_text,45) AS sql_text
           FROM mysql.slow_log WHERE sql_text LIKE '%big%'
           ORDER BY start_time DESC LIMIT 3"

echo
echo "== 5. redo 的四个 LSN（innodb_flush_log_at_trx_commit=1）"
my -B -e "SHOW GLOBAL VARIABLES LIKE 'innodb_flush_log_at_trx_commit';"
my -B -e "USE lifecycle; UPDATE big SET c=c+1 WHERE id<=20000; SHOW ENGINE INNODB STATUS;" \
  | awk '{gsub(/\\n/,"\n"); print}' \
  | grep -E "Log sequence number|Log flushed up to|Pages flushed up to|Last checkpoint at|Modified db pages"

echo
echo "== 6. 同一负载，innodb_flush_log_at_trx_commit=0（commit 不等 fsync）"
my -e "SET GLOBAL innodb_flush_log_at_trx_commit=0"
my -B -e "USE lifecycle; UPDATE big SET c=c+1 WHERE id<=20000; SHOW ENGINE INNODB STATUS;" \
  | awk '{gsub(/\\n/,"\n"); print}' \
  | grep -E "Log sequence number|Log flushed up to|Pages flushed up to|Last checkpoint at|Modified db pages"
my -e "SET GLOBAL innodb_flush_log_at_trx_commit=1"   # 立刻改回来，别把危险配置留在实例上

echo
echo "== 7. 用 mysqlbinlog 解出行镜像（本机没装就跳过）"
if command -v mysqlbinlog >/dev/null 2>&1; then
  docker cp "$NAME:/var/lib/mysql/$TARGET" "$TMPDIR_LAB/$TARGET" >/dev/null
  mysqlbinlog --base64-output=DECODE-ROWS -vv "$TMPDIR_LAB/$TARGET" \
    | grep -E "^### (UPDATE|WHERE|SET|  @)|Xid =|Table_map|BEGIN|COMMIT" | head -20
else
  echo "(skipped: 本机无 mysqlbinlog，第 3 段的 SHOW BINLOG EVENTS 已够看事件类型)"
fi

echo
echo "== 完成：容器已删除"
