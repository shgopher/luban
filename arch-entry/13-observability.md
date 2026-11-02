# 第 13 章 可观测性：出事 5 分钟内定位

> **本章一句话**：指标发现出事，链路定位到服务，日志定位到代码，三板斧按这个顺序用。

**读完你会：**
- 配一套能上生产的 Go 结构化日志，每条带 trace_id
- 写出 8 条必配告警的 PromQL、阈值和 `for` 时长
- 按五步 SOP，把一次线上故障在 5 分钟内定位到代码

---

## 13.1 日志、指标、链路：各管什么

**结论先行**：三支柱不是三套工具，是三个问题。指标答"是不是出事了"，链路答"慢在哪一跳"，日志答"到底发生了什么"。查故障的顺序永远是 指标 → 链路 → 日志。

| 支柱 | 回答什么 | 成本量级 | 采样策略 | 保留期 |
|---|---|---|---|---|
| 日志 | 发生了什么，事后取证 | 1000 QPS 约 70GB/天，压缩后 7GB/天 | 错误全采，成功 1%–10% | 7–30 天 |
| 指标 | 趋势与告警，预聚合后极便宜 | 2000 序列约 1–2GB/月 | 不采样，固定 15s 一点 | 6–13 个月 |
| 链路 | 跨服务耗时归因，谁拖慢了谁 | 100% 采样约 50GB/天，1% 约 1GB/天 | 头部 1%–10%，错误全采 | 3–7 天 |

成本差 2–3 个数量级，所以三者的用法完全不同：**指标用来"每 15 秒扫一遍全部服务"，日志用来"盯着一个请求看细节"**。拿日志做告警，凌晨三点会被刷屏；拿指标查根因，永远差最后一公里。

一个问题该先看哪个：

| 你的问题 | 先看 | 原因 |
|---|---|---|
| 用户说下单报错，不知道多严重 | 指标 | 先看错误率曲线，确定影响面和时间点 |
| 某个接口变慢，慢在哪 | 指标 → 链路 | P99 定位服务，trace 定位那一跳 |
| 偶发 500，复现不了 | 日志 | 按 trace_id 捞全链路日志 |
| 下游说我们超时 | 链路 | 看我们发出的那个 client span 的实际耗时 |
| 系统现在有没有问题 | 指标 | 只有指标便宜到能全量常扫 |
| 容量还够不够 | 指标 | 看饱和度（连接池、磁盘、CPU）的趋势 |
| 这个请求走了哪些服务 | 链路 | 一条 trace 就是一张实时调用图 |

💡 三跳铁律：每条告警能点开看板，每个看板能下钻到 trace，每条 trace 能跳到这批日志。三跳有一跳走不通，这套可观测性就是摆设。

---

## 13.2 日志规范：结构化与 trace_id

**结论先行**：生产日志只留一种格式——单行 JSON，字段名固定，每条都带 trace_id。彩色文本日志只在本地开发用。

字段就这些，多一个都别加：

| 字段 | 类型 | 说明 |
|---|---|---|
| `ts` | string | RFC3339 带毫秒，统一 UTC，别用本地时间 |
| `level` | string | `DEBUG` `INFO` `WARN` `ERROR` `FATAL` |
| `service` `env` | string | 服务名、`prod`/`staging`，进程启动时写死 |
| `trace_id` `span_id` | string | 32 位 / 16 位十六进制，从 context 取 |
| `msg` | string | 常量字符串，不带变量，方便按 msg 聚合 |
| `error` | string | `err.Error()`，不要塞整个结构体 |
| `http.method` `http.path` `http.status` `duration_ms` | 混合 | `http.path` 用路由模板 `/api/orders/{id}`，不用真实路径 |
| `user_id` | string | 脱敏后的值，手机号只留前 3 后 4，密码/token/身份证一律不落 |

分级标准（判据是"要不要人来看"，不是"严不严重"）：

