# price-demo：0.12 的三版计价

[0.12 过度设计与模式反模式](../../../v2/内功/0-内功/0.12-过度设计与模式反模式.md) 的配套代码：同一个计价需求，三种写法各占 18 / 18 / 29 行。

| 目录 | 版本 | 说明 |
|---|---|---|
| `plain/` | if-else | 最直白，加一档要改函数体 |
| `strategy/` | 函数值策略 | `map[string]Rule`，加一档只加一行配置 |
| `factory/` | 抽象工厂 | `Pricer` + `Factory` 两层，每次调用 `Create()` 多一次分配 |
| `bencher/` | 压测 | 三版都套一层 `//go:noinline`，防止内联把分派优化掉 |

## 跑起来

```bash
cd tools/examples/price-demo

go test ./bencher -bench . -benchmem -count=3
```

期望输出（go1.27.1 darwin/arm64，Apple M1；不同时段重测差约 10%）：

```text
BenchmarkPlain-8       2.43–2.74 ns/op    0 B/op    0 allocs/op
BenchmarkStrategy-8   10.71–11.03 ns/op   0 B/op    0 allocs/op
BenchmarkFactory-8    14.72–15.16 ns/op   8 B/op    1 allocs/op
```

三版的相对关系是稳定的：策略版约 4.4 倍、工厂版约 6.1 倍，绝对差 8–13 ns；工厂版每次调用多 8 B / 1 次分配，来自 `&ratePricer{}` 逃逸到堆。

复现注意：`bencher` 用 `-count=3` 看区间，单次结果会被 CPU 频率与后台负载带偏；这组代码不吃时钟，所以比 0.12 后半段的订单号基准稳定。
