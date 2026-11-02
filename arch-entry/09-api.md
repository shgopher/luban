# 第 9 章 接口与集成：契约先行

> **本章一句话**：接口先定契约再写代码，把错误、幂等、版本、重试四件事定死。

**读完你会：**
- 用一套规范评审任意一个 REST 接口，当场指出 URL、状态码、返回体的问题
- 给下单、支付、回调三类接口加幂等键，挡住重复提交与重复回调
- 给开放接口配上限流，给第三方对接补上超时、重试、对账、验签四道防线

---

## 9.1 REST 接口设计规范

**结论先行**：URL 只描述资源，动作交给 HTTP 方法。两条规则挡住 80% 的问题：资源用复数名词，层级不超过两层。

方法语义：

| 方法 | 语义 | 幂等 | 典型用途 |
|---|---|---|---|
| GET | 查询 | 是 | 列表、详情 |
| POST | 创建 / 动作 | 否 | 下单、批量查询 |
| PUT | 全量替换 | 是 | 覆盖整个配置 |
| PATCH | 局部更新 | 否 | 改一个字段 |
| DELETE | 删除 | 是 | 取消订单、删地址 |

PUT 幂等、PATCH 不幂等的判据只有一条：同样的请求打两次，最终状态是否一样。

状态码选择表，评审时最先被问的就是这张：

| 场景 | 状态码 | 补充 |
|---|---|---|
| 查询成功 | 200 | 带返回体 |
| 创建成功 | 201 | 带 `Location` 头指向新资源 |
| 更新 / 删除成功且无需回体 | 204 | 不返回 body |
| 参数缺失、JSON 解析失败 | 400 | 业务码 10001 |
| 未登录、token 过期 | 401 | 别在此处暴露账号是否存在 |
| 已登录但无权限 | 403 | 与 401 严格区分 |
| 资源不存在 | 404 | 别用 400 代替 |
| 状态冲突（重复提交、版本号旧） | 409 | 与 422 严格区分 |
| 参数格式对但业务校验不过 | 422 | 如「优惠券已过期」 |
| 超限流 | 429 | 必带 `Retry-After` |
| 未捕获异常 | 500 | 业务码 10500 |
| 依赖不可用、主动摘流 | 503 | 带 `Retry-After` |

好 / 坏 URL 对比：

| 坏的 | 好的 | 问题 |
|---|---|---|
| `GET /getUserList` | `GET /v1/users` | 动词进了 URL |
| `POST /user/create` | `POST /v1/users` | 单数 + 动词 |
| `POST /v1/users/1/delete` | `DELETE /v1/users/1` | 用 POST 装删除 |
| `GET /v1/users/1/orders/2/items` | `GET /v1/order-items?order_id=2` | 层级超两层 |
| `GET /v1/getOrderDetailById?id=1` | `GET /v1/orders/1` | 方法名进了路径 |
| `POST /v1/orders/1/pay` | 保留 | 动作型接口的例外 |

支付、取消、发货这类动作确实没有资源可表达，允许 `POST /v1/orders/{id}/pay`，但它必须幂等，见 9.3。

分页只有两种，按场景选：

| 场景 | 方式 | 参数 | 返回 |
|---|---|---|---|
| 后台列表，要总数、可跳页 | 页码 | `page`（从 1 起）、`size`（默认 20，上限 100） | `total`、`page`、`size` |
| C 端信息流，数据量大 | 游标 | `cursor`、`limit`（默认 20，上限 50） | `next_cursor`、`has_more` |

`page` 超过 1000 强制走游标，否则 `LIMIT 100000, 20` 会把数据库拖到 CPU 80%。

过滤参数用字段名本身，范围用 `_from` / `_to`，排序用 `sort`，前缀 `-` 表示降序：

```text
GET /v1/orders?status=paid&user_id=1001&created_from=2026-11-01&created_to=2026-11-02&sort=-created_at,id&page=1&size=20
```

命名全站统一 snake_case，query 和 body 都不破例。一半 camelCase 一半 snake_case，前端会为此写一层适配，然后这层适配永远删不掉。

批量查询用 POST，不用 GET 拼 id：

```text
POST /v1/orders/batch-get
{"ids": ["202611020001", "202611020002"]}
```