| 级别 | 什么时候用 | 量级 |
|---|---|---|
| `DEBUG` | 只有排障时才想看的中间变量，生产默认关闭 | 开关打开时才会产生 |
| `INFO` | 每个请求一条访问日志 + 关键业务节点（下单成功、回调收到） | 最大，必须采样 |
| `WARN` | 能自愈的异常：重试第 1 次成功、降级命中、慢查询超 500ms | 量小，全采 |
| `ERROR` | 请求失败且用户可感知，需要人介入 | 全采 |
| `FATAL` | 进程无法继续（配置加载失败、端口被占），打完就 `os.Exit(1)` | 极少 |

三条禁令：不打印请求/响应全量 body（单个订单 JSON 可能有几十 KB，一天能打爆磁盘）；不打印密码、token、Cookie、身份证、银行卡；不打印 SQL 的完整参数（`db.statement` 只留模板）。

采样按 trace_id 做一致性哈希：同一个请求的所有日志要么全留、要么全丢，否则你会拿到半条链路。

```go
import (
	"hash/fnv"
	"log/slog"
)

// 错误全采，成功按比例采，保证同一 trace 的日志同进同出
func ShouldLog(traceID string, level slog.Level) bool {
	if level >= slog.LevelWarn {
		return true
	}
	h := fnv.New32a()
	h.Write([]byte(traceID))
	return h.Sum32()%100 < 5 // 成功请求留 5%
}
```

### 可直接抄

Go 1.21+，标准库 `log/slog`，不引第三方：

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"time"
)

type traceKey struct{}

// WithTraceID 由中间件在入口调用，之后全链路只传 ctx
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}

// traceHandler 把 ctx 里的 trace_id 自动补进每条日志，业务代码不用手写
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := ctx.Value(traceKey{}).(string); ok {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(as)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

func NewLogger(service, env, level string) *slog.Logger {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		lv = slog.LevelInfo // 配错就退回 INFO，不要让服务起不来
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Key = "ts" // 字段名统一成 ts，检索系统里少一次映射
				a.Value = slog.TimeValue(a.Value.Time().UTC().Truncate(time.Millisecond))
			}
			return a
		},
	})
	return slog.New(traceHandler{h}).With("service", service, "env", env)
}

