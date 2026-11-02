# 第 10 章 稳定性五件套：超时、重试、熔断、限流、降级

> **本章一句话**：稳定性不是「不出故障」，而是「故障发生时只坏一小块、并且能自己恢复」；五件套就是给每一次调用装上保险丝。

**读完你会：**
- 给每条调用链定出合理的超时
- 判断哪些错误能重试、重几次
- 配出一套能真正熔断的阈值
- 把限流、降级、隔离落到代码和网关配置里

---

## 10.1 超时：每个调用都必须有

**结论先行**：超时值 = 下游 P99 × 2~3；没有超时的调用，等于把线程、连接和内存无限期押给一个可能永远不返回的依赖。

| 层级 | 超时预算 | 说明 |
|---|---|---|
| 客户端 / App | 3s | 用户可感知的上限 |
| 网关 | 2.5s | 比客户端短，留渲染时间 |
| 业务服务 | 2s | 自己的处理 + 下游调用 |
| 数据库 | 1s | 单条 SQL，慢查询另算 |
| 下游 RPC | 800ms | 越往下越短，总和不超上层 |

```go
func (s *Service) CreateOrder(ctx context.Context, req *Req) (*Order, error) {
	// 整个请求 2 秒预算；下游调用在此基础上再切分
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	o, err := s.order.Create(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("create order: upstream timeout: %w", err)
		}
		return nil, err
	}
	return o, nil
}
```

| 规则 | 原因 |
|---|---|
| 禁止超时设为 0 / 无限 | 一个慢依赖就能拖垮整个进程 |
| 超时必须往下传 | 上层取消了，下层还在跑就是浪费 |
| 各层超时要递减 | 下层比上层还长，等于没设 |
| 超时要能区分「自己慢」和「下游慢」 | 日志里记 `stage=dependency` 与耗时 |

---

## 10.2 重试：什么能重试，重几次

**结论先行**：只重试「幂等 + 临时性失败」，最多 2–3 次，必须指数退避加抖动，且重试流量不超过总流量的 10%。

| 错误类型 | 能重试吗 | 原因 |
|---|---|---|
| 连接超时、连接被拒 | 能 | 临时网络问题 |
| 503 / 429 | 能（429 要更久） | 对方过载，退避后可能恢复 |
| 500 / 502 / 504 | 谨慎 | 可能已产生副作用，只对幂等接口重试 |
| 400 / 401 / 403 / 404 | 不能 | 重试多少次都一样 |
| 409 冲突 | 不能 | 业务状态问题，要人处理 |
| 本地校验失败 | 不能 | 代码或参数问题 |

```go
func retry(ctx context.Context, attempts int, fn func(context.Context) error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(ctx); err == nil {
			return nil
		}
		if !isRetryable(err) || ctx.Err() != nil {
			return err
		}
		// 100ms, 200ms, 400ms ... 再加 20% 抖动，避免同时重试
		base := time.Duration(100<<i) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(base/5) + 1))
		select {
		case <-time.After(base + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", attempts, err)
}

func isRetryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errConnRefused) {
		return true
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	return false
}
```

⚠️ 不要在每一层都重试：网关重试 2 次、服务重试 2 次、客户端再重试 2 次，一个请求会放大成 8 个，故障时直接把下游打死。

---

## 10.3 熔断：三个阈值怎么设

**结论先行**：熔断是「下游已经不行了，别再打了」；用错误率 + 最小请求数 + 打开时长三个参数，替代人工关流量。

| 状态 | 行为 | 进入条件 |
|---|---|---|
| 关闭 | 正常放行，统计失败 | 默认 |
| 打开 | 直接失败，不再调用下游 | 窗口内错误率 > 50% 且请求数 ≥ 20 |
| 半开 | 放少量请求试探 | 打开 30 秒后 |

```go
br := gobreaker.NewCircuitBreaker(gobreaker.Settings{
	Name:        "payment-api",
	MaxRequests: 5,               // 半开状态最多放 5 个试探请求
	Interval:    10 * time.Second, // 关闭状态每 10 秒清零统计
	Timeout:     30 * time.Second, // 打开 30 秒后转半开
	ReadyToTrip: func(c gobreaker.Counts) bool {
		return c.Requests >= 20 &&
			float64(c.TotalFailures)/float64(c.Requests) > 0.5
	},
	OnStateChange: func(name string, from, to gobreaker.State) {
		slog.Warn("circuit state changed", "name", name, "from", from, "to", to)
	},
})
```

| 参数 | 建议值 | 调大/调小的后果 |
|---|---|---|
| 错误率阈值 | 50% | 太低误熔断，太高失去保护 |
| 最小请求数 | 20 | 太小被偶发错误触发，太大反应慢 |
| 统计窗口 | 10s | 太长恢复慢，太短太敏感 |
| 打开时长 | 30s | 太短下游没恢复就再打，太长恢复慢 |

熔断打开时必须有兜底：返回缓存、返回默认值，或明确告诉调用方「稍后再试」。

---

## 10.4 限流：放哪一层，限多少

**结论先行**：限流要放在「最靠近入口、且能拿到限流维度」的地方；阈值按压测容量的 70% 设，超限返回 429 而不是排队。

| 位置 | 维度 | 优点 | 缺点 |
|---|---|---|---|
| 网关 / Nginx | IP、路由、租户 | 保护全局，成本低 | 拿不到业务维度 |
| 服务单机 | 接口、用户 | 简单、无网络开销 | 集群总量不准 |
| 服务集群 | 用户、租户 | 准确 | 依赖 Redis，成本高 |

