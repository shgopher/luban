# 第 12 章 发布与回滚：上线那一刻

> **本章一句话**：发布能力 = 出问题时能多快退回去；所有发布策略都是在「风险、成本、速度」三者间买保险。

**读完你会：**
- 给不同重要级别的服务选对发布策略
- 配出一套 5% 金丝雀并知道什么时候放量、什么时候回滚
- 写出不丢请求的优雅停机代码
- 用 Expand-Contract 安全地改数据库

---

## 12.1 四种发布策略怎么选

**结论先行**：能停机就停机，不能停机就滚动；核心链路要秒级回滚就用蓝绿；改动风险高、影响面大就用金丝雀。

| 策略 | 做法 | 停机 | 回滚速度 | 资源成本 | 适用 |
|---|---|---|---|---|---|
| 停机发布 | 停旧 → 起新 | 有 | 慢 | 1× | 内部系统、低峰可停 |
| 滚动发布 | 逐台替换 | 无 | 慢（逐台滚） | 1× | 无状态服务，默认选它 |
| 蓝绿发布 | 两套环境，流量一次切换 | 无 | 秒级 | 2× | 核心交易、支付 |
| 金丝雀 | 新版本先接 1%–5% 流量 | 无 | 快（切走流量） | 略 >1× | 高风险改动、大版本 |

> 💡 「能停机」是一种奢侈：它意味着你不用做任何新旧兼容设计。很多团队为了「不停机」，付出了远超停机窗口价值的复杂度。

---

## 12.2 滚动发布：两个参数和三条检查

**结论先行**：K8s 滚动发布只要调对 `maxSurge` 和 `maxUnavailable`，再确认三件事，就能覆盖 80% 的场景。

```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1          # 最多多起 1 个新 Pod
      maxUnavailable: 0    # 不允许任何时刻少 Pod → 发布期间容量不降
```

| 参数 | 含义 | 建议 |
|---|---|---|
| `maxSurge` | 允许多出几个新 Pod | 资源紧张用 0（先杀后起，会短暂降容量） |
| `maxUnavailable` | 允许多少 Pod 不可用 | 核心服务设 0，内部服务可设 25% |

三条检查，缺一条就会在发布窗口里随机报错：

1. **新旧版本兼容**：接口只加字段、不改语义；数据库先加列再用。
2. **探针正确**：readiness 通过才接流量，否则新 Pod 一启动就接请求。
3. **优雅停机**：旧 Pod 被杀前把在途请求处理完。

```bash
kubectl -n prod set image deploy/order-api app=registry.example.com/order-api:9f3c2a1
kubectl -n prod rollout status deploy/order-api --timeout=180s
kubectl -n prod rollout history deploy/order-api
kubectl -n prod rollout undo deploy/order-api            # 回上一版
kubectl -n prod rollout undo deploy/order-api --to-revision=3
```

⚠️ 滚动期间新旧版本同时在线。如果新版本改了响应结构，旧客户端会在「部分请求」上失败——**时好时坏，最难查**。

---

## 12.3 金丝雀：1% 流量怎么切

**结论先行**：金丝雀的关键不是「切多少」，而是「切之前定好观察指标和回滚阈值」。

用 Ingress-Nginx 做按权重切流：

```yaml
# 先部署一个 canary 版本（名字不同，带 canary 标签）
apiVersion: apps/v1
kind: Deployment
metadata: { name: order-api-canary, namespace: prod }
spec:
  replicas: 1
  selector: { matchLabels: { app: order-api, track: canary } }
  template:
    metadata: { labels: { app: order-api, track: canary } }
    spec:
      containers:
        - name: app
          image: registry.example.com/order-api:9f3c2a1
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: order-api-canary
  namespace: prod
  annotations:
    nginx.ingress.kubernetes.io/canary: "true"
    nginx.ingress.kubernetes.io/canary-weight: "5"     # 5% 流量
spec:
  ingressClassName: nginx
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /orders
            pathType: Prefix
            backend:
              service: { name: order-api-canary, port: { number: 80 } }
```

放量阶梯与判定：

