#!/bin/bash
# 实验 C：重连风暴。
# 1000 个客户端先连上网关，然后由服务端统一断开（等价于网关重启 / 网络分区），
# 再按两种退避策略同时重连；对比服务端每秒 accept 数与拒绝数。
# 第三组在断开后把网关建连限速设成 500/s，看限速如何把尖峰摊平。
set -u
cd "$(dirname "$0")/.."
source scripts/lib.sh
RAW=$S/raw

run_one() { # run_one <标签> <storm-mode> <set-limit>
  local label=$1 mode=$2 lim=$3
  stop_gw gw1
  sleep 20   # 等上一轮的 TIME_WAIT 临时端口回收（本机 16384 个，被并行实验共享）
  redis-cli -p 16381 FLUSHALL > /dev/null
  start_gw gw1 18081 || exit 1   # 首连不限速，限速在断开后由 exp 通过 /admin/limit 打开
  echo "=== ${label} storm_mode=${mode} set-limit=${lim} $(date '+%F %T') load=$(sysctl -n vm.loadavg) ===" | tee "$RAW/c-${label}.txt"
  "$BIN/exp661" -mode=c -n 1000 -gw1 127.0.0.1:18081 -redis 127.0.0.1:16381 \
    -storm-mode "$mode" -set-limit "$lim" -wait 12 2>&1 | tee -a "$RAW/c-${label}.txt"
  stop_gw gw1
  sleep 20   # 等上一轮的 TIME_WAIT 临时端口回收（本机 16384 个，被并行实验共享）
}

run_one fixed fixed 0
run_one backoff backoff 0
run_one backoff-limited backoff 500
echo "C DONE"
