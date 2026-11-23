# 84.11-limitfw：限流框架骨架

[84.11 从零设计一个限流框架](../../../v2/附录/84-典型案例/84.11-从零设计限流框架.md) 的配套代码：单机、按 key、令牌桶，155 行框架 + 97 行分片版。

**这是教学示例，不是生产库**：没有分布式配额、没有热更新、没有内置指标，这些在正文里都标了「第二轮」。

## 跑起来

```bash
cd tools/examples/84.11-limitfw

go test -race ./...                        # 8 个用例，含并发 burst、TTL 回收、fail-open/closed
go test -run '^$' -bench . -benchmem -count=3
```

期望输出（go1.27.1 darwin/arm64，Apple M1；数字随机器浮动，量级不变）：

```text
BenchmarkAllow-8            114–116 ns/op    0 B/op    0 allocs/op
BenchmarkAllowParallel-8    196–200 ns/op    0 B/op    0 allocs/op   # 1024 key，全局 map
BenchmarkShardedParallel-8  105–109 ns/op    0 B/op    0 allocs/op   # 8 分片
BenchmarkKeyedHotKey-8      203–208 ns/op    0 B/op    0 allocs/op   # 同一个热点 key
BenchmarkShardedHotKey-8    203–207 ns/op    0 B/op    0 allocs/op   # 分片救不了热点

绝对值随机器负载浮动（同一台 M1 能差近 2 倍），**看比值**：多 key 并行时分片约 1.9 倍，热点 key 时二者持平。
```

## 目录

| 文件 | 内容 |
|---|---|
| `limiter.go` | `Decision` / `Limiter` / `Config`、配置校验、令牌桶、`Keyed` + TTL 回收 |
| `middleware.go` | HTTP 中间件：429 + `Retry-After`，故障时 fail-open / 503 |
| `sharded.go` | `Sharded`：同一接口，key 哈希到 8 个分片 |
| `limiter_test.go` | 8 个用例 + 5 个 benchmark |

最小用法：

```go
k, err := limitfw.NewKeyed(limitfw.Config{Rate: 100, Burst: 200, TTL: time.Minute})
if err != nil {
	log.Fatal(err) // 配置非法就不要启动
}
defer k.Close()

handler := limitfw.Middleware(k, func(r *http.Request) string {
	return r.RemoteAddr // 登录后换成租户/用户 ID
}, true) // true = 限流器故障时放行（读接口）
```