| 阶段 | 流量 | 观察时长 | 放量条件 | 回滚条件 |
|---|---|---|---|---|
| 1 | 1% | 30 分钟 | 错误率 ≤ 基线 +0.1%，P99 ≤ 基线 +10% | 任一超标 |
| 2 | 5% | 30 分钟 | 同上，且无新告警 | 同上 |
| 3 | 20% | 1 小时 | 业务指标不降 | 同上 |
| 4 | 50% → 100% | 2 小时 | 全量 | 同上 |

```bash
# 放量就是改一个注解
kubectl -n prod annotate ingress order-api-canary \
  nginx.ingress.kubernetes.io/canary-weight=20 --overwrite
# 回滚就是把它删掉/权重归零
kubectl -n prod annotate ingress order-api-canary \
  nginx.ingress.kubernetes.io/canary-weight=0 --overwrite
```

> 🔑 金丝雀必须同时看**技术指标**（错误率、P99）和**业务指标**（下单成功率、支付成功率）。技术指标正常但业务指标掉了，一样要回滚。

---

## 12.4 回滚：三个层面都要能退

**结论先行**：回滚不止回代码，还有配置和数据；数据回滚最难，所以要在发布前就想好。

| 层面 | 手段 | 速度 | 前提 |
|---|---|---|---|
| 代码 | 镜像 tag 回退 / `rollout undo` | 秒到分钟 | 制品还在仓库 |
| 配置 | 配置中心版本回滚 / 上一版 YAML | 秒 | 配置有版本 |
| 数据 | 备份恢复 / binlog 反向补偿 / 双写回切 | 分钟到小时 | 有备份、有补偿脚本 |

回滚纪律：

- **5 分钟内定位不了，先回滚再说**，保留现场（日志、堆栈、线程 dump）。
- 回滚后不要立刻再发，先搞清楚「为什么」。
- 每次发布前写一句「回滚方式」，贴进发布单。

```bash
# 镜像回退（最常用）
kubectl -n prod set image deploy/order-api app=registry.example.com/order-api:<上一版sha>
# 或者
kubectl -n prod rollout undo deploy/order-api
```

---

## 12.5 优雅停机：发布不丢请求

**结论先行**：优雅停机 = 从负载均衡摘除 → 停止接新请求 → 等在途请求完成 → 关连接池 → 退出；四步顺序错一步就会 502。

```go
func waitAndShutdown(srv *http.Server) {
	// 1. 收到 SIGTERM / Ctrl-C
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()

	// 2. 停止接新请求，等在途请求处理完（必须小于 K8s 的 grace period）
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown", "err", err)
	}
	// 3. Shutdown 返回后关闭 DB/Redis 连接池，再退出
}
```

完整的 HTTP 服务模板（含超时、中间件、优雅停机）见第 16 章。K8s 侧配套：

```yaml
spec:
  terminationGracePeriodSeconds: 30   # 要大于 Shutdown 的 20s
  containers:
    - name: app
      lifecycle:
        preStop:
          exec: { command: ["sleep", "5"] }   # 等 Endpoint 摘除传播
```

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 没配 preStop，直接 SIGTERM | 摘流量还没生效，请求打到已退出进程 | preStop sleep 3–5 秒 |
| `Shutdown` 超时 ≥ grace period | 进程被 SIGKILL，在途请求全丢 | 留 5–10 秒余量 |
| 后台 goroutine 没停 | 进程卡住，滚动发布超时 | 用同一个 ctx 控制所有 goroutine |

---

## 12.6 数据库变更安全：Expand-Contract

**结论先行**：数据库变更必须拆成「先扩、再迁、后收」三步，每一步都能独立回滚；一次发布里同时改结构和改代码，是事故的温床。

| 步骤 | 做什么 | 代码状态 | 可回滚 |
|---|---|---|---|
| Expand | 加新列/新表，允许 NULL，双写新旧 | 旧代码照常跑 | 删新列 |
| Migrate | 回填历史数据，校验一致 | 双写 + 双读比对 | 停双写 |
| Contract | 切读新列，停止写旧列，最后删旧列 | 只读写新结构 | 保留旧列一段时间 |

