# 15.12-go-semantics：11 个语义陷阱的最小复现

[15.12 Go 语义陷阱：编译器不报的 11 个错](../../../v2/L0-运行时底座/15-语言运行时/15.12-Go语义陷阱.md) 的配套代码。

```bash
cd tools/examples/15.12-go-semantics
go run .                                              # 1–11 号 demo，输出确定
go test -race ./...                                   # 语义断言（别名、副本、typed nil、布局）
go test -run '^$' -bench . -benchmem -benchtime=300ms -count=3
cd oldloop && go run .                                # go.mod = 1.21：循环变量只有一份
```

环境：Go 1.27.1、Apple M1（8 核）、macOS 15.6。基准取 `-count=3` 的中位数。

## 一、`go run .` 的真实输出

```text
启动顺序 ① 包级变量初始化
启动顺序 ② init()
启动顺序 ③ main()

== 1 shadow：:= 造出新变量，外层永远是 nil ==
  内层 client = "tracing"
  有 bug 的返回值 = <nil>
  修好后的返回值 = "tracing"

== 2 init：不是最先执行，也不是显式依赖 ==
  init 在包级变量之后、main 之前；它不接收参数、不返回错误、不能被调用
  同一个包里的多个 init 按文件名顺序执行；跨包按 import 依赖的拓扑序

== 3 嵌入类型：省下的转发代码，变成了公开的 API ==
  外部直接 Lock/Unlock 后读到 k = 2
  嵌入 sync.Mutex 让 *InMem 直接满足 sync.Locker：能力外泄，不是 bug 是语言规则

== 4 slice：len/cap、别名、copy、底层数组钉住 ==
  append 别名：s1=[1 2 10] s2=[2] s3=[2 10]
  copy 到 nil slice：n=0 dst=[]
  预分配：make([]int,3,6) 的 len=3 cap=6
  钉住版本 Alloc=128182 KB（1000×128KB 全部存活）
  复制版本 Alloc=414 KB（只留 2 个元素）

== 5 range：拿到的是副本，不是元素本身 ==
  改副本后：[{100} {200} {300}]
  go.mod >= 1.22：3 个指针互不相同 = true；go.mod = 1.21 时见 oldloop/
  按下标写回后：[{1 0} {2 0} {3 0}]

== 6 defer：声明时求值、返回前执行、循环里堆积 ==
  defer 参数在声明时求值：i=0
  匿名返回值函数返回 3
  命名返回值函数返回 6
  闭包读到的是最终值：i+1=2
  带闭包的函数返回 11
  循环结束，还没关闭任何一个
  第 2 个文件在函数返回时才关闭
  第 1 个文件在函数返回时才关闭
  第 0 个文件在函数返回时才关闭

== 7 string：字节序列，不是字符序列 ==
  len=6 bytes=68 c3 aa 6c 6c 6f
  下标 0 → h
  下标 1 → Ã
  下标 2 → ª
  下标 3 → l
  下标 4 → l
  下标 5 → o
  range 0 → h
  range 1 → ê
  range 3 → l
  range 4 → l
  range 5 → o
  截断后 Alloc=32932 KB（36 字节钉住 32MB 底层数组）
  strings.Clone 后 Alloc=169 KB

== 8 接口值不是 nil，只是它装的东西是 nil ==
  validateBug 返回 err == nil ? false
  err 的动态类型 = *main.MultiError，动态值 = <nil>
  validateFixed 返回 err == nil ? true

== 9 打印即加锁：String() 与锁重入 ==
  300ms 仍未返回：UpdateAge 持写锁 → fmt 调 String() → 抢读锁，自锁

== 10 结构体布局：对齐、填充、零大小字段 ==
  M{int64; struct{}} = 16 bytes（零大小字段在尾部，补齐到 16）
  N{struct{}; int64} = 8 bytes
  Bad{bool; int64; bool} = 24 bytes（两个 7 字节填充）
  Good{bool; bool; int64} = 16 bytes（填充合并成 6 字节）

== 11 []byte ↔ string：共享内存是结论，不是目的 ==
  改 []byte 之后 string = "Hello"（共享同一块内存）
  string → []byte：len=9 cap=9，没有复制
  规则：转换结果不能写字符串底层（可能只读段），也不能比原值活得更久
```

