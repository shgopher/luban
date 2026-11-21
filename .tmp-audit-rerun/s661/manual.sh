#!/bin/bash
set -u
REPO=/Users/shgopher/Desktop/family/luban
BIN=$REPO/.cache/ch66/bin
A=$REPO/.tmp-audit-rerun/s661
stop_all() { for p in gw1 gw2; do [ -f "$A/$p.pid" ] && kill "$(cat "$A/$p.pid")" 2>/dev/null; rm -f "$A/$p.pid"; done; sleep 0.4; }
start_gw() {
  local id=$1 port=$2; shift 2
  nohup "$BIN/gw661" -id "$id" -addr "127.0.0.1:$port" -redis 127.0.0.1:16381 "$@" > "$A/$id.log" 2>&1 &
  echo $! > "$A/$id.pid"
  for _ in $(seq 1 50); do curl -s -o /dev/null "http://127.0.0.1:$port/metrics" && return 0; sleep 0.1; done
  echo "gw $id start failed"; return 1
}
stop_all
redis-cli -p 16381 FLUSHALL >/dev/null
redis-server --port 16381 --save '' --appendonly no --daemonize yes --logfile "$A/redis-16381.log" --dir "$A" 2>/dev/null || true
sleep 0.3
start_gw gw1 18081 || exit 1
start_gw gw2 18082 || exit 1

echo '### 1) 一次性票据 + WS 连接 + 跨网关下行（正文 可验 段原样命令）'
redis-cli -p 16381 SET im:ticket:demo-1 u100 PX 60000
$BIN/wsclient661 -url 'ws://127.0.0.1:18081/ws?ticket=demo-1' -hold 8s > "$A/wsclient.out" 2>&1 &
WSPID=$!
for _ in $(seq 1 30); do grep -q CONNECTED "$A/wsclient.out" && break; sleep 0.1; done
cat "$A/wsclient.out"
sleep 0.3
echo '--- KEYS/GET route ---'
redis-cli -p 16381 KEYS 'im:route:*'
redis-cli -p 16381 GET im:route:u100
echo '--- POST gw2 /api/send ---'
curl -s -XPOST http://127.0.0.1:18082/api/send -d '{"uid":"u100","seq":7,"body":"hello"}'; echo
sleep 0.4
echo '--- client frames ---'; cat "$A/wsclient.out"
echo '--- gw1 metrics ---'
curl -s http://127.0.0.1:18081/metrics | grep -E 'online_conns|push_delivered|route_lookup'
echo '--- gw2 metrics ---'
curl -s http://127.0.0.1:18082/metrics | grep -E 'online_conns|push_delivered|route_lookup|route_stale'
wait $WSPID 2>/dev/null
echo '### 2) 同一张票再连 → 401'
$BIN/wsclient661 -url 'ws://127.0.0.1:18081/ws?ticket=demo-1' -hold 1s 2>&1 || echo "(exit $?)"

echo '### 3) 建连限速 rate=1 burst=1 → 429'
curl -s -XPOST 'http://127.0.0.1:18082/admin/limit?rate=1&burst=1'; echo
redis-cli -p 16381 SET im:ticket:r1 u201 PX 60000 >/dev/null
redis-cli -p 16381 SET im:ticket:r2 u202 PX 60000 >/dev/null
redis-cli -p 16381 SET im:ticket:r3 u203 PX 60000 >/dev/null
$BIN/wsclient661 -url 'ws://127.0.0.1:18082/ws?ticket=r1' -hold 1s >/dev/null 2>&1 &
$BIN/wsclient661 -url 'ws://127.0.0.1:18082/ws?ticket=r2' -hold 1s > "$A/limit2.out" 2>&1 &
$BIN/wsclient661 -url 'ws://127.0.0.1:18082/ws?ticket=r3' -hold 1s > "$A/limit3.out" 2>&1 &
sleep 2
cat "$A/limit2.out" "$A/limit3.out"
curl -s http://127.0.0.1:18082/metrics | grep -E 'rejected'
echo '### 4) drain（另起一条连接）'
redis-cli -p 16381 SET im:ticket:d1 u301 PX 60000 >/dev/null
$BIN/wsclient661 -url 'ws://127.0.0.1:18082/ws?ticket=d1' -hold 5s > "$A/drainclient.out" 2>&1 &
sleep 0.8
curl -s -XPOST http://127.0.0.1:18081/admin/drain; echo
sleep 0.5
cat "$A/drainclient.out"
curl -s http://127.0.0.1:18082/metrics | grep -E 'online_conns|draining'
stop_all
echo MANUAL_DONE
