#!/bin/bash
# 建 3 个 worktree + 3 个 tmux 会话，每个会话在自己的 worktree 里跑一个 mock agent。
# 用独立 socket（-L luban64），不会碰你本机已有的 tmux 会话。
set -euo pipefail

SOCKET=luban64
DEMO=${DEMO_ROOT:-/tmp/agent-orchestration}
HERE=$(cd "$(dirname "$0")" && pwd)
AGENT="$HERE/agent.sh"

t() { tmux -L "$SOCKET" "$@"; }

# 上一次的残留先清掉；只杀这个 demo 的 socket
t kill-server 2>/dev/null || true
rm -rf "$DEMO"
mkdir -p "$DEMO/status"

# 1. 一个只有一次提交的仓库
git init -q -b main "$DEMO/repo"
cd "$DEMO/repo"
git config user.email demo@example.com
git config user.name demo
printf 'package main\n\nfunc main() {}\n' >main.go
git add .
git commit -qm init

# 2. 每个 agent 一个 worktree + 一个分支
for a in a b c; do
  git worktree add -q "../wt-$a" -b "agent/$a"
done

# 3. 每个 agent 一个 tmux 会话，会话的工作目录就是它的 worktree
t new-session -d -s agent-a -c "$DEMO/wt-a" "STATUS_DIR='$DEMO/status' bash '$AGENT' working"
t new-session -d -s agent-b -c "$DEMO/wt-b" "STATUS_DIR='$DEMO/status' bash '$AGENT' blocked"
t new-session -d -s agent-c -c "$DEMO/wt-c" "STATUS_DIR='$DEMO/status' bash '$AGENT' idle"

# 等 agent 写完状态文件
for _ in $(seq 1 20); do
  [ "$(ls "$DEMO"/status/*.status 2>/dev/null | wc -l)" -eq 3 ] && break
  sleep 0.1
done

echo "== 1. worktree 隔离：一 agent 一目录一分支"
git worktree list

echo
echo "== 2. tmux 会话：每个 agent 一个"
t list-sessions -F '#{session_name} (#{session_windows} window)'
