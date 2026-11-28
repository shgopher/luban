# 0.16-rsqrt：平方根倒数的位运算近似

[0.16 浮点数的位表示与近似算法](../../../v2/内功/0-内功/0.16-浮点数的位表示与近似算法.md) 的配套代码：把《左耳听风》第 13 讲那条带 `// what the fuck?` 的代码拆成三段，逐段验证，再在 2026 年的机器上重测它到底还快不快。

| 文件 | 内容 |
|---|---|
| `rsqrt.go` | `Split`/`Compose`（S/E/M 三段）、`FastInvSqrt`（一次牛顿迭代）、`FastInvSqrt2`（两次）、`Exact`（`1/math.Sqrt`）、四个候选魔数常量 |
| `rsqrt_test.go` | 3.14 与 0.015 的位拆解与回算、`0.1+0.2 != 0.3`、三个魔数的误差统计、四个基准 |

## 跑起来

```bash
cd tools/examples/0.16-rsqrt

go test -v -run 'TestSplit|TestZeroOne|TestAccuracy' ./...
go test -run '^$' -bench . -benchmem -count=5
```

> 本机 `GOCACHE` 若不可写（沙箱环境），先 `export GOCACHE=$PWD/../../.cache/gocache` 再跑。

## 期望输出（go1.27.1 darwin/arm64，Apple M1，2026-11 实测）

位拆解与回算：

```text
rsqrt_test.go:33: 3.14   E=128 M=4781507  回算=3.1400001049041748047 相对误差=3.341e-08
rsqrt_test.go:33: 0.015  E=120 M=7717519  回算=0.014999999664723873138 相对误差=-2.235e-08
rsqrt_test.go:50: 0.1+0.2 = 0.30000000000000004441, 0.3 = 0.2999999999999999889, 差 = 5.551e-17
```

精度：在 `[10^-2, 10^3]` 上取 20 万个点，相对误差的分位数就是迭代次数的函数：

```text
0x5f3759df(Quake)      1 次迭代 最大相对误差=1.752e-03 平均=9.363e-04 | 2 次迭代 最大=4.706e-06 平均=1.828e-06
0x5f37642f(最小误差)    1 次迭代 最大相对误差=1.776e-03 平均=9.461e-04 | 2 次迭代 最大=4.842e-06 平均=1.865e-06
0x5f375a86(迭代后最优)  1 次迭代 最大相对误差=1.751e-03 平均=9.369e-04 | 2 次迭代 最大=4.718e-06 平均=1.831e-06
```

三个魔数的差别在**第四位有效数字**上，说明"挑魔数"远不如"多迭代一次"值钱：一次迭代 1e-3，两次迭代 5e-6，差三个数量级。

速度（1024 个不同输入轮转，避免常数折叠）：

```text
BenchmarkFast1Slice-8   459922612   3.122 ns/op   0 B/op   0 allocs/op
BenchmarkFast1Slice-8   538702098   3.311 ns/op   0 B/op   0 allocs/op
BenchmarkFast1Slice-8   520299822   3.456 ns/op   0 B/op   0 allocs/op
BenchmarkFast1Slice-8   476247092   2.727 ns/op   0 B/op   0 allocs/op
BenchmarkFast1Slice-8   507701101   2.330 ns/op   0 B/op   0 allocs/op
BenchmarkExactSlice-8   655173081   1.711 ns/op   0 B/op   0 allocs/op
BenchmarkExactSlice-8   593094039   2.092 ns/op   0 B/op   0 allocs/op
BenchmarkExactSlice-8   636177154   2.633 ns/op   0 B/op   0 allocs/op
BenchmarkExactSlice-8   704043039   1.741 ns/op   0 B/op   0 allocs/op
BenchmarkExactSlice-8   756898418   1.675 ns/op   0 B/op   0 allocs/op
```

结论：**位运算版没有更快，均值还略慢**（位版 2.33–3.46 ns，硬件版 1.68–2.63 ns）。1990 年代浮点远慢于整数，这段代码才成立；今天 `sqrtss` 是一条指令、延迟约十几个周期，位技巧换来的只是"少一次函数调用"的错觉。

## 数字会漂

- 绝对值随负载漂 30–40%，同一台机器不同时段测到过 2.33 与 3.46 ns；**只看区间是否重叠**，单次结果不作数。
- 两次迭代的误差（4.7e-6）与 `float32` 的有效位（约 1.2e-7）还差一个量级，所以"两次迭代"已是这套近似的天花板。
- `0.1+0.2` 的差值 5.551e-17 是确定的，不随机器变。

## 这个例子证明什么

1. 浮点数存不住十进制小数，是**位宽决定的**，不是实现 bug；
2. `log2(1+m) ≈ m + σ` 这条直线近似，让"求平方根倒数"变成"一次减法 + 一次右移"；
3. 初值质量决定迭代次数，而**迭代次数决定精度**，挑魔数的收益远小于多迭代一次；
4. 一段"经典优化"可能只在它的年代成立——**照抄前先在本机测**。
