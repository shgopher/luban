#!/usr/bin/env bash
# 44.28 MySQL 慢 SQL 案例集：一条命令起库 + 灌数 + 跑完全部实验
#
#   bash code/run.sh              # 默认端口 33199
#   PORT=33200 bash code/run.sh   # 换端口
#
# 前置：本机装了 Docker（macOS 上 docker 常在 /Applications/Docker.app/Contents/Resources/bin）；
#       本机装了 mysql 客户端（只用来连容器，不要求本机跑 MySQL 服务）。
set -uo pipefail

PORT="${PORT:-33199}"
NAME="mysqllab-$PORT"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MYSQL=(mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP)

export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST="${DOCKER_HOST:-unix://$HOME/.docker/run/docker.sock}"

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "== 1/4 起容器 $NAME（mysql:8.0，buffer pool 128M）=="
docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null

READY=0
for i in $(seq 1 90); do
  if "${MYSQL[@]}" -e "SELECT 1" >/dev/null 2>&1; then READY=1; break; fi
  sleep 1
done
if [ "$READY" -ne 1 ]; then
  echo "!! MySQL 未就绪（端口 $PORT 可能被占用，换 PORT= 再试）" >&2
  docker logs --tail 20 "$NAME" >&2 2>&1 || true
  exit 1
fi

echo "== 环境 =="
"${MYSQL[@]}" -t -e "SELECT VERSION() AS version, @@transaction_isolation AS isolation,
  @@innodb_page_size AS page_size, @@innodb_buffer_pool_size/1024/1024 AS bp_mb;"

echo "== 2/4 建表 + 灌数（4 张表 × 5 万行）=="
"${MYSQL[@]}" -t < "$HERE/00-schema.sql"

echo "== 3/4 案例 1~6 =="
for f in "$HERE"/10-exp1-implicit-cast.sql \
         "$HERE"/11-exp2-function.sql \
         "$HERE"/12-exp3-prefix-index.sql \
         "$HERE"/13-exp4-count.sql \
         "$HERE"/14-exp5-orderby.sql \
         "$HERE"/15-exp6-random.sql; do
  echo
  echo "---- $(basename "$f") ----"
  "${MYSQL[@]}" -t < "$f"
done

echo
echo "== 4/4 案例 7（三个并发连接：等 MDL、等行锁）=="
bash "$HERE/16-exp7-lock.sh" "$PORT"

echo
echo "== 收尾：容器 $NAME 将由 trap 删除 =="