func main() {
	log := NewLogger("order-service", "prod", "info")
	slog.SetDefault(log)

	ctx := WithTraceID(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736")
	log.InfoContext(ctx, "order created",
		"user_id", "u_8f3c1a", "duration_ms", 132)

	// error 用 String 传 err.Error()，避免 JSON Handler 对自定义错误序列化出 {}
	log.ErrorContext(ctx, "create order failed",
		"error", "inventory: deadline exceeded",
		"http.method", "POST", "http.path", "/api/orders", "http.status", 500,
	)
}
```

输出一行一条，直接进 Loki / ES / ClickHouse：

```json
{"ts":"2026-11-02T04:11:25.127Z","level":"INFO","msg":"order created","service":"order-service","env":"prod","user_id":"u_8f3c1a","duration_ms":132,"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}
```

---

## 13.3 指标：四个黄金信号与八个告警

**结论先行**：指标只干两件事——看趋势、触发告警。所以只记四个黄金信号，其余按需加；告警只报症状，不报原因。

| 黄金信号 | 指标定义 | 看哪个值 | 经验阈值 |
|---|---|---|---|
| 延迟 | 请求耗时分布 | **P99**，绝不用平均值 | P99 < 200ms（对外接口） |
| 流量 | QPS | 按 route 拆分 | 与 1 小时前比，跌 50% 就是事故 |
| 错误 | 5xx 占比 | 按 status + route 拆 | < 0.1% 正常，> 1% 告警 |
| 饱和度 | 最紧张的那个资源 | 连接池使用率、磁盘使用率 | 任一 > 80% 就该扩容了 |

RED 管请求，USE 管资源，别混：

| 方法 | 适用对象 | 三个字母 |
|---|---|---|
| RED | 请求型服务（HTTP、gRPC） | Rate 请求速率、Errors 错误率、Duration 耗时 |
| USE | 资源（CPU、内存、磁盘、连接池） | Utilization 使用率、Saturation 饱和度（排队）、Errors 错误数 |

八个必配告警（阈值是经验值，按你的 SLO 微调）：

| 告警 | 判据 | 阈值 | `for` | 级别 |
|---|---|---|---|---|
| 5xx 错误率 | 5xx / 总请求 | > 1% | 5m | P1 |
| P99 延迟 | 服务级 P99 | > 500ms | 5m | P1 |
| QPS 突降 | 与 1 小时前比 | 跌 > 50% | 5m | P1 |
| 依赖错误率 | 单个下游的失败率 | > 5% | 5m | P1 |
| DB 连接池 | 已用 / 上限 | > 80% | 10m | P2 |
| MQ 积压 | 消费组 lag | > 10 万条或持续上涨 | 10m | P2 |
| 磁盘使用率 | 可用 / 总量 | < 15% | 10m | P2 |
| 证书过期 | 剩余天数 | < 14 天 | 1h | P2 |

```promql
# 5xx 错误率（按服务）
sum by (service) (rate(app_http_requests_total{status=~"5.."}[5m]))
  / sum by (service) (rate(app_http_requests_total[5m]))

# P99 延迟：histogram_quantile 必须带 sum by (le)，否则分位数是错的
histogram_quantile(0.99,
  sum by (service, le) (rate(app_http_duration_seconds_bucket[5m])))

# QPS 突降：offset 1h 对比，避免早高峰误报
sum by (service) (rate(app_http_requests_total[5m]))
  < 0.5 * sum by (service) (rate(app_http_requests_total[5m] offset 1h))

# 依赖错误率
sum by (dep) (rate(app_dependency_requests_total{status="error"}[5m]))
  / sum by (dep) (rate(app_dependency_requests_total[5m]))

# DB 连接池使用率
db_pool_in_use{service="order-service"} / db_pool_max{service="order-service"}

# MQ 积压（kafka_exporter 指标）
sum by (topic) (kafka_consumergroup_lag{group="order-consumer"})

# 磁盘使用率
node_filesystem_avail_bytes{mountpoint="/", fstype!~"tmpfs|overlay"}
  / node_filesystem_size_bytes{mountpoint="/", fstype!~"tmpfs|overlay"} < 0.15

# 磁盘将在 4 小时内打满（比使用率更早报警）
predict_linear(node_filesystem_avail_bytes{mountpoint="/"}[6h], 4*3600) < 0

# 证书剩余时间（blackbox_exporter）
probe_ssl_earliest_cert_expiry - time() < 14 * 86400
```

⚠️ 直方图桶必须覆盖你的 SLO：`[]float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5}`。桶里没有 200ms 这一档，`histogram_quantile(0.99)` 就只能在线性插值里瞎猜，报出来的 P99 会偏差一倍以上。

---

## 13.4 链路追踪：最小接入

**结论先行**：最小接入是三处改动——装一个 `otelhttp` 中间件、配一个 OTLP exporter、设 5% 采样率。半天能接完，别立项。

透传靠 W3C `traceparent` 头，格式固定四段：

```text
00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
```

| 段 | 位置 | 长度 | 说明 |
|---|---|---|---|
| 版本 | 第 1 段 | 2 | 固定 `00` |
| trace_id | 第 2 段 | 32 hex | 全链路唯一，16 字节 |
| span_id | 第 3 段 | 16 hex | 本跳唯一，8 字节 |
| flags | 第 4 段 | 2 hex | `01` 表示已采样 |

规则只有四条：入口没有 `traceparent` 就新生成；有就透传，不改；每跳生成新 `span_id`；响应头回写 `X-Trace-Id`，用户报障时报这一串就能直达日志。

```go
package main

// go 1.22+
// go get go.opentelemetry.io/otel@latest \
//        go.opentelemetry.io/otel/sdk@latest \
//        go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest \
//        go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@latest

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func initTracer(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint), // collector:4318，不带 http://
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(newResource()),
		// 头部采样 5%：父子一致，保证一条 trace 不会缺半截
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.05))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

func newResource() *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("order-service"),
		attribute.String("deployment.environment", "prod"),
	)
}

// traced 每个路由单独包一层：span 名就是路由模板，不会出现 /orders/88123 这种高基数
func traced(pattern string, h http.HandlerFunc) http.Handler {
	return otelhttp.NewHandler(h, pattern)
}

