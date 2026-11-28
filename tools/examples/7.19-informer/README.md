# 7.19 Informer 与控制器工程化：最小可跑骨架

对应书稿：[7.19 Informer 与控制器工程化](../../../v2/L0-运行时底座/7-容器编排/7.19-Informer与控制器工程化.md)。

用 `client-go` 的 fake clientset + SharedInformerFactory，验证三件事：

| 断言 | 实现 |
|---|---|
| **WaitForCacheSync 之前不处理** | `TestRunWaitsForCacheSync`：先 `Run`、不启动 factory，100ms 内 `Attempts=0`；启动 factory 后才开始 reconcile |
| **失败走 AddRateLimited** | 第一次 `syncHandler` 返回 error，`processNextItem` 调 `AddRateLimited`，`NumRequeues` 从 0 变 1 |
| **成功走 Forget** | 第二次成功，`Forget` 把 `NumRequeues` 清零，队列清空 |

## 跑法

```bash
go test -v ./...
go run .
```

依赖：Go 1.26+、`k8s.io/client-go v0.37.3`（`go.mod` 已锁）。

## 期望输出（真实粘贴）

```text
$ go test -v ./...
=== RUN   TestRunWaitsForCacheSync
--- PASS: TestRunWaitsForCacheSync (0.22s)
=== RUN   TestRateLimitAndForget
--- PASS: TestRateLimitAndForget (0.12s)
PASS
ok  	github.com/shgopher/luban/tools/examples/7.19-informer	3.924s

$ go run .
first reconcile failed, requeues=1
second reconcile ok, attempts=2 requeues=0 queue_len=0
```

## 文件

| 文件 | 作用 |
|---|---|
| `controller.go` | Controller、Run/worker/processNextItem/syncHandler，以及 demo |
| `controller_test.go` | 两个断言：缓存同步门禁、AddRateLimited/Forget |
| `main.go` | 入口 |

## 注意

- 这是**最小骨架**：`syncHandler` 只从 Lister 读对象，不做真实业务；真实控制器要在 `IsNotFound` 分支清理外部资源。
- `NewSharedInformerFactory(client, 0)` 的第二个参数是 resyncPeriod；0 表示不周期重放。生产常用 30s~几分钟，并要在 `UpdateFunc` 里比较 `ResourceVersion`，否则 resync 会造成周期性热循环。
- `workqueue.DefaultControllerRateLimiter()` 的基数是 5ms，所以测试里第二次 reconcile 几乎立刻发生；不要用「等一会儿再看 NumRequeues」的写法，容易在 `Forget` 之后读到 0。示例用 `Requeued()` 通道在 `AddRateLimited` 处取样。
