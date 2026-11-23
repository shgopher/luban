#!/bin/bash
set -u
cd "$(dirname "$0")"
BIN=./s665
RAW=../raw
ts() { date -u +%H:%M:%S; }

echo "### $(ts) memmap MEMORY USAGE" > "$RAW/05-memmap.txt" 2>&1
$BIN -mode memmap >> "$RAW/05-memmap.txt" 2>&1
echo "memmap done $(ts)"

echo "### $(ts) B receipts n=5000 readers=1000 (fixed accounting)" > "$RAW/20-B-receipts.txt" 2>&1
$BIN -mode receipts -n 5000 -readers 1000 >> "$RAW/20-B-receipts.txt" 2>&1
echo "B done $(ts)"

echo "### $(ts) A3-rerun W n=100000 m=25（复跑验证首轮冷启动影响）" > "$RAW/12b-A3-rerun.txt" 2>&1
for p in 0.1 0.5 1.0; do
  echo "### $(ts) A3rerun strategy=w n=100000 p=$p m=25" >> "$RAW/12b-A3-rerun.txt"
  $BIN -mode fanout -strategy w -n 100000 -p $p -m 25 >> "$RAW/12b-A3-rerun.txt" 2>&1
done
echo "A3 rerun done $(ts)"

echo "### $(ts) deploy_check" > "$RAW/40-deploy.txt" 2>&1
bash deploy_check.sh >> "$RAW/40-deploy.txt" 2>&1
echo "deploy done $(ts)"
echo "REST DONE $(ts)"