func main() {
	ctx := context.Background()
	shutdown, err := initTracer(ctx, "127.0.0.1:4318")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(c) // 停机前必须 flush，否则最后一批 span 丢
	}()

	mux := http.NewServeMux()
	mux.Handle("POST /api/orders", traced("POST /api/orders", createOrder))
	_ = http.ListenAndServe(":8080", mux)
}

func createOrder(w http.ResponseWriter, r *http.Request) {
	// 下游调用用 r.Context()，traceparent 自动注入请求头
	_, span := otel.Tracer("order").Start(r.Context(), "order.create")
	defer span.End()
	span.SetAttributes(attribute.String("order.channel", "app"))
	w.WriteHeader(http.StatusOK)
}
```

采样策略：头部采样 1%–10%（QPS 1000 以上取 1%，以下取 10%）。错误请求想全采，靠 OTel Collector 的尾部采样——但别自己写，先接受"错误请求可能没 trace"，用日志兜底，等链路系统稳定了再上尾部采样。

span 命名规范，一条都不许破：

| 类型 | 命名 | 反例 |
|---|---|---|
| HTTP 入口 | `POST /api/orders` | `POST /api/orders/88123`（高基数） |
| RPC 调用 | `inventory.GetStock` | `call inventory` |
| 数据库 | `db.query orders.select_by_id` | `SELECT * FROM orders WHERE ...` |
| 缓存 | `cache.get user:{id}` | `redis GET user:88123` |
| 消息 | `mq.publish order.created` | `kafka send` |

不采集的：URL 里的 query 参数、请求体、带参数的 SQL、Cookie、Authorization 头。`http.url` 只留 path 模板，否则用户的手机号就进了链路系统。

---

## 13.5 看板：三层就够

**结论先行**：一个服务三个看板，每层 6–9 个图，值班只看第一屏。超过 20 个图的看板，等于没有看板。

| 层 | 回答什么 | 图（6–9 个） | 谁看 |
|---|---|---|---|
| 业务 | 用户有没有受到影响 | 订单量、支付成功率、下单失败数、各渠道分布、GMV、注册数 | 老板、值班 |
| 应用 | 是哪个服务、哪个接口出问题 | QPS、P99/P95、错误率、依赖延迟、依赖错误率、goroutine 数、GC 暂停 | 研发、值班 |
| 资源 | 是不是资源打满了 | CPU、内存、磁盘、网络、连接数、FD、load、Pod 重启次数 | SRE |

每层 6–9 个图，是一屏能看完的量。放 20 个，值班的人会直接关掉。

看板上的每张图，必须能回答一个具体问题，答不上就删：

1. 现在有没有问题？（错误率、P99 曲线，看的是当前值 vs 昨天同时段）
2. 影响多大？（按 route / 地域 / 版本拆分的错误数，绝对值，不是百分比）
3. 什么时候开始的？（曲线拐点，和发布记录对齐）
4. 是全面挂了还是部分挂了？（多实例曲线是否一致，一个实例高就是单机问题）
5. 是变慢还是变错？（P99 和错误率两条线一起看）
6. 是不是下游的问题？（依赖延迟和依赖错误率）
7. 是不是资源不够？（饱和度和请求量的关系：量没涨、饱和度涨＝泄漏）
8. 现在要不要扩容/回滚？（第一屏的四个数字就够做这个决定）

看板顶部固定一行变量：`env` / `service` / `instance`，三秒能切到出问题的那个。

---

## 13.6 告警设计：症状优先

**结论先行**：只对"用户能感知到的现象"告警：错、慢、没流量。CPU 高、GC 频繁、连接池涨，这些都是原因——原因进看板，不进告警。

| 类型 | 例子 | 该不该告警 |
|---|---|---|
| 症状 | 5xx 错误率 3%、P99 2.3s、QPS 跌 60% | 该，用户已经在骂了 |
| 原因 | CPU 85%、GC 频繁、连接池 80% | 不该，先看它有没有变成症状 |
| 预警 | 磁盘 4 小时打满、证书 7 天过期 | 该，这是唯一值得例外的原因告警 |

`for: 5m` 是硬性要求。没有 `for`，你会在每次发布、每次缓存预热、每次 GC 抖动时收到告警，一周之内所有人都会把告警群静音——那比没有告警更危险。

分级与响应时限：

| 级别 | 判据 | 响应 | 恢复目标 | 通知方式 |
|---|---|---|---|---|
| P0 | 全站不可用或资金损失 | 5 分钟内 | 30 分钟 | 电话 + 群 |
| P1 | 核心接口错误率 > 1% 或 P99 > 500ms | 15 分钟内 | 2 小时 | 群 + @值班 |
| P2 | 资源预警、非核心功能降级 | 次日工作时间 | 3 天 | 群消息 |

每条告警必须带两个链接，缺一不可：

```yaml
groups:
  - name: order-service-symptoms
    rules:
      - alert: HighErrorRate
        expr: |
          sum by (service) (rate(app_http_requests_total{status=~"5.."}[5m]))
          /
          sum by (service) (rate(app_http_requests_total[5m])) > 0.01
        for: 5m
        labels:
          severity: P1
        annotations:
          summary: "{{ $labels.service }} 5xx 错误率 {{ $value | humanizePercentage }}"
          dashboard: "https://grafana.example.com/d/order-svc"
          runbook: "https://wiki.example.com/runbook/high-error-rate"
