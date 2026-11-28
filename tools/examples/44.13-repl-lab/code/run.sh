#!/usr/bin/env bash
# 44.13 复制实验：一条命令起 MySQL 8.0，量 binlog、量探针、验只读，跑完删容器。
# 用法：bash code/run.sh                    # 默认端口 33199，自己起容器
#       PORT=33202 bash code/run.sh        # 端口被占时换一个
#       SKIP_START=1 PORT=33199 bash code/run.sh  # 复用已就绪的实例，不新建容器
set -uo pipefail

PORT=${PORT:-33199}
NAME="mysqllab-repl-$PORT"
DIR="$(cd "$(dirname "$0")" && pwd)"
M() { mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP "$@"; }

export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=${DOCKER_HOST:-unix://$HOME/.docker/run/docker.sock}

cleanup() { [[ "${SKIP_START:-0}" == "1" ]] || docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

if [[ "${SKIP_START:-0}" == "1" ]]; then
  echo "== 复用已在 $PORT 上就绪的实例（SKIP_START=1，不新建容器）=="
else
  echo "== 起容器 $NAME（mysql:8.0，buffer pool 128M）=="
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker run -d --rm --name "$NAME" \
    -e MYSQL_ALLOW_EMPTY_PASSWORD=yes \
    -p "127.0.0.1:$PORT:3306" mysql:8.0 \
    --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null
fi
# 首次启动要 initialize 数据目录，机器负载高时可达 2~3 分钟，等满 300 秒
for _ in $(seq 1 300); do
  if M -e "select 1" >/dev/null 2>&1; then break; fi
  sleep 1
done
if ! M -e "select 1" >/dev/null 2>&1; then
  echo "容器 300 秒内未就绪，docker logs 末尾：" >&2
  docker logs --tail 5 "$NAME" >&2
  exit 1
fi

echo "-- 环境 --"
M -t -e "SELECT VERSION() AS ver, @@transaction_isolation AS iso, @@binlog_format AS fmt, @@innodb_page_size AS page, @@innodb_buffer_pool_size/1024/1024 AS bp_mb;"

echo
echo "########## 1. 并行复制与依赖跟踪的默认值 ##########"
M -t -e "SHOW VARIABLES LIKE 'slave_parallel%'; SHOW VARIABLES LIKE 'replica_parallel%'; SHOW VARIABLES LIKE 'binlog_transaction_dependency_tracking';"

echo
echo "########## 2. 半同步模块：默认没装 ##########"
echo "-- 装之前 --"
M -t -e "SHOW VARIABLES LIKE 'rpl_semi_sync%';"
M -e "INSTALL PLUGIN rpl_semi_sync_source SONAME 'semisync_source.so'; INSTALL PLUGIN rpl_semi_sync_replica SONAME 'semisync_replica.so';"
echo "-- 装之后 --"
M -t -e "SHOW VARIABLES LIKE 'rpl_semi_sync_source%';"

echo
echo "########## 3. 位点与 GTID：CHANGE MASTER TO 要填的东西 ##########"
M -t -e "SHOW MASTER STATUS; SHOW BINARY LOGS; SELECT @@gtid_mode AS gtid_mode, CONCAT('[', @@gtid_executed, ']') AS gtid_executed;"

echo
echo "########## 4. 同一个 50,000 行 UPDATE，三种格式的 binlog 体积 ##########"
M < "$DIR/schema.sql" >/dev/null
M -e "FLUSH BINARY LOGS;"
size() { M -N -B -e "SHOW BINARY LOGS" | tail -1 | awk '{print $2}'; }
run_case() {
  local tag=$1 setup=$2 sql=$3
  local before after t0 t1 out
  before=$(size)
  t0=$(python3 -c 'import time;print(time.time())')
  out=$(M -N -B -e "$setup USE repl_lab; $sql SELECT ROW_COUNT() AS rows_affected; SHOW WARNINGS;" 2>&1 | tr '\n' ' ')
  t1=$(python3 -c 'import time;print(time.time())')
  after=$(size)
  echo "-- [$tag] binlog 增长 $((after - before)) 字节，语句耗时 $(python3 -c "print(f'{$t1-$t0:.2f}s')")"
  echo "   $out" | cut -c1-400
}
run_case "ROW + binlog_row_image=FULL" "" "UPDATE big SET c=c+1 ORDER BY id LIMIT 50000;"
run_case "ROW + binlog_row_image=MINIMAL" "SET SESSION binlog_row_image=MINIMAL;" "UPDATE big SET c=c+1 ORDER BY id DESC LIMIT 50000;"
run_case "STATEMENT" "SET SESSION binlog_format=STATEMENT;" "UPDATE big SET c=c+1 ORDER BY id LIMIT 50000;"

echo
echo "########## 5. 探针代价：select 1 vs 查真实表（performance_schema 计时）##########"
M -t -e "
TRUNCATE performance_schema.events_statements_history;
SELECT 1 AS probe;
SELECT COUNT(*) FROM repl_lab.big;
SELECT SQL_TEXT, ROUND(TIMER_WAIT / 1000000000, 3) AS ms, ROWS_SENT
FROM performance_schema.events_statements_history
WHERE SQL_TEXT IS NOT NULL ORDER BY TIMER_START DESC LIMIT 4;"

echo
echo "########## 6. 只读语义：read_only 与 super_read_only ##########"
M -e "CREATE USER IF NOT EXISTS app@'%' IDENTIFIED BY 'app'; GRANT SELECT,INSERT ON repl_lab.* TO app@'%'; INSERT IGNORE INTO repl_lab.big VALUES (900001,1,'x');"
M -e "SET GLOBAL read_only=ON;"
echo "-- 普通账号（read_only=ON）--"
mysql -h127.0.0.1 -P"$PORT" -uapp -papp --protocol=TCP -e "INSERT INTO repl_lab.big VALUES (900002,2,'x')" 2>&1 | grep -v Warning
echo "-- root 即 SUPER（read_only=ON）--"
M -e "INSERT INTO repl_lab.big VALUES (900003,3,'x'); SELECT ROW_COUNT() AS inserted;"
M -e "SET GLOBAL read_only=OFF; SET GLOBAL super_read_only=ON;"
echo "-- root 即 SUPER（super_read_only=ON）--"
M -e "INSERT INTO repl_lab.big VALUES (900004,4,'x')" 2>&1
M -e "SET GLOBAL super_read_only=OFF; DELETE FROM repl_lab.big WHERE id>=900001;"

echo
echo "########## 7. 单实例上没有复制关系 ##########"
M -t -e "SHOW REPLICA STATUS;" ; echo "（0 行：本机是单实例，多机才有的字段都不存在）"

echo
echo "== 清理：容器 $NAME 删除 =="
