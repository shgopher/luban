#!/bin/bash
set -u
REPO=/Users/shgopher/Desktop/family/luban
BIN=$REPO/.cache/ch66/bin
A=$REPO/.tmp-audit-rerun/s661
export GOMODCACHE=$REPO/.cache/ch66/gomodcache GOCACHE=$REPO/.cache/gocache GOBIN=$BIN GOFLAGS=-mod=mod GOWORK=off

stop_all() {
  for p in gw1 gw2; do
    [ -f "$A/$p.pid" ] && kill "$(cat "$A/$p.pid")" 2>/dev/null; rm -f "$A/$p.pid"
  done
  sleep 0.5
}
start_gw() {
  local id=$1 port=$2; shift 2
  nohup "$BIN/gw661" -id "$id" -addr "127.0.0.1:$port" -redis 127.0.0.1:16381 "$@" > "$A/$id.log" 2>&1 &
  echo $! > "$A/$id.pid"
  for _ in $(seq 1 50); do curl -s -o /dev/null "http://127.0.0.1:$port/metrics" && return 0; sleep 0.1; done
  echo "gw $id start failed"; tail -5 "$A/$id.log"; return 1
}

echo "=== load: $(sysctl -n vm.loadavg) ; date: $(date '+%F %T') ==="
# fresh redis under our control
redis-cli -p 16381 ping >/dev/null 2>&1 && redis-cli -p 16381 shutdown nosave >/dev/null 2>&1
sleep 0.3
redis-server --port 16381 --save '' --appendonly no --daemonize yes --logfile "$A/redis-16381.log" --dir "$A"
sleep 0.5
echo "redis: $(redis-cli -p 16381 ping)"

stop_all
redis-cli -p 16381 FLUSHALL >/dev/null
echo "########## B1 route-cache=on ##########"
echo "=== gw2 route-cache=on $(date '+%F %T') load=$(sysctl -n vm.loadavg) ==="
start_gw gw1 18081 -route-cache=true || exit 1
start_gw gw2 18082 -route-cache=true || exit 1
"$BIN/exp661" -mode=b -n 1000 -gw1 127.0.0.1:18081 -gw2 127.0.0.1:18082 -redis 127.0.0.1:16381 \
  -desc 'gw2 route-cache=on cache-ttl=2s route-ttl=60s'
echo "--- redis check (gw still up) ---"
redis-cli -p 16381 --scan --pattern 'im:route:*' | head -5
echo -n "im:route:* entries: "; redis-cli -p 16381 --scan --pattern 'im:route:*' | wc -l
stop_all
sleep 1
redis-cli -p 16381 FLUSHALL >/dev/null
echo "########## B2 route-cache=off ##########"
echo "=== gw2 route-cache=off $(date '+%F %T') load=$(sysctl -n vm.loadavg) ==="
start_gw gw1 18081 -route-cache=true || exit 1
start_gw gw2 18082 -route-cache=false || exit 1
"$BIN/exp661" -mode=b -n 1000 -gw1 127.0.0.1:18081 -gw2 127.0.0.1:18082 -redis 127.0.0.1:16381 \
  -desc 'gw2 route-cache=off (per-message Redis GET)'
stop_all
echo "B DONE"