```

收敛三件套，不开就是告警风暴：

| 手段 | 配置 | 解决什么 |
|---|---|---|
| 分组 | `group_by: [alertname, service]` | 20 个实例同时挂，只发 1 条 |
| 抑制 | 入口故障时抑制下游所有告警 | 一条根因引发 200 条告警 |
| 静默 | 发布窗口 15 分钟静默 | 发布期抖动淹没真故障 |

坏告警与好告警的差别：

| 坏告警 | 为什么坏 | 好告警 |
|---|---|---|
| CPU > 80% | 阈值型原因告警，半夜把你叫醒，服务可能一切正常 | P99 > 500ms 持续 5 分钟 |
| 某个实例 down | 20 个实例挂 1 个不影响用户，但会发 20 条 | 可用实例数 < 总数的 50% |
| 错误数 > 10 | 不用比例，低峰期 10 个错误就是灾难，高峰期 10 个是噪声 | 错误率 > 1% |
| 没有 for | 每次发布都响，一周后被静音 | `for: 5m` |
| 告警文案只有"接口异常" | 收到后还得自己找看板，浪费 3 分钟 | 摘要带服务名、当前值、看板链接、runbook |
| 一个指标配 8 条告警 | 同一次故障响 8 次，值班直接忽略 | 一条告警，看板里看细节 |

---

## 13.7 排障 SOP：从告警到根因五步

**结论先行**：五步固定顺序，每步 1 分钟时间盒：影响面 → 变更 → 依赖 → 资源 → 代码。顺序反了，你会花 20 分钟证明"不是我们的问题"。

| 步 | 动作 | 看什么 | 时间盒 |
|---|---|---|---|
| 1 确认影响面 | 哪些接口、用户、地域受影响 | 按 route / region / version 拆的错误率与 P99 | 1 分钟 |
| 2 看最近变更 | 发布、配置、DB 变更、流量、开关 | 发布记录和告警时间点对齐 | 1 分钟 |
| 3 看依赖 | 下游错误率、延迟、超时 | 依赖看板、client span 耗时 | 1 分钟 |
| 4 看资源 | CPU、内存、连接、磁盘、GC | 资源层看板，对比 1 小时前 | 1 分钟 |
| 5 定位到代码 | 最慢 span + 该 trace 的日志 | trace 瀑布图、按 trace_id 查日志 | 1 分钟 |

第 1 步的判据是绝对值不是百分比：错误率 0.8% 看着不高，但如果只打挂了 `/api/orders/create`，那就是 100% 的下单失败。先回答"多少用户在受影响"，再回答"为什么"。

**止血优先于定位**。第 2 步一旦发现有变更，且影响面在扩大，直接回滚——回滚 2 分钟，定位可能要 20 分钟。回滚和定位可以并行，但顺序不能反。

一个真实风格的案例（订单服务 P99 从 120ms 涨到 2.3s）：

| 时间 | 动作 | 看到什么 |
|---|---|---|
| 14:02 | 收到 P1 告警：`order-service` P99 > 500ms | 看板显示 P99 2.3s，错误率 0.8%，QPS 正常 |
| 14:03 | 第 1 步：按 route 拆 | 只有 `POST /api/orders` 变慢，其他接口正常，影响约 8% 的请求 |
| 14:04 | 第 2 步：看变更 | 13:56 发布 v2.14.0，改动是"库存扣减逻辑重构" |
| 14:05 | 第 3 步：看依赖 | `inventory-service` QPS 从 300 涨到 5200，P99 从 30ms 到 1.8s |
| 14:06 | 第 4 步：看资源 | 订单服务 CPU 45% 正常；库存服务 DB 连接池 100%，CPU 85% |
| 14:07 | 第 5 步：看 trace | 一个下单请求 32 个 span，其中 32 次串行 `inventory.GetStock` |
| 14:08 | 定位 | `GetStockBatch(skus)` 被改成了 `for sku := range skus { GetStock(sku) }`，N+1 |
| 14:09 | 决策：回滚 | 影响面在扩大，先止血，代码明天修 |
| 14:13 | 回滚完成 | P99 回到 130ms，库存服务 QPS 回落到 310 |
| 14:20 | 复盘 | 金丝雀阶段只要看一眼依赖 QPS，1% 流量就能拦住这个改动 |

### 坑

排障体系自己最常见的六个坑，每一个都会把 5 分钟变成 5 小时：

| 坑 | 后果 | 正确做法 |
|---|---|---|
| 日志打爆磁盘 | 磁盘满，进程写不进去，服务假死；排障时恰好没有日志 | 单条日志 < 2KB，不打印 body；按天轮转保留 7 天；磁盘 85% 就告警 |
| 高基数指标 | 把 `user_id`、`order_id`、原始 URL 做 label，序列数爆炸，Prometheus OOM | label 只用低基数维度：`service`、`route`、`status`、`region`，基数控制在 100 以内 |
| trace 采样率 100% | span 存储一天几十 TB，链路系统先挂，故障时查不到 trace | 头部采样 1%–10%，错误靠日志兜底，别一上来就全采 |
| 告警风暴 | 一次故障 200 条告警，真问题被淹没，值班开始静音整个群 | 分组 + 抑制 + 静默三件套；根因告警抑制下游告警 |
| 没有 trace_id 串联 | 日志、链路、指标三套系统各查一遍，靠时间戳猜 | 入口生成 trace_id，注入 ctx，日志自动带，响应头回写 |
| 只配原因告警 | CPU 告警响了一晚上，用户其实没受影响；真出事时反而没告警 | 症状告警（错/慢/没流量）为主，原因指标只做看板和预警 |

---

## 本章自检

**Q1** 服务 P99 从 120ms 涨到 2.3s，但 CPU、内存、错误率全部正常。按 SOP，你的前三步分别看什么？
▸ 答案：① 按 route/region 拆，确认是全部接口还是单个接口变慢（影响面）；② 对齐最近 1 小时的发布、配置、DB 变更（变更）；③ 看依赖的错误率、延迟和 client span 耗时（依赖）。资源正常恰恰说明问题在依赖或代码，不在机器。

**Q2** 为什么 `histogram_quantile(0.99, rate(x_bucket[5m]))` 算出来的 P99 会偏？
▸ 答案：缺 `sum by (le)`，多个实例的桶被当成一组乱聚合，分位数失去意义；桶边界没覆盖 200ms 这类关键档位时，只能线性插值猜，偏差可达一倍。正确写法是 `histogram_quantile(0.99, sum by (service, le) (rate(...)))`，且桶里必须有你的 SLO 值。

**Q3** 你已经配了"CPU > 80%"和"连接池 > 80%"两条告警，为什么说这两条大概率是坏告警？
▸ 答案：它们是原因告警，不是症状。CPU 85% 时服务可能完全正常，但会在发布、批处理、GC 抖动时反复叫醒人，一周内告警就被静音。改成症状告警（错误率 > 1%、P99 > 500ms，均带 `for: 5m`），原因指标留在看板上供排障时下钻；唯一值得保留的原因是"磁盘 4 小时内打满"这类不可逆的预警。
