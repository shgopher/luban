# 15.3-alloc：栈/堆、预分配与字符串拼接的分配账

[15.3 内存分配与逃逸分析](../../../v2/L0-运行时底座/15-语言运行时/15.3-内存分配与逃逸分析.md) 的配套基准。

```bash
cd tools/examples/15.3-alloc
go test -run '^$' -bench . -benchmem -benchtime=300ms -count=3
```

环境：Go 1.27.1、Apple M1（8 核）、macOS 15.6。取三次中位数。

| 基准 | ns/op | B/op | allocs/op | 说明 |
|---|---:|---:|---:|---|
| `PointValue` | 0.33 | 0 | 0 | 返回值类型，未逃逸 |
| `PointPointer` | 6.35 | 16 | 1 | 返回指针，逃逸到堆 |
| `SlicePrealloc` | 1132 | 8216 | 2 | `make([]int, 0, 1024)` 一次分配 |
| `SliceGrow` | 2471 | 25232 | 13 | 从 nil 开始 append，多次扩容 |
| `Sprintf` | 103.3 | 48 | 3 | `fmt.Sprintf` 解析格式串、装箱、新建字符串 |
| `AppendInt` | 35.8 | 31 | 2 | 复用 `[]byte`，最后转 string |

原始输出（节选）：

```text
BenchmarkPointValue-8       	1000000000	         0.3324 ns/op	       0 B/op	       0 allocs/op
BenchmarkPointPointer-8     	54564055	         6.354 ns/op	      16 B/op	       1 allocs/op
BenchmarkSlicePrealloc-8    	  299758	      1120 ns/op	    8216 B/op	       2 allocs/op
BenchmarkSliceGrow-8        	  144532	      2488 ns/op	   25232 B/op	      13 allocs/op
BenchmarkSprintf-8          	 3693512	        97.72 ns/op	      48 B/op	       3 allocs/op
BenchmarkAppendInt-8        	 9827048	        35.76 ns/op	      31 B/op	       2 allocs/op
```

## 读法

- **值 vs 指针**：`PointValue` 被内联/留在栈上，0 alloc；`PointPointer` 返回的指针必须逃逸，16B + 1 alloc。指针不一定“更省”，小对象反而多一次堆分配。
- **预分配**：`SlicePrealloc` 的 8216B/2 allocs 包含底层数组（8192B）和把 slice 存进 `sink` 的装箱；`SliceGrow` 扩容到 25232B/13 allocs，约 2.2× 时间、3× 内存。
- **字符串**：`fmt.Sprintf` 解析格式串、装箱、新建字符串，约 103ns/3 allocs；`AppendInt` 复用 `[]byte`，最后 `string(buf)` 仍有一次拷贝（31B/2 allocs）。
- `sink` 是包级 `any`，故意让结果逃逸；去掉它，编译器可能把更多东西优化掉，数字会变。

## 浮动说明

- 绝对数随机器/负载漂 ±10–20%；结论看比值（值 vs 指针、预分配 vs 扩容）。
- `PointValue` 的 0.33ns 已经接近“被优化掉”，只用于对比“栈上值”的下限。
- 字符串基准没有用 `fmt.Sprintf`，避免把格式串解析成本混进来；换成 `fmt.Sprintf` 会更高。
