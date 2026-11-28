#!/usr/bin/env bash
# 44.25 配套实验：自增主键空洞、批量预分配、自增 ID 到顶、权限表一致性
# 依赖：docker、mysql 客户端。默认端口 33199，可用 PORT=34199 覆盖。
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
PORT=${PORT:-33199}
NAME=${NAME:-mysqllab-4425-example}
export PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"
export DOCKER_HOST=${DOCKER_HOST:-unix://$HOME/.docker/run/docker.sock}
MYSQL=(mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP)

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# 端口可能被上一次残留占用，先清掉同名容器
docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "== 启动 MySQL 8.0（端口 $PORT，首次初始化约 1–3 分钟）"
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M --max-connections=200 >/dev/null

for _ in $(seq 1 150); do
  "${MYSQL[@]}" -N -e 'select 1' >/dev/null 2>&1 && break
  sleep 2
done

if ! "${MYSQL[@]}" -N -e 'select 1' >/dev/null 2>&1; then
  echo "MySQL 未就绪，容器日志尾部：" >&2
  docker logs "$NAME" 2>&1 | tail -20 >&2
  exit 1
fi

echo
echo "== 环境"
"${MYSQL[@]}" -t -e "select version() as version,
  @@innodb_buffer_pool_size/1024/1024 as bp_mb,
  @@transaction_isolation as iso,
  @@binlog_format as binlog_fmt,
  @@innodb_autoinc_lock_mode as autoinc_mode;"

echo
echo "== 1. 自增空洞、批量分配、int unsigned 到顶（故意报 2 个错）"
"${MYSQL[@]}" --force -t < "$HERE/autoinc.sql" || true

echo
echo "== 2. grant 与 flush privileges（只改 mysql.user 的磁盘行才会不一致）"
"${MYSQL[@]}" --force -t < "$HERE/permission.sql" || true

echo
echo "== 完成：容器已由 trap 清理"
