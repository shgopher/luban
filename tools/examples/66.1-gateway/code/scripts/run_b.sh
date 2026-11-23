#!/bin/bash
# 实验 B：跨网关卡投递。
# gw1/gw2 两个网关进程 + 一台 Redis（16381）。客户端连 gw1，业务侧把下行 push
# 打到 gw2 —— gw2 上没有这条连接，必须查路由表才知道消息该转发到 gw1。
set -u
cd "$(dirname "$0")/.."
source scripts/lib.sh
RAW=$S/raw
stop_all_gw
redis-cli -p 16381 FLUSHALL > /dev/null
sleep 2

# 对照 1：路由缓存开（默认）
start_gw gw1 18081 -route-cache=true || exit 1
start_gw gw2 18082 -route-cache=true || exit 1
echo "=== gw2 route-cache=on $(date '+%F %T') load=$(sysctl -n vm.loadavg) ===" | tee "$RAW/b-route-cache-on.txt"
"$BIN/exp661" -mode=b -n 1000 -gw1 127.0.0.1:18081 -gw2 127.0.0.1:18082 -redis 127.0.0.1:16381 \
  -desc 'gw2 route-cache=on cache-ttl=2s route-ttl=60s' \
  2>&1 | tee -a "$RAW/b-route-cache-on.txt"
echo "--- redis 侧核对 ---" | tee -a "$RAW/b-route-cache-on.txt"
UID1=$(grep '^ROUTE_UID' "$RAW/b-route-cache-on.txt" | awk '{print $2}')
echo "im:route:$UID1 -> $(redis-cli -p 16381 GET "im:route:$UID1")" | tee -a "$RAW/b-route-cache-on.txt"
echo -n "im:route:* 条目数: " | tee -a "$RAW/b-route-cache-on.txt"
redis-cli -p 16381 --scan --pattern 'im:route:*' | wc -l | tee -a "$RAW/b-route-cache-on.txt"
stop_all_gw
sleep 3
redis-cli -p 16381 FLUSHALL > /dev/null

# 对照 2：路由缓存关（每条下行消息都查一次 Redis）
start_gw gw1 18081 -route-cache=true || exit 1
start_gw gw2 18082 -route-cache=false || exit 1
echo "=== gw2 route-cache=off $(date '+%F %T') load=$(sysctl -n vm.loadavg) ===" | tee "$RAW/b-route-cache-off.txt"
"$BIN/exp661" -mode=b -n 1000 -gw1 127.0.0.1:18081 -gw2 127.0.0.1:18082 -redis 127.0.0.1:16381 \
  -desc 'gw2 route-cache=off（每条消息一次 Redis GET）' \
  2>&1 | tee -a "$RAW/b-route-cache-off.txt"
stop_all_gw
echo "B DONE"