`GET /v1/orders?ids=1,2,3...` 会同时被三件事咬：URL 长度上限、缓存键爆炸、网关日志截断。批量写更严，一次最多 50 条，逐条返回，允许部分成功：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "succeeded": ["202611020001"],
    "failed": [{"id": "202611020002", "code": 20003, "message": "订单状态不允许取消"}]
  },
  "trace_id": "4f1c2a9e8b7d4c31"
}
```

---

## 9.2 错误码与返回体

**结论先行**：返回体只有一个形状 `{code, message, data, trace_id}`；HTTP 状态码管传输层的粗分类，业务码管具体原因，两层各管一段，不许互相顶替。

成功：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "order_id": "202611020001",
    "status": "pending_payment",
    "amount": 19900,
    "created_at": "2026-11-02T12:00:00+08:00"
  },
  "trace_id": "4f1c2a9e8b7d4c31"
}
```

失败，HTTP 409 配业务码 20003：

```json
{
  "code": 20003,
  "message": "订单状态不允许取消",
  "data": null,
  "trace_id": "4f1c2a9e8b7d4c31"
}
```

字段级校验错误塞进 `data`，别把结构撑成第五个字段：

```json
{
  "code": 10001,
  "message": "参数校验失败",
  "data": {"fields": [{"field": "phone", "reason": "格式不正确"}]},
  "trace_id": "4f1c2a9e8b7d4c31"
}
```

金额一律用「分」的整数，`19900` 就是 199 元。时间一律带时区偏移，`2026-11-02T12:00:00+08:00`，不要给 `2026-11-02 12:00:00`。

业务码 5 位数字，分段，每段留 1000 个号：

| 号段 | 归属 | 示例 |
|---|---|---|
| 0 | 成功 | 唯一表示成功 |
| 10xxx | 通用、框架 | 10001 参数错误、10401 未登录、10403 无权限、10429 限流、10500 系统错误 |
| 20xxx | 订单 | 20001 不存在、20003 状态冲突 |
| 21xxx | 支付 | 21001 支付单不存在、21002 重复支付 |
| 30xxx | 商品 | 30001 已下架 |
| 40xxx | 用户 | 40001 已注销 |

分工表：

| 层面 | 谁在读 | 管什么 | 例子 |
|---|---|---|---|
| HTTP 状态码 | 网关、监控、HTTP SDK、重试逻辑 | 传输与粗分类 | 401、409、429、503 |
| 业务码 | 业务前端、对接方 | 具体原因与文案分支 | 20003、10001 |

🔑 状态码必须与错误性质一致。全站返 200、body 里塞 `code=500`，网关认为请求成功，重试、告警、熔断全部不触发——这是最贵的一类省事。

`message` 只给人看。前端不许拿它做判断，否则改一次文案就挂一个页面。

### 坑

| 坑 | 后果 | 正确做法 |
|---|---|---|
| `message` 里塞 `err.Error()` | 泄露 SQL、表名、内网 IP、文件路径 | 固定文案；堆栈只进日志，用 `trace_id` 关联 |
| 5xx 返回具体异常 | 攻击者按报错信息试探边界 | 5xx 统一「系统繁忙，请稍后重试」 |
| 业务码直接用 400 / 500 | 分不清「缺 phone」和「缺 email」 | 5 位业务码分段 |
| 成功也返回 `code: 200` | 前端判断 `code === 200` 跟新接口的 0 打架 | 成功固定 `code: 0`，全站一致 |
| `trace_id` 只写进日志 | 用户报障时只能按时间捞 | 同时放 `X-Trace-Id` 响应头 |

---

## 9.3 幂等接口：下单、支付、回调

**结论先行**：幂等不是「防重复点击」，是「同一个请求到达 N 次，结果与 1 次相同」。靠客户端一个 UUID 加服务端一条唯一约束，别靠前端按钮置灰。

客户端在**进入下单页时**生成 `Idempotency-Key`（UUID v4），提交与每次重试都带同一个；用户重新下单才换新的。放到「点提交」时生成就晚了——超时重试那一次你已经没有 key 可复用。

三种场景的幂等键：

| 场景 | 幂等键 | 落在哪 | 有效期 |
|---|---|---|---|
| 下单 | 客户端 `Idempotency-Key` | 幂等记录表唯一索引 | 24h |
| 支付 | 商户订单号 `out_trade_no` | 支付单表唯一索引 | 永久 |
| 渠道回调 | 渠道事件 ID + 渠道标识 | 回调记录表唯一索引 | 30 天 |
| 退款 | 退款单号 | 退款表唯一索引 | 永久 |
| 内部服务调用 | 上游透传同一个 key | Redis `SET NX EX 86400` | 24h |

支付和退款用单号而不是 `Idempotency-Key`：单号天然唯一，对账时还能直接拿来比对。

