#!/usr/bin/env bash
# 44.29 临时表、内部临时表与 Memory 引擎：一条命令复现正文全部实测
# 依赖：Docker（能拉 mysql:8.0）+ bash；不需要本机装 MySQL 客户端（SQL 在容器内执行）
# 用法：bash code/run.sh          # 默认端口 33199
#       PORT=33299 bash code/run.sh
set -euo pipefail
cd "$(dirname "$0")"

PORT=${PORT:-33199}
NAME="luban-4429-$PORT"
IMAGE=${IMAGE:-mysql:8.0}

# macOS 上 docker 常不在 PATH 里；Docker Desktop 的 socket 也要显式指一下
if ! command -v docker >/dev/null 2>&1; then
  [ -x /Applications/Docker.app/Contents/Resources/bin/docker ] && \
    export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
fi
if [ -z "${DOCKER_HOST:-}" ] && [ -S "$HOME/.docker/run/docker.sock" ]; then
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

sql()  { docker exec -i "$NAME" mysql -h127.0.0.1 -uroot "$@"; }
sqlf() { docker exec -i "$NAME" mysql -h127.0.0.1 -uroot -t "$@"; }

echo "== 起容器 $NAME（$IMAGE，端口 $PORT）"
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes \
  -p "127.0.0.1:$PORT:3306" \
  "$IMAGE" --innodb-buffer-pool-size=128M >/dev/null

wait_ready() {
  for _ in $(seq 1 90); do
    if sql -e "select 1" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  echo "MySQL 未就绪" >&2; exit 1
}
wait_ready
sqlf -e "select @@version as version, @@transaction_isolation as isolation,
                @@innodb_buffer_pool_size/1024/1024 as buffer_pool_mb,
                @@innodb_page_size as page_size"

echo
echo "== 01 造数（t1=1000 行；t_big=10 万行，用于把内部临时表撑到磁盘）"
sqlf < 01-setup.sql

echo
echo "== 02 用户临时表：只在创建它的 session 里可见，可与普通表重名，断开即消失"
sql -t < 02a-temp-table-session-a.sql > /tmp/4429-a.log 2>&1 &
A_PID=$!
sleep 2
sqlf --force < 02b-temp-table-session-b.sql 2>&1 || true
wait "$A_PID"
echo "--- 会话 A 自己的输出"
cat /tmp/4429-a.log
echo "--- A 断开后：服务端临时表数量归零（DROP TEMPORARY TABLE 由服务端在会话结束时执行）"
sqlf -e "select count(*) as temp_tables_left from information_schema.innodb_temp_table_info"

echo
echo "== 02c 临时表与 binlog：ROW 格式不记，STATEMENT 格式才记"
sqlf < 02c-binlog-temp.sql
BINLOG=$(sql -N -e "SHOW MASTER STATUS" | cut -f1)
echo "--- $BINLOG 里的临时表相关事件（STATEMENT 格式才有）"
sql -t -e "SHOW BINLOG EVENTS IN '$BINLOG'" | grep -Ei "temporary|temp_t|t_normal_binlog" || true

echo
echo "== 03 内部临时表：UNION / GROUP BY / order by null / SQL_BIG_RESULT"
sqlf < 03-internal-temp.sql

echo
echo "== 04 内部临时表落盘：tmp_table_size、max_heap_table_size、temptable_max_ram"
sqlf --force < 04-internal-tmp-disk.sql 2>&1 || true

echo "--- 临时文件都落在哪"
sqlf -e "SELECT @@tmpdir AS tmpdir, @@innodb_temp_data_file_path AS ibtmp"
docker exec -i "$NAME" sh -c 'du -h /var/lib/mysql/ibtmp1 2>/dev/null; ls /var/lib/mysql/\#innodb_temp 2>/dev/null | wc -l | sed "s/^/会话临时表空间文件数: /"'

echo
echo "== 05 Memory 引擎：堆组织表、hash 索引、varchar 定长、表满"
sqlf --force < 05-memory-engine.sql 2>&1 || true

echo
echo "== 06 哪些 SQL 在建临时表：performance_schema 按 digest 聚合"
sqlf < 09-digest-tmp.sql

echo
echo "== 07 案例：10 万行 GROUP BY 从 750ms 到 187ms"
sqlf < 08-case-groupby.sql

echo
echo "== 07b 微基准：临时表选哪个引擎 + Memory/InnoDB 的等值查询"
sqlf --force < 10-bench.sql 2>&1 || true

echo
echo "== 08 Memory 表锁 vs InnoDB 行锁（A 的 update 本身跑 5 秒，B 同时更新另一行）"
sql -e "USE d; UPDATE t_mem SET c=c+1 WHERE id=1 AND sleep(5)=0" >/dev/null 2>&1 &
A_PID=$!
sleep 1
sqlf < 07-lock-memory.sql
wait "$A_PID"
sql -e "USE d; UPDATE t_inn SET c=c+1 WHERE id=1 AND sleep(5)=0" >/dev/null 2>&1 &
A_PID=$!
sleep 1
sqlf < 07-lock-innodb.sql
wait "$A_PID"

echo
echo "== 09 重启容器：Memory 表数据清空、表结构还在；binlog 里多一条清空语句"
docker restart "$NAME" >/dev/null
wait_ready
sqlf < 06-memory-after-restart.sql
BINLOG=$(sql -N -e "SHOW MASTER STATUS" | cut -f1)
echo "--- $BINLOG 里与内存表相关的语句"
sql -t -e "SHOW BINLOG EVENTS IN '$BINLOG'" | grep -Ei "implicitly|t_mem" || true

echo
echo "== 完成：容器 $NAME 由 trap 删除"