```go
// 单机令牌桶：每秒 100 个，允许突发 20 个
limiter := rate.NewLimiter(rate.Limit(100), 20)

func Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

```nginx
# Nginx 入口限流：每 IP 每秒 20 个请求，突发 40
limit_req_zone $binary_remote_addr zone=perip:10m rate=20r/s;
limit_req_status 429;

server {
    location /api/ {
        limit_req zone=perip burst=40 nodelay;
        proxy_pass http://order_api;
    }
}
```

阈值怎么定：压测得到单实例 500 QPS 稳定，线上限流设 350（70%）；集群 4 实例，网关总量限 1400。

---

## 10.5 降级：提前写好预案

**结论先行**：降级不是「关功能」，而是「用更便宜的方式提供不完整但可用的服务」；必须提前写开关、提前演练。

| 类型 | 做法 | 例子 |
|---|---|---|
| 读降级 | 返回缓存/默认值 | 推荐服务挂了，返回热门榜单 |
| 写降级 | 转异步、排队 | 积分入账改为消息队列异步 |
| 功能降级 | 关掉非核心功能 | 大促关闭评论、晒单 |
| 依赖降级 | 跳过非必要校验 | 风控超时按低风险放行（要有额度上限） |

```go
func (s *Service) Recommend(ctx context.Context, uid int64) ([]Item, error) {
	items, err := s.recClient.List(ctx, uid)
	if err == nil {
		return items, nil
	}
	// 降级：记录指标，返回兜底数据，不把错误抛给用户
	s.metrics.Inc("recommend_fallback_total")
	slog.WarnContext(ctx, "recommend fallback", "err", err)
	return s.hotItems(ctx), nil
}
```

降级开关要满足三条：**能秒级生效、有明确负责人、有恢复条件**。只写在文档里的降级预案，等于没有。

---

## 10.6 隔离：别让一个依赖拖死全部

**结论先行**：隔离的本质是「给每个依赖划一块独立资源」，一个依赖慢，最多耗光自己那块。

| 隔离方式 | 做法 | 适用 |
|---|---|---|
| 连接池隔离 | 每个下游一个池，设上限 | 所有 RPC / DB 调用 |
| 并发隔离 | 每个依赖独立信号量/协程池 | 慢调用、批量任务 |
| 集群隔离 | 核心业务独立集群 | 大促、多租户 |
| 单元化 | 按用户分片到独立单元 | 超大规模、容灾 |

```go
// 每个下游一个带缓冲 channel 当信号量，最多 20 个并发
var paymentSem = make(chan struct{}, 20)

func (s *Service) Pay(ctx context.Context, id int64) error {
	select {
	case paymentSem <- struct{}{}:
		defer func() { <-paymentSem }()
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrPaymentBusy // 快速失败，不排队堆积
	}
	return s.payClient.Charge(ctx, id)
}
```

> 🔑 隔离的判据：**这个依赖挂 10 分钟，会不会影响其他接口？** 会，就还没隔离干净。

---

## 10.7 Go 落地：把五件套串成一个中间件

**结论先行**：入站用「限流 + 超时 + recover」，出站用「熔断 + 重试 + 超时」；两边都只做一次，不叠加。

```go
// 出站守卫：限流 → 熔断 → 重试（每次重试都带超时）
type Guard struct {
	br      *gobreaker.CircuitBreaker
	lim     *rate.Limiter
	timeout time.Duration
	retries int
}

func (g *Guard) Do(ctx context.Context, fn func(context.Context) error) error {
	if !g.lim.Allow() {
		return ErrRateLimited
	}
	_, err := g.br.Execute(func() (any, error) {
		return nil, retry(ctx, g.retries, func(ctx context.Context) error {
			c, cancel := context.WithTimeout(ctx, g.timeout)
			defer cancel()
			return fn(c)
		})
	})
	return err
}
```

```go
// 入站中间件：recover + 超时 + 限流，顺序不能反
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

func WithTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

失败决策顺序：

```text
请求失败
  ├─ 是 4xx（参数/权限/不存在）→ 不重试，直接返回
  ├─ 是超时/连接错误/5xx
  │    ├─ 接口幂等 → 退避重试 2 次
  │    │    └─ 仍失败 → 看熔断器状态
  │    └─ 非幂等 → 不重试，走补偿/对账
  ├─ 熔断已打开 → 直接走降级
  └─ 降级也没有 → 返回明确错误 + 告警
```

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 超时设 0 | 线程/连接被永久占用 | 每个调用都设，且逐层递减 |
| 重试不看幂等 | 重复下单、重复扣款 | 先判幂等，再决定重试 |
| 多层重试叠加 | 故障时流量放大 8 倍 | 只在最外层重试一次 |
| 熔断没有兜底 | 熔断后直接 500 | 配缓存/默认值/友好错误 |
| 限流阈值拍脑袋 | 要么误杀，要么没用 | 压测容量 × 70% |

---

## 本章自检

**Q1** 下游 P99 是 80ms，超时设 3 秒合理吗？
▸ 答案：不合理。3 秒意味着故障时每个请求要占住线程 3 秒，并发很快被耗尽。设 200–300ms（P99 的 2–3 倍），既覆盖正常抖动，又快速失败。

**Q2** 支付接口返回 504，为什么不能直接重试？
▸ 答案：504 说明网关等不到响应，但支付可能已经成功，重试会造成重复扣款。正确做法是查单确认，或用幂等键重试，而不是盲目重发。

**Q3** 熔断器一直停在打开状态，可能是什么原因？
▸ 答案：半开试探请求全部失败（下游没恢复），或 `ReadyToTrip` 的统计窗口/最小请求数配置导致无法转半开。先确认下游健康，再检查 `Interval`、`Timeout` 与半开请求数。