以「给 orders 表加 `channel` 字段」为例：

```sql
-- 第 1 次发布：只加列，允许 NULL
ALTER TABLE orders ADD COLUMN channel VARCHAR(32) NULL,
  ALGORITHM=INSTANT;
-- 代码：写订单时同时写 channel（旧代码不写也能跑）
```

```sql
-- 第 2 次发布：回填历史数据（分批，每批 1000 行，避免长事务）
UPDATE orders SET channel = 'unknown'
WHERE channel IS NULL LIMIT 1000;
```

```sql
-- 第 3 次发布：切读、加 NOT NULL 约束、删旧字段
ALTER TABLE orders MODIFY COLUMN channel VARCHAR(32) NOT NULL DEFAULT 'unknown';
```

红线：

- 禁止在同一次发布里 `RENAME COLUMN` / `DROP COLUMN`；
- 大表加索引用 `ALGORITHM=INPLACE, LOCK=NONE`，或 gh-ost；
- 变更脚本必须能重复执行（`IF NOT EXISTS`）并记录版本。

---

## 12.7 发布检查清单

**结论先行**：发布事故八成不是代码问题，是「少做了一步」。把下面这张表贴进发布单。

| 阶段 | 检查项 | 完成 |
|---|---|---|
| 发布前 | 变更已评审，影响面写清 | ☐ |
| 发布前 | 回滚方式已写明并验证过 | ☐ |
| 发布前 | 数据库脚本按 Expand-Contract 拆分 | ☐ |
| 发布前 | 监控看板、告警已就位 | ☐ |
| 发布前 | 值班人、升级联系人已确定 | ☐ |
| 发布中 | 先发 1 个实例，观察 5 分钟 | ☐ |
| 发布中 | 错误率、P99、核心业务指标无异常 | ☐ |
| 发布后 | 核心链路手工验证（下单/支付/登录） | ☐ |
| 发布后 | 观察 30 分钟再离开 | ☐ |
| 发布后 | 记录发布结果与新版本号 | ☐ |

```bash
# 发布后 5 分钟快速核对
kubectl -n prod get pods -l app=order-api
kubectl -n prod logs deploy/order-api --since=5m | grep -c '"level":"ERROR"'
curl -s https://api.example.com/healthz
```

---

## 12.8 特性开关：把发布和上线分开

**结论先行**：部署（代码上线）和发布（功能对用户可见）是两件事；用开关把它们拆开，回滚就从「重新发版」变成「改一个配置」。

```go
// 一个最小的开关：从配置中心读取，带默认值
type Flags struct{ mu sync.RWMutex; m map[string]bool }

func (f *Flags) Enabled(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.m[key]
}

func (f *Flags) Set(key string, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[key] = on
}
```

| 开关类型 | 用途 | 生命周期 |
|---|---|---|
| Release | 新功能逐步放量 | 全量后删除 |
| Experiment | A/B 实验 | 实验结束删除 |
| Ops | 降级、限流、维护模式 | 长期保留 |
| Permission | 白名单、付费用户 | 长期保留 |

⚠️ 开关最大的债是「不清理」：半年后没人知道 `enable_new_checkout_v3` 是干嘛的，也没人敢删。每个开关建的时候就要写「删除条件」和负责人。

---

## 本章自检

**Q1** 滚动发布时错误率时高时低，最可能是什么原因？
▸ 答案：新旧版本同时在线且不兼容（接口字段、数据格式、缓存结构），请求落到哪个版本是随机的，所以表现为随机报错。先回滚，再检查兼容性。

**Q2** 为什么 `preStop` 要 sleep 几秒，直接 SIGTERM 不行吗？
▸ 答案：Pod 进入 Terminating 到 Endpoints 摘除、再到各节点规则生效有延迟，这段时间流量仍可能打进来。sleep 给传播留时间，否则请求打到正在退出的进程。

**Q3** 数据库加字段，为什么不能一次发布就 `ADD COLUMN NOT NULL` 再立刻改代码？
▸ 答案：旧代码还在跑，插入时不写新列会失败；必须允许 NULL 或给默认值，先双写、回填，再收紧约束，分三次发布。
