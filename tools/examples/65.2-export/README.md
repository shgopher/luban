# 65.2-export：数据导出、探针与密钥擦除

[65.2 数据隔离与合规](../../../v2/L4-组件与技术/65-多租户SaaS/65.2-数据隔离与合规.md) 的配套代码。

| 路径 | 内容 | 跑法 |
|---|---|---|
| `export/main.go` | 断点续传 + 重投幂等的导出器 | `go run ./export` |
| `probe/main.go` | 导出链路探针 | `go run ./probe` |
| `shred/main.go` | AES-GCM 加密与密钥擦除 | `go run ./shred`（纯标准库） |

`*.log` 与 `VERIFY.md` 是当时的运行记录与验证步骤。导出器与探针需要可写的目标存储，按各自 `main.go` 顶部的参数改路径。
