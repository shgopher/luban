#!/bin/bash
# 在 run.sh 退出后单独运行：证明 tmux server 托管着会话，而 tmux 本身不认识 agent 状态。
set -euo pipefail

SOCKET=luban64
DEMO=${DEMO_ROOT:-/tmp/agent-orchestration}

t() { tmux -L "$SOCKET" "$@"; }

echo
echo "== 3. 创建脚本已退出：0 个客户端，3 个会话仍在"
clients=$(t list-clients -F '#{client_name}' 2>/dev/null | wc -l | tr -d ' ')
echo "attached clients: $clients"
t list-sessions -F '#{session_name} (#{session_windows} window)'

echo
echo "== 4. tmux 眼里只有 pane：看不到 working / blocked / idle"
t list-panes -a -F '#{session_name} | #{pane_current_path} | #{pane_current_command}'

echo
echo "== 5. 真实状态由 agent 自己报（这里用状态文件模拟）"
for f in "$DEMO"/status/*.status; do
  printf '%s: %s\n' "$(basename "$f" .status)" "$(cat "$f")"
done

echo
echo "结论：tmux 能保住终端，但 working 与 blocked 在 pane 里都只是 sleep；"
echo "     要显示 working/blocked/idle，得在 tmux 之上再包一层状态感知（herdr 那一类）。"

# 收尾：只清理这个 demo 的 socket 与目录
if [ "${KEEP:-0}" != "1" ]; then
  t kill-server 2>/dev/null || true
  for a in a b c; do
    git -C "$DEMO/repo" worktree remove --force "$DEMO/wt-$a" 2>/dev/null || true
  done
  rm -rf "$DEMO"
  echo
  echo "已清理（KEEP=1 可保留现场）"
fi
