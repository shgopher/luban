# 40.6-errbench：error 与 panic+recover 的传播成本

[40.6 错误处理](../../../v2/L3-质量属性/40-可维护性/40.6-错误处理.md) 的配套代码：只测「传播一次」的开销，不含业务逻辑。

```bash
cd tools/examples/40.6-errbench
go run .
```

一次真实输出（单次运行，`testing.Benchmark` 的 b.N 会自动定）：

```text
BenchmarkError        1000000000     0.9702 ns/op
BenchmarkPanicPlain    7926078     148.1  ns/op
BenchmarkPanicRecover  5416059     218.6  ns/op
```

五次连测的区间：error 0.96–1.06 ns、panic+recover 154–186 ns、panic+recover+包 error 215–262 ns。结论看量级：**约 150–270 倍**，所以异常不能当流程控制。

`//go:noinline` 是为了防止编译器把整个调用优化掉；benchmark 里只判断 `err != nil`，不读错误内容。
