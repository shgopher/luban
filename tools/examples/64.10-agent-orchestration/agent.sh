#!/bin/bash
# 模拟一个 agent：自报状态，然后进入对应形态。
# 用法：STATUS_DIR=/path agent.sh <working|blocked|idle>
set -euo pipefail

status=${1:?usage: agent.sh <working|blocked|idle>}
status_dir=${STATUS_DIR:?STATUS_DIR is required}
mkdir -p "$status_dir"

# 用当前工作目录名当 agent 名：wt-a -> wt-a.status
name=$(basename "$PWD")
echo "$status" >"$status_dir/$name.status"

case "$status" in
working) exec sleep 600 ;; # 模拟在跑长任务（真实 agent 里是一次工具调用）
blocked) exec sleep 600 ;; # 模拟等待人工审批
idle) exec bash ;;         # 模拟空闲，等人派活
esac
