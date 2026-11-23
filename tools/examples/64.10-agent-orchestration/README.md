# 64.10-agent-orchestration：worktree 隔离 + tmux 会话托管

[64.10 多 Agent 编排](../../../v2/L4-组件与技术/64-机器学习系统/64.10-多Agent编排.md) 的配套示例。

为什么用 shell：这个示例验证的是 tmux 会话与 git worktree 的进程/文件系统行为，不是算法；shell 是唯一能直接驱动两者的语言，判断逻辑只有一个 `case`，没有可测的复杂分支。

## 跑法

```bash
cd tools/examples/64.10-agent-orchestration
docker build -t luban-64.10-agent .
docker run --rm luban-64.10-agent
```

本机已装 tmux / git：

```bash
bash run.sh && bash verify.sh
```

- 用独立 socket `-L luban64`，不会碰你已有的 tmux 会话。
- `KEEP=1 bash verify.sh` 可保留现场；默认会清理 `/tmp/agent-orchestration` 和本 demo 的 tmux server。
- 依赖 `git`、`tmux`、`bash`。

## 脚本在做什么

| 文件 | 作用 |
|---|---|
| `run.sh` | 建一个仓库、3 个 worktree/分支、3 个 detached tmux 会话；每个会话的工作目录指向自己的 worktree |
| `agent.sh` | mock agent：先写状态文件（`working` / `blocked` / `idle`），再进入对应形态 |
| `verify.sh` | 在另一个进程里检查：会话是否仍存活、pane 里显示什么、真实状态在哪、最后清理 |
| `Dockerfile` | Alpine 3.20 + tmux + git + bash |

## 实测输出

环境：Alpine 3.20 / tmux 3.4 / git 2.45.4 / arm64（Apple Silicon 上的 Docker Desktop）。

```text
== 1. worktree 隔离：一 agent 一目录一分支
/tmp/agent-orchestration/repo  c15ebdb [main]
/tmp/agent-orchestration/wt-a  c15ebdb [agent/a]
/tmp/agent-orchestration/wt-b  c15ebdb [agent/b]
/tmp/agent-orchestration/wt-c  c15ebdb [agent/c]

== 2. tmux 会话：每个 agent 一个
agent-a (1 window)
agent-b (1 window)
agent-c (1 window)

== 3. 创建脚本已退出：0 个客户端，3 个会话仍在
attached clients: 0
agent-a (1 window)
agent-b (1 window)
agent-c (1 window)

== 4. tmux 眼里只有 pane：看不到 working / blocked / idle
agent-a | /tmp/agent-orchestration/wt-a | sleep
agent-b | /tmp/agent-orchestration/wt-b | sleep
agent-c | /tmp/agent-orchestration/wt-c | bash

== 5. 真实状态由 agent 自己报（这里用状态文件模拟）
wt-a: working
wt-b: blocked
wt-c: idle

结论：tmux 能保住终端，但 working 与 blocked 在 pane 里都只是 sleep；
     要显示 working/blocked/idle，得在 tmux 之上再包一层状态感知（herdr 那一类）。

已清理（KEEP=1 可保留现场）
```

## 验到了什么

1. **worktree 隔离**：4 个目录、4 个分支，同一起点（`c15ebdb`），各自的暂存区互不影响。
2. **会话由 server 托管**：`run.sh` 退出后再查，`attached clients: 0` 且 3 个会话仍在——没有客户端，终端没死。
3. **tmux 的状态盲区**：`working` 与 `blocked` 两个 pane 的 `pane_current_command` 都是 `sleep`，从 pane 无法区分「在干活」还是「在等人」；`idle` 是 `bash`。
4. **状态得自己带**：状态文件里才有 `working` / `blocked` / `idle`，这正是 herdr 一类运行时在 tmux 之上补的那层。

## 环境与浮动

- 路径固定为 `/tmp/agent-orchestration`；commit hash 每次不同（内容相同也会因时间戳变化）。
- `pane_current_command` 依赖进程调度：采样瞬间可能显示 `bash` 或 `sleep`。**关键不是具体值，而是 `working` 与 `blocked` 无法区分**。
- `seq`、`basename` 用 Alpine 自带版本；x86_64 下输出一致，只有容器架构不同。
- `verify.sh` 默认清理：杀 `luban64` 这个 socket 的 server、删 worktree 和 demo 目录。镜像不会自动删，需要时 `docker rmi luban-64.10-agent`。

## 不验证什么

- 真实 herdr / luvus 的 socket API、pane 状态感知：示例只是用状态文件模拟 agent 自报状态。
- 交互式 attach/detach 的键盘操作：验证的是「0 个客户端时会话仍存活」，不是快捷键。
- 多机、权限隔离、资源限制：这些属于容器/调度层，不在本节范围。