### 可直接抄

服务端两条路：唯一索引（强，适合订单、支付）或 Redis `SETNX`（快，适合查询与短窗口）。下面是唯一索引版，幂等键与业务写在**同一个事务**里——提交才算占住，回滚就当没占过。

```sql
CREATE TABLE idem_record (
  idem_key   VARCHAR(64) NOT NULL,
  scope      VARCHAR(32) NOT NULL,
  resp_code  INT         NOT NULL,
  resp_body  JSON        NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (idem_key, scope)
) ENGINE=InnoDB;
```

```go
// idem.go —— Go 1.22+，标准库 + database/sql
package idem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// BizError 是确定性业务失败（状态冲突、校验不过），要写进幂等记录。
type BizError struct {
	Status int
	Body   any
}

func (e *BizError) Error() string { return "biz error" }

// IsDup 判断唯一键冲突。MySQL InnoDB 的错误号是 1062。
// 生产建议 errors.As 成 *mysql.MySQLError 再比 Number。
var IsDup = func(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Error 1062")
}

// BusinessFunc 在同一个事务内执行业务，返回状态码与响应体。
type BusinessFunc func(ctx context.Context, tx *sql.Tx, r *http.Request) (int, any, error)

type Idempotent struct {
	db    *sql.DB
	scope string // 业务域，如 "order.create"
	fn    BusinessFunc
}

func New(db *sql.DB, scope string, fn BusinessFunc) *Idempotent {
	return &Idempotent{db: db, scope: scope, fn: fn}
}

func (i *Idempotent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 64 {
		writeJSON(w, http.StatusBadRequest,
			map[string]any{"code": 10001, "message": "缺少或非法的 Idempotency-Key"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 1. 命中已完成的记录：原样回放，不再执行业务
	if status, body, ok := i.replay(ctx, key); ok {
		w.Header().Set("Idempotent-Replayed", "true")
		writeRaw(w, status, body)
		return
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w)
		return
	}
	defer tx.Rollback() // 已 Commit 时返回 ErrTxDone，忽略

	// 2. 同一事务里抢占幂等键：提交才算生效，回滚就当没占过
	_, err = tx.ExecContext(ctx,
		`INSERT INTO idem_record (idem_key, scope, resp_code, resp_body) VALUES (?, ?, 0, '{}')`,
		key, i.scope)
	if err != nil {
		// 3. 并发请求已提交：回放它的结果，而不是报冲突
		if IsDup(err) {
			if status, body, ok := i.replay(ctx, key); ok {
				writeRaw(w, status, body)
				return
			}
		}
		writeErr(w)
		return
	}

	status, resp, err := i.fn(ctx, tx, r)
	if err != nil {
		var be *BizError
		if !errors.As(err, &be) {
			writeErr(w) // 系统错误：回滚，键随事务消失，可用同一个键重试
			return
		}
		status, resp = be.Status, be.Body // 业务失败也是确定性结果，照样缓存
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		writeErr(w)
		return
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE idem_record SET resp_code = ?, resp_body = ? WHERE idem_key = ? AND scope = ?`,
		status, raw, key, i.scope); err != nil {
		writeErr(w)
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w)
		return
	}
	writeRaw(w, status, raw)
}

func (i *Idempotent) replay(ctx context.Context, key string) (int, []byte, bool) {
	var (
		status int
		body   []byte
	)
	err := i.db.QueryRowContext(ctx,
		`SELECT resp_code, resp_body FROM idem_record WHERE idem_key = ? AND scope = ?`,
		key, i.scope).Scan(&status, &body)
	if err != nil || status == 0 {
		return 0, nil, false
	}
	return status, body, true
}

