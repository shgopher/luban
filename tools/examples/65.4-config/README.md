# 65.4-config：四层配置覆盖与组合矩阵

[65.4 定制化与版本管理](../../../v2/L4-组件与技术/65-多租户SaaS/65.4-定制化与版本管理.md) 的配套代码。

| 路径 | 内容 | 跑法 |
|---|---|---|
| `config/main.go` | 四层 map 逐层覆盖 + 来源追踪 + 五类校验 | `go run ./config` |
| `matrix/main.go` | 可达组合（平台版本 × 配置上限 × 数据区间）的验证矩阵 | `go run ./matrix` |

根目录 `config-output.txt`、`matrix-output.txt` 是原始输出。
