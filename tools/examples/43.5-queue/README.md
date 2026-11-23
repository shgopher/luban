# 43.5-queue：channel 会合 → list 缓冲（vendored）

[43.5 积压与背压](../../../v2/L4-组件与技术/43-消息队列/43.5-积压与背压.md) 与 [84.9](../../v2/附录/84-典型案例/84.9-onex生态九仓的可抄零件.md) 引用的 `go-sample-queue`：93 行实现 + 328 行测试，把 channel 的“会合”变成 list 的“缓冲”。

```bash
cd tools/examples/43.5-queue
go test ./...
```

这是**第三方仓库**（onexstack/go-sample-queue，MIT，见 `LICENSE.txt`），原样 vendored 以支撑正文数字：生产者 200ms/条、消费者 400ms/条时，最后一条发送从 1608ms 降到 804ms；100 万条不消费堆从 0.2MB 涨到 72.2MB。
