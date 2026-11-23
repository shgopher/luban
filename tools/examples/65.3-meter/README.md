# 65.3-meter：计量、配额与超卖

[65.3 计费与配额](../../../v2/L4-组件与技术/65-多租户SaaS/65.3-计费与配额.md) 的配套代码。

| 路径 | 内容 | 跑法 |
|---|---|---|
| `meter/main.go` | 事件流计量：重复、迟到、乱序 | `go run ./meter`（纯内存） |
| `quota/main.go` | 并发配额：先查后改 vs CAS 预扣 | `go run ./quota`（纯内存） |
| `redis_quota/main.go` | Redis 原子预扣 | 需 Redis：`go run ./redis_quota` |

Redis（Docker）：

```bash
docker run --rm -d --name redis65 -p 16382:6379 redis:7
```

`redis_quota` 默认连 `127.0.0.1:16382`，Redis 没起会直接 panic——这是有意的，配额不可用时不能静默放行。