func writeErr(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable,
		map[string]any{"code": 10500, "message": "系统繁忙，请稍后重试"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
```

不想上表的场景用 Redis，两条命令就够：

```bash
# Redis 7.2：占到返回 OK，没占到返回 (nil)
redis-cli -c SET idem:order:9f2c1a7b 1 NX EX 86400
# 处理完覆盖成响应快照，供后续重放
redis-cli -c SET idem:order:9f2c1a7b '{"code":0,"data":{"order_id":"202611020001"}}' EX 86400
```

⚠️ 两个请求同时到达、都查不到记录时，靠唯一索引兜底，不能靠「先查后插」。并发下「查-插」之间的窗口一定被踩穿，第二个请求会抛 duplicate，代码必须接住并回放第一个的结果，而不是返回 500。

---

## 9.4 版本与兼容：只加不改

**结论先行**：版本号是「不兼容的承诺」，不是迭代记录。没有破坏兼容就不许升版本号。

版本放哪里：

| 方式 | 写法 | 适合 | 代价 |
|---|---|---|---|
| URL 前缀 | `/v1/orders` | 对外开放接口，默认选这个 | 路径带版本，资源标识不纯 |
| Header | `X-API-Version: 2` | 内部服务 | 网关、日志、缓存都要读 Header，排障时看不见 |
| Query | `?version=2` | 不要用 | 污染缓存键，代理会丢参数 |
| GraphQL | —— | 本书不展开 | schema 演进需要专门的治理能力 |

选 URL 前缀的核心理由是可运维：网关能按 `/v1` 前缀直接路由，灰度切 10% 流量、出事切回旧版，都是一条 Nginx `location` 的事。

「只加不改」六条，按破坏性从高到低：

| 变更 | 兼容 | 做法 |
|---|---|---|
| 新增可选字段 | 兼容 | 直接加，响应里能不带就不带 |
| 新增必填请求字段 | 不兼容 | 给默认值，或开新版本 |
| 删除字段 | 不兼容 | 先标废弃，按调用量归零后再删 |
| 改字段类型（int → string） | 不兼容 | 新增 `order_id_str`，旧的留着 |
| 改字段语义（同名不同义） | 不兼容 | 最隐蔽的破坏，宁可变名 |
| 枚举加值 | 兼容 | 客户端必须有 default 分支 |
| 枚举删值 | 不兼容 | 只加不删 |
| 改错误码含义 | 不兼容 | 新增一个码，旧的冻结 |

客户端要容忍未知字段。Go 里 `encoding/json` 默认就忽略未知字段，别为了「严格」打开 `DisallowUnknownFields`——那是校验入参用的，用在解析响应上，对方加一个字段你就全站解析失败。枚举必须写 default 分支，对方加一个值你就 panic，这类事故一个月能见到一次。

上线前逐条打勾：

```text
[ ] 有没有删字段
[ ] 有没有改字段类型（int→string、decimal→float 都算）
[ ] 有没有把可选字段变必填
[ ] 有没有改枚举值或删枚举值
[ ] 有没有改字段语义（同名不同义）
[ ] 有没有改默认值（分页大小、排序、时区）
[ ] 有没有改已有业务码的含义
[ ] 旧版本还有多少调用量（按 app_version / User-Agent 统计）
[ ] 废弃字段有没有 Deprecation 与 Sunset 头
```

下线一个版本走三步：先发 `Deprecation: true` 和 `Sunset: Wed, 01 Apr 2026 00:00:00 GMT`，观察两周调用量，归零后再摘路由。出兼容事故时，把网关前缀切回 `/v1` 就是回滚，所以旧版代码至少保留两个发布周期。

---

## 9.5 对接第三方：四道防线

**结论先行**：对接第三方只信四件事——超时、重试、对账、验签。前三个防数据不一致，第四个防别人冒充。

| 防线 | 防什么 | 关键参数 | 漏了会怎样 |
|---|---|---|---|
| 超时 | 连接挂死、goroutine 堆积 | 连接 1–2s，读 3–5s（经验值） | 一次抖动拖垮整个服务 |
| 重试 | 网络抖动、对方瞬时 5xx | 3 次，退避 200ms / 600ms / 1.8s 加抖动 | 一次抖动就失败，客诉进群 |
| 对账 | 静默不一致（成功但没回调） | T+1 02:00 全量，每小时增量 | 少收了钱三天后才发现 |
| 验签 | 伪造请求、重放 | HMAC-SHA256 + 时间窗 300s + nonce | 任何人可伪造回调改单 |

超时要分开设：`http.Client.Timeout` 是总数，`DialContext` 是连接，`ResponseHeaderTimeout` 是等首字节。只设总数，DNS 卡住时你连重试的机会都没有。

```go
client := &http.Client{
	Timeout: 6 * time.Second, // 总数，含重定向与读 body
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ResponseHeaderTimeout: 3 * time.Second,
		MaxIdleConnsPerHost:   20,
	},
}
```

不设 `Timeout` 的 `http.Client` 默认永不超时，这是 Go 里最贵的一个默认值。

重试三条规则：只重试幂等接口；只重试超时、连接错误和 5xx（429 看 `Retry-After`）；退避加随机抖动，否则同一秒的失败会同时重试，把对方再压死一次。

对账别等到出事才做。每天 02:00 拉对方对账单，只比三个字段：单号、金额、状态。差异写进 `recon_diff` 表，专人每天早上处理 20 条以内还能收得干净；攒到几百条就没人看了。

### 可直接抄

验签的签名串固定五段，接收方用**原始 body 字节**重算——不能用解析后的结构体，JSON 字段顺序一变就验不过。

```go
// sign.go —— Go 1.22+，只用标准库
package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const maxSkew = 5 * time.Minute

var (
	ErrMissingHeader = errors.New("缺少签名头")
	ErrExpired       = errors.New("时间戳超出窗口")
	ErrBadSignature  = errors.New("签名不匹配")
)

// Sign 签名串：METHOD\nPATH\nTIMESTAMP\nNONCE\nSHA256(BODY)
func Sign(secret []byte, method, path, timestamp, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\n%s\n%s\n%s\n%s", method, path, timestamp, nonce, hex.EncodeToString(sum[:]))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify 用原始 body 校验签名。nonce 去重放在调用方（Redis SETNX 5 分钟）。
func Verify(secret []byte, r *http.Request, body []byte, now time.Time) error {
	ts := r.Header.Get("X-Sign-Timestamp")
	nonce := r.Header.Get("X-Sign-Nonce")
	got := r.Header.Get("X-Signature")
	if ts == "" || nonce == "" || got == "" {
		return ErrMissingHeader
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrMissingHeader
	}
	if d := now.Sub(time.Unix(sec, 0)); d > maxSkew || d < -maxSkew {
		return ErrExpired
	}
	want := Sign(secret, r.Method, r.URL.EscapedPath(), ts, nonce, body)
	if !hmac.Equal([]byte(want), []byte(got)) {
		return ErrBadSignature
	}
	return nil
}
```

比较用 `hmac.Equal`，不能用 `==`：`==` 在第一个不同字节就返回，逐字节比较的时间差可以被用来猜签名。验签通过后还要查 nonce，`SET nonce:{id} 1 NX EX 300` 返回 nil 就是重放，直接拒。

---

## 9.6 回调与 Webhook 的可靠做法

**结论先行**：Webhook 的可靠性不在「发出去」，在「对方确认收到并处理完」。你发要有重试和签名，你收要先回 2xx 再异步处理。

发送方五件事：

| 项 | 做法 |
|---|---|
| 签名头 | `X-Webhook-Signature`、`X-Webhook-Timestamp`、`X-Webhook-Event-Id`、`X-Webhook-Event-Type` |
| 先落库再发 | 事件表 `status=pending`，worker 扫描发送，成功置 `done`，进程挂了不丢 |
| 超时 | 连接 2s、读 3s，接收方必须快 |
| 重试 | 1m / 5m / 30m / 2h / 6h / 24h，累计 24h 进死信并告警 |
| 只重试该重试的 | 5xx、超时、连接失败重试；4xx 一律不重试（429 除外） |

事件表至少这几列：`event_id`、`event_type`、`payload`、`status`、`attempts`、`next_retry_at`。有了 `next_retry_at`，重试就不依赖内存里的定时器，重启后接着发。

接收方五件事：先验签，再落库，立刻返回 `{"code":0}`，业务处理丢进 MQ，总耗时压在 500ms 内。对方超时通常 3–5s，你要留足余量。

按 `event_id` 去重：唯一索引，冲突就返回 200 并记一笔重复投递。不能返回 409 或 400——返回 4xx 对方会认为你没收到，继续重试。

乱序一定会发生，因为重试会插队。事件带 `occurred_at` 和 `sequence`，状态机只允许向前迁移（`pending → paid → shipped`），旧事件到达时丢弃并打点计数。用「最后写入者获胜」处理乱序，等于让旧状态覆盖新状态。

回调失败排查：

| 现象 | 先查 | 大概率原因 | 处置 |
|---|---|---|---|
| 对方说没收到 | 发送日志里有没有你的 `event_id` | 地址配错、防火墙没放行 | 让对方加白你的出口 IP |
| 收到但验签失败 | 签名串拼接顺序、body 有没有被网关改写 | 网关压缩或转码，签名用了改写后的 body | 签名只认原始字节，先读 body 再解析 |
| 同一事件一直重试 | 事件表 `status` 与 `attempts` | 对方同步处理超时或返 500 | 让对方先回 200 再异步；你确认自身幂等 |
| 事件丢了 | `status=pending` 且 `next_retry_at` 已过期 | worker 挂了、没有补偿扫描 | 加扫描任务，超过 5 分钟未发的重投 |
| 顺序错乱 | 事件 `occurred_at`、`sequence` | 并行消费、重试插队 | 状态机只前进，旧事件丢弃 |
| 重复处理 | 去重表有没有唯一索引 | 去重写在事务外、索引缺失 | 唯一索引 + 捕获冲突后返 200 |

---

## 9.7 开放接口的限流与配额

**结论先行**：限流防「打垮系统」，按秒算；配额防「超出套餐」，按月算。两者拒绝时都必须告诉对方等多久。

| 维度 | key | 典型阈值（经验值） | 用在哪 |
|---|---|---|---|
| 应用 | `X-App-Key` | 100 r/s | 开放平台按应用分发 |
| 租户 | `tenant_id` | 500 r/s | SaaS 防单个租户独占 |
| IP | `$binary_remote_addr` | 20 r/s | 未登录接口、防爬 |
| 用户 | `user_id` | 5 r/s | 短信、下单防刷 |

拒绝时三样一起给，缺一样对方就只能靠猜：

```text
HTTP/1.1 429 Too Many Requests
Retry-After: 1
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1762041600
X-RateLimit-Policy: 100;w=1
```

200 响应也要带 `X-RateLimit-Remaining`，让对方自己节流，别等撞墙。

Nginx 1.26 上验证过的配置，注意两个点：`limit_req_zone` 只能用变量做 key，`map` 是给空 appkey 兜底的；`error_page` 是让 429 也返回 JSON 的，不配就返回 HTML 错误页，SDK 解析直接失败。

```nginx
worker_processes auto;

http {
    # 空 appkey 落到 IP 桶，否则所有无 appkey 的请求共用一个桶
    map $http_x_app_key $rl_key {
        default $http_x_app_key;
        ""      $binary_remote_addr;
    }

    limit_req_zone $rl_key                 zone=api_app:10m rate=100r/s;
    limit_req_zone $binary_remote_addr     zone=api_ip:10m  rate=20r/s;
    limit_req_status 429;

    server {
        listen 8080;

        location /v1/ {
            limit_req zone=api_app burst=200 nodelay;
            limit_req zone=api_ip  burst=40  nodelay;
            limit_req_status 429;

            add_header X-RateLimit-Limit 100 always;
            add_header Retry-After 1 always;

            proxy_set_header X-Real-IP $remote_addr;
            proxy_pass http://backend;
            error_page 429 = @rl_json;
        }

        location @rl_json {
            internal;
            default_type application/json;
            add_header X-RateLimit-Limit 100 always;
            add_header Retry-After 1 always;
            return 429 '{"code":10429,"message":"请求过于频繁，请稍后重试"}';
        }
    }
}
```

验证：`nginx -t` 过之后用 1r/s burst=2 的小参数打 5 次，前三返回 200，第四、第五返回 429 并带 `Retry-After`。

配额是另一套数：`INCRBY quota:{tenant}:202611 1`，首次 `EXPIREAT` 到月末，超额同样返 429，用 `X-Quota-Limit` 与 `X-Quota-Remaining` 跟每秒限流区分开，别让调用方分不清「等一秒」还是「等下个月」。

💡 限流器依赖的 Redis 挂了怎么办：判断标准是「放行会不会打死你」。查询接口 fail-open（放行并告警），写接口和支付 fail-closed（直接 503）。这个开关要写进配置中心，出事能一键切换。

---

## 本章自检

**Q1** 下单接口加了 `Idempotency-Key` 和唯一索引，用户投诉「重试一次，订单金额变了」。问题在哪？
▸ 答案：key 在「点提交」时生成，重试路径重新生成了新 key，等于两次不同请求。key 必须在进入下单页时生成并挂在请求上下文里，重试复用同一个；同时确认唯一索引建在 `(idem_key, scope)` 上而不是只建在订单号上。

**Q2** 全站返 HTTP 200、body 里塞 `code=500`，三个月后网关的重试和熔断为什么全失效？
▸ 答案：网关按状态码分流。200 被计入成功，5xx 重试、错误率告警、熔断阈值都不触发，故障只能靠人工发现。状态码必须与错误性质一致，业务码只做细分类。

**Q3** 对方说回调发了 3 次，你只处理了 1 次，而且状态是旧的。先查哪三件事？
▸ 答案：① 去重键是不是用了业务单号而不是 `event_id`，把合法的新事件误判成重复；② 返回 200 之前有没有先落库，先回后处理的窗口里进程重启就丢了；③ 状态机是否允许回退，旧事件把新状态覆盖了。
