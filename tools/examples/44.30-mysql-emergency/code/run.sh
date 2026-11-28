#!/usr/bin/env bash
# 44.30 MySQL 应急手册：一条命令复现
#   长事务 -> innodb_trx / data_locks -> KILL 回滚
#   60MB 结果集 -> Sending to client -> KILL QUERY 无效 -> KILL CONNECTION 退出
#   巡检 SQL / 状态计数器 / 关键参数
#
# 依赖：Docker（mysql:8.0 镜像）、本机 mysql 客户端（8.0）
# 用法：bash code/run.sh          # 默认端口 33199
#       PORT=33199 bash code/run.sh
set -uo pipefail

export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=unix://${HOME}/.docker/run/docker.sock

PORT="${PORT:-33199}"
NAME="mysqllab-${PORT}"
DIR="$(cd "$(dirname "$0")" && pwd)"
MYSQL=(mysql -h127.0.0.1 -P"${PORT}" -uroot --protocol=TCP --connect-timeout=5)

say() { printf '\n=== %s ===\n' "$*"; }

cleanup() {
  for p in $(jobs -p); do kill "${p}" 2>/dev/null; done
  docker rm -f "${NAME}" >/dev/null 2>&1
}
trap cleanup EXIT

command -v mysql >/dev/null || { echo "需要本机 mysql 客户端（8.0）"; exit 1; }

say "启动 MySQL 8.0（端口 ${PORT}，buffer pool 128M）"
docker rm -f "${NAME}" >/dev/null 2>&1
docker run -d --rm --name "${NAME}" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% -e MYSQL_INITDB_SKIP_TZINFO=1 \
  -p "127.0.0.1:${PORT}:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null

for _ in $(seq 1 300); do
  "${MYSQL[@]}" -e "SELECT 1" >/dev/null 2>&1 && break
  sleep 1
done
if ! "${MYSQL[@]}" -e "SELECT 1" >/dev/null 2>&1; then echo "MySQL 未在 300s 内就绪"; exit 1; fi
"${MYSQL[@]}" -N -e "SELECT CONCAT('version=', @@version, ' buffer_pool_mb=', @@innodb_buffer_pool_size/1024/1024, ' isolation=', @@transaction_isolation)"

say "准备数据（20 万行小行 + 20 万行 × 300B 大行）"
"${MYSQL[@]}" < "${DIR}/00-setup.sql"
"${MYSQL[@]}" -t -e "SELECT (SELECT COUNT(*) FROM luban.t) AS t_rows, (SELECT COUNT(*) FROM luban.big) AS big_rows, ROUND((SELECT SUM(LENGTH(pad)+17) FROM luban.big)/1024/1024,1) AS big_payload_mb"

say "关键参数实际值"
"${MYSQL[@]}" -t < "${DIR}/04-vars.sql"

say "造一个 20 万行、不提交的长事务（后台 session）"
"${MYSQL[@]}" < "${DIR}/02-long-trx.sql" >/dev/null 2>&1 &
TRX_PID=$!

TID=""
for _ in $(seq 1 40); do
  TID="$("${MYSQL[@]}" -N -e "SELECT trx_mysql_thread_id FROM information_schema.innodb_trx WHERE trx_rows_modified = 200000 LIMIT 1" 2>/dev/null)"
  [ -n "${TID}" ] && break
  sleep 0.25
done
if [ -z "${TID}" ]; then echo "长事务未出现，退出"; exit 1; fi

"${MYSQL[@]}" -t -e "SELECT trx_id, trx_state, trx_started, trx_rows_modified, trx_mysql_thread_id, trx_is_read_only FROM information_schema.innodb_trx"
"${MYSQL[@]}" -t -e "SELECT COUNT(*) AS data_locks_rows, SUM(lock_status='WAITING') AS waiting FROM performance_schema.data_locks"

say "起一条 60MB 结果集查询，客户端不读（管道挂起）"
( "${MYSQL[@]}" --quick -N -e "SELECT * FROM luban.big" | (sleep 60) ) >/dev/null 2>&1 &
sleep 4
if ! "${MYSQL[@]}" -e "SELECT 1" >/dev/null 2>&1; then
  echo "!! 容器在 60MB 流式阶段被移除（宿主内存不足时常见），请释放内存后重跑；前 3 组输出已打印"
  exit 2
fi

say "巡检 SQL（长事务 + Sending to client 同时存在）"
"${MYSQL[@]}" -t < "${DIR}/01-patrol.sql"
"${MYSQL[@]}" -t -e "SELECT id, command, time, state, LEFT(info,36) AS info FROM information_schema.processlist WHERE command = 'Query'"

say "KILL 长事务，观察回滚（trx_state / trx_rows_modified）"
echo "KILL 之前："; "${MYSQL[@]}" -t -e "SHOW GLOBAL STATUS LIKE 'Com_kill'"
START=${SECONDS}
"${MYSQL[@]}" -e "KILL CONNECTION ${TID}"
for i in $(seq 1 30); do
  ROW="$("${MYSQL[@]}" -N -e "SELECT CONCAT(trx_state, ' rows_modified=', trx_rows_modified, ' op=', IFNULL(trx_operation_state,'-')) FROM information_schema.innodb_trx WHERE trx_mysql_thread_id = ${TID}" 2>/dev/null)"
  if [ -n "${ROW}" ]; then
    printf 't+%-3ss %s\n' "$((SECONDS-START))" "${ROW}"
  else
    printf 't+%-3ss trx gone\n' "$((SECONDS-START))"
    break
  fi
  sleep 0.4
done

say "对 Sending to client 的语句执行 KILL QUERY（预期：无效）"
BID="$("${MYSQL[@]}" -N -e "SELECT id FROM information_schema.processlist WHERE state='Sending to client' LIMIT 1")"
if [ -z "${BID}" ]; then echo "没有 Sending to client 的线程（客户端可能已读走结果），跳过该实验"; exit 3; fi
echo "blocked id=${BID}"
"${MYSQL[@]}" -e "KILL QUERY ${BID}"
sleep 2
"${MYSQL[@]}" -t -e "SELECT id, command, time, state, LEFT(info,36) AS info FROM information_schema.processlist WHERE id = ${BID}"

say "再执行 KILL CONNECTION（预期：线程消失）"
"${MYSQL[@]}" -e "KILL CONNECTION ${BID}"
sleep 1
"${MYSQL[@]}" -t -e "SELECT COUNT(*) AS still_there FROM information_schema.processlist WHERE id = ${BID}"

say "状态计数器（Com_kill=3：长事务 1 次 + 大查询 2 次；Questions/Queries 含监控连接自身语句）"
"${MYSQL[@]}" -t -e "SHOW GLOBAL STATUS WHERE Variable_name IN ('Com_kill','Com_select','Questions','Queries','Threads_running','Threads_connected','Connections','Aborted_clients')"

say "结束：清理后台会话与容器"
echo "done"
