#!/bin/bash
# 实验 A：单机 WebSocket 连接成本（同一台机器跑服务端 + 客户端）。
# 前半段：只改服务端 websocket 读写缓冲，看每连接内存差多少。
# 后半段：把网关进程的 fd 上限压到 2000，验证「单机连接数先撞 fd」。
set -u
cd "$(dirname "$0")/.."
source scripts/lib.sh
RAW=$S/raw
stop_all_gw
redis-cli -p 16381 FLUSHALL > /dev/null

{
  echo "=== fd 上限对照 $(date '+%F %T') ==="
  echo "shell ulimit -n : $(ulimit -n)"
  echo "shell ulimit -Hn: $(ulimit -Hn)"
  sysctl kern.maxfiles kern.maxfilesperproc | sed 's/^/sysctl /'
} | tee "$RAW/a-fd-ceiling.txt"

for cfg in "4096:4KB" "65536:64KB" "4096:4KB-repeat"; do
  rb=${cfg%%:*}; label=${cfg##*:}
  stop_gw gw1
  sleep 3
  start_gw gw1 18081 -readbuf "$rb" -writebuf "$rb" || exit 1
  echo "=== readbuf=${label} n=2000 $(date '+%F %T') ===" | tee "$RAW/a-conncost-${label}.txt"
  "$BIN/exp661" -mode=a -n 2000 -gw1 127.0.0.1:18081 -redis 127.0.0.1:16381 \
    2>&1 | tee -a "$RAW/a-conncost-${label}.txt"
  grep -E '^FD_CEILING' "$RAW/a-conncost-${label}.txt" | tee -a "$RAW/a-fd-ceiling.txt"
  stop_gw gw1
  sleep 3
done

# fd 上限实验：把网关进程自己的 nofile 压到 2000，再让它接 3000 条连接。
stop_gw gw1
sleep 2
start_gw_ulimit gw1 18081 2000 -readbuf 4096 -writebuf 4096 || exit 1
echo "=== gateway ulimit -n 2000, n=3000 $(date '+%F %T') ===" | tee "$RAW/a-conncost-fdlimit.txt"
"$BIN/exp661" -mode=a -n 3000 -gw1 127.0.0.1:18081 -redis 127.0.0.1:16381 \
  2>&1 | tee -a "$RAW/a-conncost-fdlimit.txt"
echo "--- gateway log ---" | tee -a "$RAW/a-conncost-fdlimit.txt"
echo -n "too many open files 行数: " | tee -a "$RAW/a-conncost-fdlimit.txt"
grep -c 'too many open files' "$S/logs/gw1.log" | tee -a "$RAW/a-conncost-fdlimit.txt"
grep 'too many open files' "$S/logs/gw1.log" | head -2 | tee -a "$RAW/a-conncost-fdlimit.txt"
stop_gw gw1
echo "A DONE"
