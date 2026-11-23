#!/bin/bash
# 实验流水：串行执行全部实验，原始输出落 raw/
set -u
cd "$(dirname "$0")"
BIN=./s665
RAW=../raw
mkdir -p "$RAW"
ts() { date -u +%H:%M:%S; }

{
  echo "# 环境 $(sw_vers -productName) $(sw_vers -productVersion) $(uname -m) cores=$(sysctl -n hw.ncpu) mem=$(sysctl -n hw.memsize)"
  echo "# go: $(go version 2>/dev/null || echo n/a)"
  echo "# redis: $(redis-cli -p 16385 info server | grep -E 'redis_version|os:' | tr '\r\n' ' ')"
  echo "# loadavg: $(sysctl -n vm.loadavg)"
  echo "# date: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
} > "$RAW/01-env.txt" 2>&1
cat "$RAW/01-env.txt"

# ---------- A1: N=1000, 1000 条消息（高样本，P99 可信） ----------
(
for p in 0.1 0.5 1.0; do
  for s in w r h; do
    echo "### $(ts) A1 strategy=$s n=1000 p=$p m=1000"
    $BIN -mode fanout -strategy $s -n 1000 -p $p -m 1000
  done
done
) > "$RAW/10-A1-n1000-m1000.txt" 2>&1
echo "A1 done $(ts)"

# ---------- A2: N=10000, 100 条 ----------
(
for p in 0.1 0.5 1.0; do
  for s in w r h; do
    echo "### $(ts) A2 strategy=$s n=10000 p=$p m=100"
    $BIN -mode fanout -strategy $s -n 10000 -p $p -m 100
  done
done
) > "$RAW/11-A2-n10000-m100.txt" 2>&1
echo "A2 done $(ts)"

# ---------- A3: N=100000, 25 条 ----------
(
for p in 0.1 0.5 1.0; do
  for s in w r h; do
    echo "### $(ts) A3 strategy=$s n=100000 p=$p m=25"
    $BIN -mode fanout -strategy $s -n 100000 -p $p -m 25
  done
done
) > "$RAW/12-A3-n100000-m25.txt" 2>&1
echo "A3 done $(ts)"

# ---------- A6: 设备数放大（n=10000 p=0.5） ----------
(
for d in 1 2 3; do
  echo "### $(ts) A6 devices=$d n=10000 p=0.5 m=100 strategy=h"
  $BIN -mode fanout -strategy h -n 10000 -p 0.5 -m 100 -devices $d
done
) > "$RAW/13-A6-devices.txt" 2>&1
echo "A6 done $(ts)"

# ---------- A4: 读侧放大 ----------
(
for g in 10 50 200; do
  echo "### $(ts) A4 read_side groups=$g"
  $BIN -mode read -groups $g
done
) > "$RAW/14-A4-read.txt" 2>&1
echo "A4 done $(ts)"

# ---------- A5: 真实订阅者端到端 ----------
echo "### $(ts) A5 e2e subscribers" > "$RAW/15-A5-e2e.txt" 2>&1
$BIN -mode e2e -n 1000 -m 100 >> "$RAW/15-A5-e2e.txt" 2>&1
echo "A5 done $(ts)"

# ---------- A7: 逐人推送 vs 批量推送 ----------
(
echo "### $(ts) A7 push online=10000 m=100 batch=500"
$BIN -mode push -readers 10000 -m 100 -batch 500
) > "$RAW/16-A7-push.txt" 2>&1
echo "A7 done $(ts)"

# ---------- B: 已读回执 ----------
(
echo "### $(ts) B receipts n=5000 readers=1000"
$BIN -mode receipts -n 5000 -readers 1000
) > "$RAW/20-B-receipts.txt" 2>&1
echo "B done $(ts)"

# ---------- C: 热点 ----------
(
echo "### $(ts) C hot"
$BIN -mode hot -readers 200
) > "$RAW/30-C-hot.txt" 2>&1
echo "C done $(ts)"
echo "ALL DONE $(ts)"
