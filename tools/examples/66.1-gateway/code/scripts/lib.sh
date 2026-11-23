#!/bin/bash
# 实验编排公共函数：后台起停网关，日志与 pid 都落在 .cache/ch66/s661/ 下。
set -u
S=/Users/shgopher/Desktop/family/luban/.cache/ch66/s661
BIN=/Users/shgopher/Desktop/family/luban/.cache/ch66/bin
REDIS_PORT=16381

start_gw() { # start_gw <id> <port> [额外参数...]
  local id=$1 port=$2; shift 2
  nohup "$BIN/gw661" -id "$id" -addr "127.0.0.1:$port" -redis "127.0.0.1:$REDIS_PORT" "$@" \
    > "$S/logs/$id.log" 2>&1 &
  echo $! > "$S/logs/$id.pid"
  for _ in $(seq 1 50); do
    if curl -s -o /dev/null "http://127.0.0.1:$port/metrics"; then return 0; fi
    sleep 0.1
  done
  echo "gw $id failed to start" >&2; tail -5 "$S/logs/$id.log" >&2; return 1
}

stop_gw() { # stop_gw <id>
  local f="$S/logs/$1.pid"
  [ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null
  rm -f "$f"
  sleep 0.3
}

stop_all_gw() { stop_gw gw1; stop_gw gw2; }

start_gw_ulimit() { # start_gw_ulimit <id> <port> <nofile> [额外参数...]
  local id=$1 port=$2 lim=$3; shift 3
  nohup bash -c "ulimit -n $lim; exec '$BIN/gw661' -id '$id' -addr '127.0.0.1:$port' -redis '127.0.0.1:$REDIS_PORT' $*" \
    > "$S/logs/$id.log" 2>&1 &
  echo $! > "$S/logs/$id.pid"
  for _ in $(seq 1 50); do
    if curl -s -o /dev/null "http://127.0.0.1:$port/metrics"; then return 0; fi
    sleep 0.1
  done
  echo "gw $id failed to start" >&2; tail -5 "$S/logs/$id.log" >&2; return 1
}
