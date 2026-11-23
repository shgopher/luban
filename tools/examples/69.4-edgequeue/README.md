# 69.4-edgequeue：断网续传（WAL + ack 水位）

[69.4 边缘计算与断网续传](../../../v2/L4-组件与技术/69-物联网/69.4-边缘计算与断网续传.md) 的配套代码：追加写 WAL + 独立 ack 水位 + 重放，三条不变量——只认 CRC 完整帧、ack 只增不减、重放不重不漏。

```bash
cd tools/examples/69.4-edgequeue
go test -race -v ./...
go test -run '^$' -bench . -benchtime 500x
```

测试（正文「实测」那段）：

```text
--- PASS: TestCrashResumeExactlyOnce (3.84s)   # 不 Close 直接重开 = 模拟 kill -9
--- PASS: TestTornTail (0.37s)
--- PASS: TestAckMonotonic (0.01s)
--- PASS: TestCompact (0.10s)
```

追加写成本：`BenchmarkAppendFsync` 约 3.7–3.8 ms/条（APFS 刷盘），`BenchmarkAppendNoFsync` 约 8–10 µs/条，两者相差两到三个数量级——**「每条都 fsync」的代价就是吞吐掉到几百条/秒**。