## 二、slice 预分配（`-bench`）

200,000 个 `int`，只差一个 `cap`：

| 基准 | 中位数 | 内存 | 分配次数 |
|---|---|---|---|
| `BenchmarkAppendNoCap` | 984,153 ns/op | 8,369,454 B/op | 31 allocs/op |
| `BenchmarkAppendWithCap` | 515,131 ns/op | 1,605,633 B/op | 1 allocs/op |

倍数：时间 1.91×、内存 5.2×、分配次数 31 → 1。原始输出（节选）：

```text
BenchmarkAppendNoCap-8       354   979214 ns/op   8369513 B/op   31 allocs/op
BenchmarkAppendNoCap-8       376   984153 ns/op   8369454 B/op   31 allocs/op
BenchmarkAppendWithCap-8     684   515131 ns/op   1605634 B/op    1 allocs/op
BenchmarkAppendWithCap-8     709   509068 ns/op   1605633 B/op    1 allocs/op
```

## 三、`[]byte` ↔ `string`：4 组转换（`-bench`）

43 字节的 payload（分配到 48B 的 size class），差异全在「复不复制」：

| 基准 | 中位数 | 内存 | 分配次数 |
|---|---|---|---|
| `BenchmarkStringToBytes` | 14.41 ns/op | 48 B/op | 1 allocs/op |
| `BenchmarkStringToBytesUnsafe` | 2.160 ns/op | 0 B/op | 0 allocs/op |
| `BenchmarkBytesToString` | 11.71 ns/op | 48 B/op | 1 allocs/op |
| `BenchmarkBytesToStringUnsafe` | 2.215 ns/op | 0 B/op | 0 allocs/op |

`unsafe` 版快 5–6.7×，靠的是「一次分配都不做」，不是更聪明的复制。

## 四、检查器：两条命令的真实输出

`shadow`（不在 `go vet` 默认集里，需要单独装/接 golangci-lint）：

```text
$ shadow ./main.go
/…/15.12-go-semantics/main.go:61:3: declaration of "client" shadows declaration at line 59
/…/15.12-go-semantics/main.go:67:3: declaration of "client" shadows declaration at line 59
```

`fieldalignment`（`go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest`）：

```text
$ fieldalignment ./main.go
/…/main.go:49:17: httpClient has 16 leading bytes of pointer data but optimal value is 8
/…/main.go:113:12: InMem has 16 leading bytes of pointer data but optimal value is 8
/…/main.go:350:15: Customer has 32 leading bytes of pointer data but optimal value is 8
/…/main.go:390:8: M has size 16 but the optimal size is 8 leading to a waste of 8 bytes (50%)
/…/main.go:400:10: Bad has size 24 but the optimal size is 16 leading to a waste of 8 bytes (33%)
```

## 五、旧循环变量语义（`oldloop/`）

```text
$ cd oldloop && go run .
go.mod = go 1.21：3 个指针地址完全相同 = true
m[1].ID=3 m[2].ID=3 m[3].ID=3
```

同一个 `for _, c := range` 写法，`go.mod` 写 `go 1.21` 时三个 `&c` 指向同一地址；主模块（`go 1.27.1`）里三个地址互不相同。语言版本由 `go.mod` 决定，不是由编译器版本决定。

## 浮动说明

- 内存绝对值（`128182 KB` / `32932 KB`）会随 GC 时机、分配器 size class 漂 ±5%，**看与对照组的比值**：128182 vs 414 是 300×，32932 vs 169 是 195×。
- `AppendNoCap` 的 31 次分配取决于 200k 规模下的 cap 序列（4 → 8 → … → 256 → 512 → 848 → …），换规模数字就变，比例不变。
- `M` 的 16 字节是「零大小字段在尾部必须补齐」的规则；`fieldalignment` 报 `optimal is 8` 是把它重排成 `N`。**规则与优化建议不矛盾**：不改布局就该认这 8 字节。
- `unsafe` 的两组基准只测转换本身，没有触发「字符串底层是否可写」的问题；把 `string` 转出的 `[]byte` 写回去可能打穿只读段，属于未定义行为，不要在生产里试。
- 第 9 个 demo 依赖 `fmt` 对 `Stringer` 的调用时机；改成 `%d` 或 `%s` 的字段级格式化就不会自锁，这也是修法之一。
