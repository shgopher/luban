# 0.12-orderid：订单号重构演算

[0.12 过度设计与模式反模式](../../../v2/内功/0-内功/0.12-过度设计与模式反模式.md) 后半段的配套代码：9 行雪花号 → 加锁 → 注入时钟 → 逻辑毫秒 → 删掉多余接口。

| 版本 | 代码 | 说明 |
|---|---|---|
| v0 | `NextV0` / `var v0seq` | 全局状态 + 真实时钟 + 无锁；同毫秒超过 4096 个就回绕，并发直接出重复 ID |
| v1 | `Gen` + `NewWithClock` | 互斥锁、时钟与节点可注入；序列用尽时向未来借 1 ms，不等待 |
| v2 | `GenIface` + `Clock` | 同一逻辑换成接口；`BenchmarkIfaceClock` 用来证明接口与函数值没有速度差 |
| 对照 | `bench_extra_test.go` | 自旋等下一毫秒的版本（`BenchmarkSpinClock`）与裸锁成本（`BenchmarkMutex`） |

## 跑起来

```bash
cd tools/examples/0.12-orderid

go test -race ./...                                  # 默认用例全绿
go test -tags v0broken -race -run TestV0             # 故意失败：复现 v0 的回绕与竞争
go test -run '^$' -bench . -benchmem -count=3
```

`v0broken` 那两个用例是**故意红**的，用来证明缺陷真实存在：

```text
--- FAIL: TestV0Concurrent (0.00s)
    duplicate id 400152562105324023      # 每次运行的 ID 不同
--- FAIL: TestV0SingleThreadRollover (0.00s)
    single-thread not increasing at i=4095
```

期望基准区间（go1.27.1 darwin/arm64，Apple M1；空闲状态）：

```text
BenchmarkV0-8           37–50 ns/op        0 B/op    0 allocs/op
BenchmarkFuncClock-8    48–63 ns/op        0 B/op    0 allocs/op
BenchmarkIfaceClock-8   49–64 ns/op        0 B/op    0 allocs/op
BenchmarkSpinClock-8    244–248 ns/op      0 B/op    0 allocs/op
BenchmarkMutex-8        14.1–14.4 ns/op    0 B/op    0 allocs/op
```

**绝对值会漂**：同一台 M1 在高负载时段测到过 57.5 / 75.6 / 76.0 ns，空闲时也有 37–50 ns 的波动，前后能差 30–40%。整章看两个比值就够了——正确性成本约 1.2 倍，接口与函数值无差别（区间直接重叠）。

自旋版 244–248 ns 几乎不随机器浮动，因为它主要不是在算，是在等：4096 个 ID 只花约 0.22 ms，剩下约 0.8 ms 在等真实时钟跨毫秒。
