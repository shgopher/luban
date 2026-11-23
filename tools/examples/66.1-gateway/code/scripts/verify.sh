#!/bin/bash
# 可验：把正文「生产落地」里要粘贴的命令原样跑一遍，输出即正文引用。
# 结果：raw/g-verify.txt
set -u
cd "$(dirname "$0")/.."
source scripts/lib.sh
RAW=$S/raw
OUT=$RAW/g-verify.txt
stop_all_gw
redis-cli -p 16381 FLUSHALL > /dev/null
sleep 1

start_gw gw1 18081 || exit 1
start_gw gw2 18082 || exit 1

{
  echo "# 环境: $(date '+%F %T') macOS 15.6 arm64 / go1.27.1 / redis 8.10.2 / gorilla websocket v1.5.3"
  echo
  echo "### 1) 登录服务发一张一次性票据（真实系统由 49.5 的登录态服务写入）"
  echo '$ redis-cli -p 16381 SET im:ticket:demo-1 u100 PX 60000'
  redis-cli -p 16381 SET im:ticket:demo-1 u100 PX 60000
  echo
  echo "### 2) 客户端带票建连（票据被 GETDEL 原子核销，无法重放）"
  echo '$ wsclient -url ws://127.0.0.1:18081/ws?ticket=demo-1 -hold 6s &'
  "$BIN/wsclient" -url "ws://127.0.0.1:18081/ws?ticket=demo-1" -hold 6s > /tmp/verify_ws1.txt 2>&1 &
  WSPID=$!
  sleep 1
  cat /tmp/verify_ws1.txt
  echo
  echo "### 3) 网关把这条连接登记进路由表"
  echo "\$ redis-cli -p 16381 KEYS 'im:route:*'"
  redis-cli -p 16381 KEYS 'im:route:*'
  echo "\$ redis-cli -p 16381 GET im:route:u100"
  redis-cli -p 16381 GET im:route:u100
  echo
  echo "### 4) 业务侧把下行 push 打到另一台网关 gw2（gw2 上没有这条连接）"
  echo '$ curl -s -XPOST http://127.0.0.1:18082/api/send -d '"'"'{"uid":"u100","seq":7,"body":"hello"}'"'"
  curl -s -XPOST http://127.0.0.1:18082/api/send -d '{"uid":"u100","seq":7,"body":"hello"}'
  echo
  sleep 0.5
  echo "---- 客户端侧收到的帧 ----"
  cat /tmp/verify_ws1.txt
  echo
  echo "### 5) 网关指标（在线数、投递数、路由查询数）"
  echo '$ curl -s http://127.0.0.1:18081/metrics | grep -E "online_conns|accepted_total|delivered|route_lookup|route_cache_hit"'
  curl -s http://127.0.0.1:18081/metrics | grep -E "online_conns|accepted_total|delivered|route_lookup|route_cache_hit"
  echo
  echo "### 6) 失败路径：同一张一次性票再连一次"
  echo '$ wsclient -url ws://127.0.0.1:18081/ws?ticket=demo-1'
  "$BIN/wsclient" -url "ws://127.0.0.1:18081/ws?ticket=demo-1" -hold 1s
  echo
  echo "### 7) 失败路径：用户不在线"
  echo '$ curl -s -XPOST http://127.0.0.1:18082/api/send -d '"'"'{"uid":"offline-user","seq":1,"body":"x"}'"'"
  curl -s -XPOST http://127.0.0.1:18082/api/send -d '{"uid":"offline-user","seq":1,"body":"x"}'
  echo
  echo "### 8) 失败路径：建连限速（rate=1/s burst=1），并发 3 个新连接"
  echo '$ curl -s "http://127.0.0.1:18081/admin/limit?rate=1&burst=1"'
  curl -s "http://127.0.0.1:18081/admin/limit?rate=1&burst=1"
  for i in 1 2 3; do
    redis-cli -p 16381 SET "im:ticket:rl-$i" "u-rl-$i" PX 60000 > /dev/null
    echo -n "  conn$i: "
    "$BIN/wsclient" -url "ws://127.0.0.1:18081/ws?ticket=rl-$i" -hold 200ms 2>&1 | head -1
  done
  echo '$ curl -s http://127.0.0.1:18081/metrics | grep rejected'
  curl -s http://127.0.0.1:18081/metrics | grep rejected
  echo "  （限速开关关掉，恢复）"
  curl -s "http://127.0.0.1:18081/admin/limit?rate=0&burst=0" > /dev/null
  echo
  echo "### 9) 优雅下线：摘流量 -> 删路由 -> redirect -> 排空"
  echo '$ curl -s -XPOST http://127.0.0.1:18081/admin/drain'
  curl -s -XPOST http://127.0.0.1:18081/admin/drain
  echo
  wait $WSPID 2>/dev/null
  echo "---- 客户端侧（应看到 redirect 帧） ----"
  cat /tmp/verify_ws1.txt
  echo
  echo '$ curl -s http://127.0.0.1:18081/metrics | grep -E "online_conns|draining"'
  curl -s http://127.0.0.1:18081/metrics | grep -E "online_conns|draining"
  echo '$ redis-cli -p 16381 KEYS "im:route:*"'
  redis-cli -p 16381 KEYS 'im:route:*'
} 2>&1 | tee "$OUT"

stop_all_gw
echo "VERIFY DONE -> $OUT"
