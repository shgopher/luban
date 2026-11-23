#!/bin/bash
# 实验 E：优雅下线（drain + redirect + 客户端重连）+ 实验 F：心跳与「假在线」检测。
set -u
cd "$(dirname "$0")/.."
source scripts/lib.sh
RAW=$S/raw
stop_all_gw
redis-cli -p 16381 FLUSHALL > /dev/null
sleep 2

start_gw gw1 18081 || exit 1
start_gw gw2 18082 || exit 1
echo "=== 优雅下线 drain $(date '+%F %T') ===" | tee "$RAW/e-drain.txt"
"$BIN/exp661" -mode=e -n 500 -gw1 127.0.0.1:18081 -gw2 127.0.0.1:18082 -redis 127.0.0.1:16381 \
  2>&1 | tee -a "$RAW/e-drain.txt"
stop_all_gw
sleep 3
redis-cli -p 16381 FLUSHALL > /dev/null

start_gw gw1 18081 -heartbeat 1s -hb-ack 2s || exit 1
echo "=== 心跳/假在线检测 $(date '+%F %T') ===" | tee "$RAW/f-heartbeat.txt"
"$BIN/exp661" -mode=f -gw1 127.0.0.1:18081 -redis 127.0.0.1:16381 \
  2>&1 | tee -a "$RAW/f-heartbeat.txt"
stop_all_gw
echo "E/F DONE"
