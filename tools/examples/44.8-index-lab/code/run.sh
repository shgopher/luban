#!/usr/bin/env bash
# 44.8 示例：索引维护 / 覆盖索引与 ICP / ORDER BY 走索引 / change buffer
# 一条命令起 MySQL 8.0 → 灌数 → 跑四组实验 → 删容器
#   bash code/run.sh              # 默认端口 33199
#   PORT=34199 bash code/run.sh   # 端口被占时换一个
#   SKIP_CB=1 bash code/run.sh    # 跳过耗时约 2 分钟的 change buffer 实验
set -euo pipefail

PORT="${PORT:-33199}"
NAME="mysqlex-4408-${PORT}"
IMG="${IMG:-mysql:8.0}"

if ! command -v docker >/dev/null 2>&1; then
  export PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== 0. 起容器 ($NAME, 端口 $PORT) =="
cleanup
docker run -d --rm --name "$NAME" \
  -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -e MYSQL_ROOT_HOST=% \
  -p "127.0.0.1:${PORT}:3306" "$IMG" \
  --innodb-buffer-pool-size=128M >/dev/null

# 连续两次探活成功才算就绪：镜像初始化阶段会先起一个临时实例
ok=0
for _ in $(seq 1 120); do
  if mysql -h127.0.0.1 -P"${PORT}" -uroot --protocol=TCP -e "SELECT @@version" >/dev/null 2>&1; then
    ok=$((ok + 1))
    [ "$ok" -ge 2 ] && break
  else
    ok=0
  fi
  sleep 1
done

M="mysql -h127.0.0.1 -P${PORT} -uroot --protocol=TCP"

echo
echo "== 1. 环境 =="
$M -t -e "SELECT @@version AS version, @@innodb_page_size AS page_size,
                 @@innodb_buffer_pool_size/1024/1024 AS bp_mb,
                 @@transaction_isolation AS isolation;"

echo
echo "== 2. 递增插入 vs 随机插入：叶子页数（seq_t / rand_t 各 5 万行）=="
$M -t < code/01-page-split.sql

echo
echo "== 3. 覆盖索引 vs 回表、ICP 开 vs 关 =="
$M -t < code/02-covering-icp.sql

echo
echo "== 4. ORDER BY 的三种结局 =="
$M -t < code/03-order-by.sql

if [ "${SKIP_CB:-0}" = "1" ]; then
  echo
  echo "== 5. change buffer：已跳过（SKIP_CB=1）=="
else
  echo
  echo "== 5. change buffer：60 万行随机二级索引键，观察 ibuf（约 2 分钟）=="
  $M -t < code/04-change-buffer.sql
  echo "-- SHOW ENGINE INNODB STATUS 的 change buffer 段 --"
  $M -E -e "SHOW ENGINE INNODB STATUS" | grep -A5 "INSERT BUFFER AND ADAPTIVE HASH INDEX" || true
fi

echo
echo "== 6. 收尾 =="
echo "容器 $NAME 会被 trap 删除；如需保留调试：注释掉 run.sh 里的 trap cleanup EXIT"
