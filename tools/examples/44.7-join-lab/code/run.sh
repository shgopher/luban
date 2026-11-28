#!/usr/bin/env bash
# 一条命令复现 44.7「join 算法与 left join 改写」的全部实验。
# 依赖：Docker、mysql 客户端；默认端口 33199，可用 PORT=xxxx 覆盖。
set -euo pipefail

PORT=${PORT:-33199}
NAME="mysql-join-lab-$PORT"
ROOT=$(cd "$(dirname "$0")/.." && pwd)

export PATH=/Applications/Docker.app/Contents/Resources/bin:$PATH
export DOCKER_HOST=unix://${HOME}/.docker/run/docker.sock

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:$PORT:3306" mysql:8.0 \
  --innodb-buffer-pool-size=128M >/dev/null

for _ in $(seq 1 90); do
  if mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP -e 'select 1' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP -N -e \
  "select concat('version=', @@version, ' arch=', @@version_compile_machine, ' page=', @@innodb_page_size, ' bp=', @@innodb_buffer_pool_size/1024/1024, 'M isolation=', @@transaction_isolation)"

for f in 00-setup 10-join-plans 20-left-join 30-mrr-bka 40-timings; do
  echo "===== $f.sql ====="
  mysql -h127.0.0.1 -P"$PORT" -uroot --protocol=TCP -t < "$ROOT/code/$f.sql"
done
