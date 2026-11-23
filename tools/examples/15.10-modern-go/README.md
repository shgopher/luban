# 15.10-modern-go：版本门控、并发泄漏与工具链

[15.10 给新时代的 Go 编程](../../../v2/L0-运行时底座/15-语言运行时/15.10-给新时代的Go编程.md) 的配套代码。

```bash
cd tools/examples/15.10-modern-go
go run .                 # 现代写法串烧：strings.Cut / errors.Join / slices / range n / WaitGroup.Go
go test ./...            # 表驱动测试；Go 1.27 会顺带跑 stdversion
go run ./leakdemo        # 故意泄漏 goroutine，用 goroutineleak profile 抓出来
cd migration             # 版本门控实验：go.mod 声明 go 1.21
go fix -diff ./...       # 无输出：rangeint 被门控挡住
go mod edit -go=1.27.1
go fix -diff ./...       # 这时才给出 for i := range 10 的 diff
go mod edit -go=1.21.0
go test -tags demo_toonew ./...   # 用 1.27 的 strings.CutLast，编译期报版本不符
```

环境：Go 1.27.1、Apple M1、macOS 15.6。

## 一、现代写法串烧（`go run .`）

```text
invalid c-3: amount 0: must be > 0
valid=[a-1 b-2] total=350 bad=2
contains b-2: true
cheapest: a-1 100
workers done: 3
init once
```

覆盖的版本点：

| 写法 | 最低版本 | 位置 |
|---|---|---|
| `strings.Cut` | 1.18 | `parseOrder` |
| `errors.Join` | 1.20 | `validate` |
| `slices.Contains` / `slices.MinFunc` | 1.21 | `main` |
| `sync.OnceFunc` | 1.21 | `main` |
| `for range 3` | 1.22 | `main` |
| `wg.Go(f)` | 1.25 | `main` |
| `atomic.Int64` | 1.19 | `main` |

## 二、版本门控（`migration/`）

同一个 `for i := 0; i < 10; i++`，`go.mod` 写 `go 1.21.0` 时 `go fix` 不动它；把 `go` 指令改到 `1.27.1` 后才给出：

`go fix` 打印绝对路径，下面缩成 `migration/main.go`，diff 内容原样：

```diff
--- migration/main.go (old)
+++ migration/main.go (new)
@@ -4,7 +4,7 @@
 
 func main() {
 	total := 0
-	for i := 0; i < 10; i++ {
+	for i := range 10 {
 		total += i
 	}
 	fmt.Println(total)
```

`toonew.go` 用 `//go:build demo_toonew` 隔离，只有显式加 tag 才参与编译；它用了 Go 1.27 的 `strings.CutLast`：

```console
$ go test -tags demo_toonew ./...
./toonew.go:10:20: strings.CutLast requires go1.27 or later (file is go1.21)
FAIL	github.com/shgopher/luban/tools/examples/15.10-modern-go/migration [build failed]
```

## 三、goroutine 泄漏（`go run ./leakdemo`）

```text
=== goroutineleak ===
goroutineleak profile: total 1
1 @ 0x1022fbf18 0x102296224 0x102295e38 0x10234f818 0x102302024
#	0x10234f817	main.leak.func1+0x27	/Users/shgopher/Desktop/family/luban/tools/examples/15.10-modern-go/leakdemo/main.go:15
```

- `leak()` 里 `ch` 是局部变量，函数返回后不可达；发送 goroutine 永远等不到接收者。
- 检测依赖 GC 的可达性分析，所以示例里先 `runtime.GC()`，再写 `pprof.Lookup("goroutineleak")`。
- 栈地址与绝对路径每次运行都可能变，`total 1` 和函数名 `main.leak.func1` 是稳定部分。
- 盲区：阻塞原语能从全局变量或可运行 goroutine 的局部变量到达时，检测不到。

## 浮动说明

- `goroutineleak` 在 Go 1.26 是实验特性（`GOEXPERIMENT=goroutineleakprofile`），Go 1.27 才 GA。
- `go fix` 的 modernizer 集合会随版本变化；本示例在 Go 1.27.1 上验证。
- 示例里的 `go mod edit -go=1.27.1` 只是实验，验证完要改回 `1.21.0`，不要提交。
