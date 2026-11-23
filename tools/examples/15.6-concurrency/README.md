# 15.6-concurrency：channel 用法与三组原语的单价

[15.6 并发原语 (channel / context / sync)](../../../v2/L0-运行时底座/15-语言运行时/15.6-并发原语.md) 的配套代码。

```bash
cd tools/examples/15.6-concurrency
go run .                                      # 五个 channel 用法，输出确定
go test -run '^$' -bench . -benchmem -benchtime=300ms -count=3   # 全部基准
go test -run '^$' -bench CounterMutex -cpu=1,2,4,8 -benchtime=300ms -count=3
```

环境：Go 1.27.1、Apple M1（8 核）、macOS 15.6。基准取 `-count=3` 的中位数。

## 一、channel 用法（`go run .`）

```text
信号量：limit=3 tasks=8 并发峰值=3
非阻塞发送：容量=2 写入=2 丢弃=3
关闭广播：3 个接收者全部退出，计数=3
nil 分支：ready 命中=42，nil 分支永不就绪
可取消等待：err=true 实际耗时≈11ms（请求 200ms）
```

- 信号量用 `sem <- struct{}{}` / `<-sem` 卡住并发；峰值由 `atomic` 统计，稳定为 3。
- 非阻塞发送用 `select + default`：5 次写入、容量 2，稳定「成功 2 / 丢弃 3」。
- `close(stop)` 一次唤醒 3 个接收者——这是 close 唯一的广播语义。
- `nil` channel 的分支永远不就绪，所以 `ready` 立即命中、不会跑到超时。
- `sleepCtx` 用 `NewTimer` 而不是 `time.Sleep`：请求 200ms，10ms 超时后立刻返回，耗时约 11ms。

## 二、同一份自增，三种同步方式（`-cpu=1,2,4,8`）

`bench_test.go` 里三个计数器做同一件事：`n++`。`BenchmarkCounterMutex` 就是正文里的 `CounterMutex`：

```go
type CounterMutex struct {
	mu sync.Mutex
	n  int64
}

func (c *CounterMutex) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}
```

`b.RunParallel` 让 `P` 个 goroutine 同时 `Inc`，`-cpu=1,2,4,8` 改变 `P`：

| P | `CounterMutex` | `CounterAtomic` | `CounterChannel` |
|---|---|---|---|
| 1 | 16.8 ns/op | 7.1 ns/op | 28.7 ns/op |
| 2 | 47.3 ns/op | 16.6 ns/op | 37.7 ns/op |
| 4 | 74.4 ns/op | 21.3 ns/op | 54.0 ns/op |
| 8 | 83.0 ns/op | 38.3 ns/op | 70.3 ns/op |

`CounterChannel` 是「一个 goroutine 独占计数器 + `cap=1024` 的 channel 收自增指令」，单消费者把并发又串回了一个核：退化比 Mutex 平缓（28.7→70.3ns），但绝对单价更高。原始输出（节选）：

```text
BenchmarkCounterMutex       	21562695	        16.68 ns/op
BenchmarkCounterMutex-2     	 7566183	        46.84 ns/op
BenchmarkCounterMutex-4     	 4778052	        74.44 ns/op
BenchmarkCounterMutex-8     	 4285035	        82.96 ns/op
BenchmarkCounterAtomic      	51313730	         7.120 ns/op
BenchmarkCounterAtomic-8    	10151281	        38.25 ns/op
BenchmarkCounterChannel-8   	 6262888	        70.97 ns/op
```

## 三、其余基准（默认 `GOMAXPROCS=8`，中位数）

| 基准 | 中位数 | 说明 |
|---|---|---|
| `MutexUncontended` | 13.8 ns/op | 单 goroutine 成对 Lock/Unlock |
| `ChanHandoffBuffered1024` | 74.0 ns/op | 有缓冲交接，收发各一次 |
| `ChanHandoffUnbuffered` | 186.4 ns/op | 无缓冲要等对方就绪，贵约 2.5× |
| `MutexReadOnly` | 85.5 ns/op | 空临界区只读 |
| `RWMutexReadOnly` | 76.9 ns/op | 空临界区只读 |
| `MutexMixed9To1` | 104.7 ns/op | map 读写 9:1 |
| `RWMutexMixed9To1` | 55.2 ns/op | 同上，RWMutex |
| `SyncMapMixed9To1` | 9.4 ns/op，6 B/op | 1024 个 key，读写 9:1 |
| `MapRWMutexMixed9To1` | 47.9 ns/op | 同上，map+RWMutex |
| `SyncMapDisjointWrites` | 108.8 ns/op，122 B/op，3 allocs | 每 goroutine 只写自己的 key |
| `MapRWMutexDisjointWrites` | 242.2 ns/op，51 B/op | 同上，map+RWMutex |
| `Make4K` | 447.8 ns/op，4120 B/op，2 allocs | 每次 `make([]byte,0,4096)` |
| `Pool4K` | 8.9 ns/op，0 B/op | 从 `sync.Pool` 取同一个 4KB 缓冲 |

`Pool4K` 的 0 alloc 有前提：基准只把缓冲指针存进包级变量，没有跨 goroutine 发布，也没有调用可能保留它的接口。真实编码路径里一旦把 `*[]byte` 交给会长期持有它的对象，逃逸与额外分配会回来——看量级，别把 0 alloc 当普适结论。

## 四、context 与分片计数

```bash
go test -run '^$' -bench 'Context|CounterSharded' -benchmem -benchtime=300ms -count=3
```

| 基准 | 中位数 | 说明 |
|---|---|---|
| `ContextWithCancel` | 55.0 ns/op，96 B/op，2 allocs | 创建 + 立即 cancel |
| `ContextWithTimeout` | 241.0 ns/op，272 B/op，4 allocs | 创建 + 立即 cancel（含 timer） |
| `ContextValueDepth1` | 3.2 ns/op | 第一层命中 |
| `ContextValueDepth4` | 11.8 ns/op | 穿 4 层 |
| `ContextValueDepth16` | 46.7 ns/op | 穿 16 层，近似线性 |
| `CounterShardedPadded` | 5.8 ns/op | 8 分片，每片按 64B 对齐 |
| `CounterShardedTight` | 42.9 ns/op | 8 个原子变量紧挨着，false sharing 把收益吃光 |

`CounterAtomic` 在 P=8 时是 38.3 ns/op；分片对齐后 5.8 ns/op，约 6.6×。但如果不 padding，`CounterShardedTight` 反而是 42.9 ns/op——**分片不等于自动变快，缓存行才是单位**。

## 浮动说明

- 绝对数会随机器、负载、Go 版本漂：同一台机重跑通常 ±10%，8 核争用项能到 ±20%。结论看比值（Mutex 17→83、atomic 7→38），不看某一次绝对值。
- `SyncMap*` 的 `B/op`、`allocs/op` 来自 `interface{}` 装箱与 trie 节点，不是漏内存；看 `ns/op` 比较。
- `CounterChannel` 的 `n` 由独占 goroutine 写，基准结束后 `Close()` 才读，因此没有数据竞争；别用 `-race` 跑它来证明「channel 自带同步」以外的结论。
- `Context*` 的数字随 Go 版本会变（Go 1.27 的 `cancelCtx` 用 `atomic.Value` 保存 `done/err`）；`ContextValueDepth16` 的耗时取决于链深与 key 的位置。
- 分片计数强依赖缓存行：`CounterShardedPadded` 只在 P=8 下比较，换机器（尤其是 128B 缓存行）要重测；`Tight` 变慢就是 false sharing 的信号。
