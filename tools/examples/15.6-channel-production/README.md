# 15.6-channel-production：三个大型项目里的 channel 结构

[15.6 并发原语 (channel / context / sync)](../../../v2/L0-运行时底座/15-语言运行时/15.6-并发原语.md) 的配套代码。

```bash
cd tools/examples/15.6-channel-production
go run ./workerpool   # 有界队列 + 固定 worker + 单独 closer
go run ./fanout       # 一个发布者、多个订阅者，慢消费者隔离
go run ./fanin        # 多副本并发，第一个成功即返回
```

环境：Go 1.27.1、Apple M1、macOS 15.6。

## 一、workerpool：任务导出 / 转码

```text
done: 10
ids: [0 1 2 3 4 5 6 7 8 9]
workers: map[0:3 1:4 2:3]
```

关键结构：

| 角色 | 谁关闭 | 为什么 |
|---|---|---|
| `jobs` | 生产者 | 生产者知道「不会再有任务」 |
| `results` | 单独的 closer（`wg.Wait()` 后） | worker 不知道其他 worker 是否还在发 |
| worker | 不关闭任何 channel | 只负责消费与发送 |

`workers` 的分布每次运行都会变，`done: 10` 与 `ids` 是稳定部分。`jobs` 容量 4 是背压点：队列满时生产者阻塞，而不是把任务堆进内存。

## 二、fanout：行情 / SSE / 聊天室推送

```text
fast: [0 1 2 3 4]
slow buffered: 2 dropped: 3
```

- 每个订阅者一个**独立** channel；共享一个 channel 是「负载均衡」，不是「广播」。
- 缓冲满了走 `default` 丢弃或踢掉慢订阅者；绝不让慢消费者阻塞发布者。
- 真实项目还要加指标：每订阅者 `len(ch)`、丢弃数、最后投递延迟。丢消息可以，但必须可观测。

## 三、fanin：多副本取最快（hedged request）

```text
winner=replica-2 elapsed≈11ms
```

- 结果 channel 容量 = 副本数：第一个成功后 `cancel()`，输掉的副本仍能把 `ctx.Err()` 写进缓冲并退出，不会泄漏。
- 如果结果 channel 无缓冲，赢家返回后没人再收，输掉的副本会永久阻塞在发送上——这是「取最快」最常见的泄漏。
- 生产上还要限制并发副本数、给每个副本单独超时、记录哪个副本赢，否则会放大下游流量。
