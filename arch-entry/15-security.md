# 第 15 章 安全基线：别在评审时被问倒

> **本章一句话**：评审只问六件事——认证怎么选、漏洞怎么防、密钥放哪、数据怎么分级、越权怎么挡、上线查什么。

**读完你会：**
- 用一张表选出 Session / JWT / OIDC，并说清各自的代价
- 给六类高频漏洞各配一条防线，Go 代码可直接替换
- 拿 14 项清单贴进发布单，上线前逐项打勾

---

## 15.1 认证与授权：三种方案怎么选

**结论先行**：三种方案只解决"你是谁"，"你能干什么"必须在业务服务里做。选型只看三件事：有没有第三方登录、要不要跨多端、能不能接受"注销有延迟"。

先分清两个词：**认证**证明身份，**授权**判断权限。评审时把这两个词混着说，后面所有设计都会歪。

- **Session**：登录后服务端存一条记录，浏览器只拿一个不可猜的 session_id。状态在服务端，删掉记录就等于立刻下线。适合 Web 单体、后台管理系统。
- **JWT**：服务端签发、客户端持有、服务端不存。多个服务共用签名公钥就能各自验签，适合服务间调用、开放 API、移动端。代价是**签出去就收不回**：token 过期之前，封号、改密码、踢下线都不生效。
- **OAuth2 / OIDC**：用于"用微信/钉钉/Google 登录"。拿到第三方身份后**仍要在自己的用户中心建账号、签自己的会话**，别把第三方 token 直接发给前端当登录态。

| 方案 | 状态存哪 | 注销 / 踢人 | 跨域与多端 | 移动端 | 实现成本 |
|---|---|---|---|---|---|
| Session | 服务端 Redis/DB | 删记录，秒级生效 | 要 Cookie 域 + `SameSite=None; Secure` + CORS 带凭证 | 弱，需自管 Cookie | 低，框架自带 |
| JWT | 客户端 | 等过期，或加黑名单/版本号 | 天然跨域，走 `Authorization` 头 | 强 | 中，签发/刷新/吊销都要自己写 |
| OAuth2/OIDC | 第三方 + 本地会话 | 第三方退出后本地还要自己删 | 依赖回调域名备案 | 强，走系统浏览器 | 中高，接入 + 对账 |

JWT 的"收不回"不是无解，标准做法是**双 token**：access token 只活 15 分钟，refresh token 存服务端（Redis，7–30 天）并可随时删除。封号时删 refresh token，最迟 15 分钟后全线失效。要在 15 分钟内生效，再加一列"用户 token 版本号"，验签后比对版本，代价是多一次 Redis 读。

**推荐架构：用户中心 + 网关统一鉴权。** 业务服务不解析 token，只认网关注入的身份头。

| 组件 | 只做 | 绝不做 |
|---|---|---|
| 客户端 | 带 access token 或 Cookie | 不判断权限 |
| 网关 | 验签、查过期、限流、剥离外部 `X-User-Id` 后重新注入 | 不做资源归属校验 |
| 用户中心 | 签发、刷新、吊销、维护权限点 | 不代理业务流量 |
| 业务服务 | 从 context 取 uid，做归属与数据级权限 | 不解析 token，不信请求头里的 uid |

⚠️ 网关注入身份头的前提是**业务服务只接受来自网关的流量**（NetworkPolicy 或安全组限定来源）。否则任何人直连 Pod IP，带一个 `X-User-Id: 1` 就能冒充管理员。

---

## 15.2 六大常见漏洞的工程防线

**结论先行**：六类里有五类的防线能收敛成三句话——不信任入参、身份从会话取、输出必编码；只有重放要额外加时间戳和随机数。

| 漏洞 | 攻击长什么样 | 防线 |
|---|---|---|
| SQL 注入 | 参数里塞 `' OR 1=1 --`，拼进 WHERE 后整表泄露 | 参数化查询（`?` 占位）、ORM 不拼字符串、DB 账号只给本库 DML、报错不回显 SQL |
| XSS | 评论里发 `<img src=x onerror=...>`，偷 Cookie 或伪造操作 | 输出编码（`html/template` 自动）、CSP `default-src 'self'`、Cookie 加 `HttpOnly`、富文本走白名单清洗 |
| CSRF | 第三方页面自动提交表单，浏览器自带 Cookie | `SameSite=Lax` 起步、变更类接口校验 `Origin` 白名单、CSRF Token 双提交 |
| SSRF | 传 `url=http://169.254.169.254/latest/meta-data/` 读云主机凭证 | 协议 + 域名白名单、解析后的 IP 禁内网段、不跟随跳转 |
| 越权 | 把 `/orders/1001` 改成 `/orders/1002` | 见 15.5，服务端归属校验 |
| 重放 | 抓一次支付回调，原样重发 100 次 | 时间戳 ±300 秒 + nonce `SET NX EX 300` + 对 body 做 HMAC 签名 |

SSRF 的内网段清单固定这几段，写成工具函数：`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`127.0.0.0/8`、`169.254.0.0/16`（云元数据）、`::1`、`fc00::/7`。**先解析域名再判 IP**，只判字符串会被 `http://127.0.0.1.nip.io` 绕过。

```go
package secure

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

// 整条 SQL 里没有一处拼接：用户输入只能作为参数交给驱动
const queryUserByID = `SELECT id, nickname, email FROM users WHERE id = ? AND deleted_at IS NULL`

type User struct {
	ID       int64
	Nickname string
	Email    string
}

// UserDetail 演示参数化查询 + 输出编码两道防线
func UserDetail(db *sql.DB) http.Handler {
	// html/template 按上下文自动转义：正文、属性、URL、JS 各用各的编码
	tmpl := template.Must(template.New("user").Parse(
		`<h1>{{.Nickname}}</h1><p>{{.Email}}</p>`))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		var u User
		err = db.QueryRowContext(r.Context(), queryUserByID, id).
			Scan(&u.ID, &u.Nickname, &u.Email)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			// 对外只给一句话，堆栈和 SQL 只进日志
			log.Printf("query user %d: %v", id, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, u); err != nil {
			log.Printf("render user %d: %v", id, err)
		}
	})
}
```

两个细节决定这段代码是否真安全：SQL 里 `id` 只以 `?` 出现，**没有一处 `fmt.Sprintf` 拼 SQL**；页面用 `html/template` 而不是 `text/template`，前者按 HTML 上下文转义，后者原样输出。前端用 JS 渲染时，把数据塞进页面用 `json.Marshal`，别用字符串拼 `<script>`。

---

## 15.3 密钥与凭证管理

**结论先行**：密钥只有两条铁律——不进代码库、能轮换。第一条守不住时，第二条是唯一的补救；第二条没有，第一条迟早也守不住。

| 存放位置 | 适用 | 风险 |
|---|---|---|
| 代码 / 配置文件 | 任何环境都不行 | 一次提交就永久留在 Git 历史里 |
| 环境变量 | 低敏感：日志级别、功能开关 | 进程可读，`ps`、崩溃转储、子进程都可能带出 |
| Vault / 云 KMS / Secret Manager | 数据库密码、第三方 Key、加密密钥 | 有一次接入成本，要配好权限边界 |

生产用哪个：已有云就用云厂商的 Secret Manager（AWS Secrets Manager、阿里云 KMS 凭据管家）；自建机房用 HashiCorp Vault。K8s 里用 External Secrets Operator 把 Vault/云 Secrets 同步成 Secret，别把明文写进 Git 里的 `secret.yaml`。

| 凭证 | 轮换周期 | 怎么换不中断 |
|---|---|---|
| 数据库密码 | 90 天 | 建双账号，改配置灰度，观察一轮后删旧账号 |
| 第三方 API Key | 180 天或按对方要求 | 新旧双 key 并行，全量切完再停旧 key |
| TLS 证书 | 90 天 | ACME 自动续期，提前 30 天告警 |
| JWT 签名密钥 | 90 天 | header 带 `kid`，新旧密钥并存一个 access 周期 |
| 员工个人凭证 | 离职当天 | 与离职流程绑定，先停权限后办手续 |

**Git 泄露后的处置顺序不能反**：吊销 → 轮换 → 清历史 → 排查。

1. **吊销**：立刻在服务商后台禁用这个 key。删除提交不等于删除记录，先止血。
2. **轮换**：签发新 key，更新到 Secret Manager 与所有引用方。
3. **清历史**：用 `git filter-repo` 或 BFG 重写历史、强推，并通知所有人重新克隆；私有仓库还要清 fork 与 PR 引用。
4. **排查**：查 key 在泄露窗口内的调用日志——陌生 IP、异常量、越权访问。这步最容易被跳过，也最该做。

### 可直接抄

```bash
# gitleaks v8.18+：扫全历史，--redact 让输出不打印明文；有泄露时退出码为 1
gitleaks detect --source . --redact -v
# 只扫暂存区，适合放进 pre-commit
gitleaks protect --staged --redact -v
# 没有二进制时用 go 直接跑（本机实测：命中 aws-access-token，exit 1）
go run github.com/zricethezav/gitleaks/v8@v8.18.4 detect --source . --redact -v

# git-secrets 等价做法
git secrets --install && git secrets --register-aws
git secrets --scan-history
```

CI 里把它放在构建之前，退出码非 0 直接 fail。**光靠人眼 review 挡不住密钥**，40 位随机串在 diff 里和哈希看起来没区别。

---

## 15.4 数据分级与脱敏

**结论先行**：分级不是为了合规文档，是让"这条数据谁能看"在代码里可以判定。四级就够：公开、内部、敏感、机密。

判定标准一句话：**泄露后会不会被直接拿去骗钱、骗贷、冒充身份**。会，就是机密；只会让公司难看，是内部；本来就对外，是公开。

| 字段 | 级别 | 存储 | 展示 | 审批 / 审计 |
|---|---|---|---|---|
| 商品标题、门店地址 | 公开 | 明文，TLS 传输 | 原值 | 无 |
| 订单金额、库存 | 内部 | 明文，TLS 传输 | 原值 | 登录即可，抽样审计 |
| 手机号、收货地址 | 敏感 | 字段级加密（AES-256，密钥进 KMS） | `138****8000` | 客服凭工单查，单条留痕 |
| 身份证号、银行卡号 | 机密 | 字段级加密 + 独立密钥 | `110101********1234` | 仅实名/支付服务，人工二级审批 |
| 登录密码 | 机密 | bcrypt / argon2id 单向哈希 | 永不展示 | 任何角色都不可读 |

传输统一 TLS 1.2+，敏感以上字段存储用 AES-256，密钥不跟密文放一起，交给 KMS 托管。密码是**哈希不是加密**——加密能解回来，哈希不能，这条在评审里区分得很清楚。

脱敏要落在**统一出口**，不是每个 handler 各写一遍：

```go
package secure

// 手机号：13800138000 -> 138****8000
func MaskPhone(s string) string {
	if len(s) != 11 {
		return s
	}
	return s[:3] + "****" + s[7:]
}

// 身份证：110101199001011234 -> 110101********1234
func MaskIDCard(s string) string {
	if len(s) != 18 {
		return s
	}
	return s[:6] + "********" + s[14:]
}

// 银行卡：6222021234567890 -> 6222 **** **** 7890
func MaskBankCard(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:4] + " **** **** " + s[len(s)-4:]
}
```

落点只有三个：接口序列化（DTO 层统一处理）、日志（脱敏后的 logger）、导出与对账文件。日志里出现完整身份证，多数不是有人故意打的，是 `log.Printf("%+v", user)` 把整个结构体打了出来。

---

## 15.5 越权：最容易漏的一类

**结论先行**：越权不是"没登录"，是"登录了，但不该看这条、不该调这个接口"。前者靠鉴权中间件，后者只能靠业务代码里的归属校验。

| 类型 | 典型手法 | 防线 |
|---|---|---|
| 水平越权 | 改 URL/body 里的 `id`、`user_id`、`order_no` 查别人数据 | 服务端从会话取 uid，资源归属校验，入参里的 uid 一律丢弃 |
| 垂直越权 | 普通用户直接调 `/admin/users/delete`，或改 `role=admin` 提权 | 接口级权限注解 + 网关路由分流，管理端独立域名与网络 |

**四条防线，缺一条就漏**：

1. 身份只从会话取。`user_id` 永远来自 token / Session，**不接受请求参数**。代码里出现 `r.URL.Query().Get("user_id")` 当身份用，就是漏洞。
2. 每条按 ID 取资源的语句都过归属校验。别指望"前端不显示按钮"。
3. 统一鉴权注解或中间件，而不是每个 handler 手写 if。手写必然漏，尤其新加的接口。
4. 管理端独立域名 + 独立网络入口（VPN / IP 白名单），和用户端不同一套鉴权。把管理接口混在用户域名下，是垂直越权最常见的入口。

```go
package secure

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
)

type ctxKey string

const userIDKey ctxKey = "uid"

// 身份只由鉴权中间件写入 context，handler 不许从 query / body 里取 user_id
func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, userIDKey, uid)
}

func UserID(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(userIDKey).(int64)
	return uid, ok
}

// OwnerOf 由业务实现：这条资源归谁；返回 sql.ErrNoRows 表示资源不存在
type OwnerOf func(ctx context.Context, id int64) (int64, error)

// RequireOwner 校验 URL 里的 {id} 是否归当前登录用户，是所有“改 ID 查别人数据”的通用闸门
func RequireOwner(ownerOf OwnerOf) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := UserID(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "bad id", http.StatusBadRequest)
				return
			}
			owner, err := ownerOf(r.Context(), id)
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if owner != uid {
				// 资源存在但不属于你：对可枚举 ID 的接口统一回 404，别回 403
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

⚠️ 返回 403 还是 404 是产品决策：403 老实，但等于告诉攻击者"这个 ID 存在"。订单、账单这类可枚举 ID 的接口，统一回 404。

---

## 15.6 上线安全清单

**结论先行**：清单的用法是打勾，不是阅读。每项都要有命令或判据，勾不上就说明没验，不是"应该没问题"。

```bash
# 1. 依赖漏洞：只报真正被调用到的（Go 1.22+）
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
# 2. 镜像漏洞：HIGH/CRITICAL 直接失败
trivy image --severity HIGH,CRITICAL --exit-code 1 registry.example.com/app:1.2.3
# 3. TLS 协议与套件
openssl s_client -connect api.example.com:443 -servername api.example.com </dev/null 2>/dev/null | grep -E "Protocol|Cipher"
# 4. CORS 是否放开任意来源：期望无输出
curl -sI -H "Origin: https://evil.example" https://api.example.com/health | grep -i access-control-allow-origin
# 5. 调试端口是否对外：期望 connection refused
nc -zv -w 2 api.example.com 6060
```

实测（本机，govulncheck v1.9.0）：干净仓库输出 `No vulnerabilities found.` 和 `Your code is affected by 0 vulnerabilities.`；有漏洞时它会把调用链一路打到你的行号。

| 检查项 | 判据 | 不过怎么办 |
|---|---|---|
| 依赖漏洞 | `govulncheck ./...` 无 affected | 升级或换库，别只加 `//nolint` |
| 镜像漏洞 | trivy 无 HIGH/CRITICAL | 换 distroless/alpine 基础镜像后重扫 |
| TLS | 只有 TLS 1.2/1.3，无 RC4/3DES | 改网关或 ingress 的 `ssl_protocols` |
| CORS | 只回显白名单内的 Origin | 删 `*`；带凭证时浏览器本来就不许 `*` |
| 限流 | 单 IP 100 次/分钟返回 429 | 网关上按 IP + 用户双维度限 |
| 日志脱敏 | 抽查 100 条日志，无完整手机号/身份证 | 改 logger，统一脱敏字段 |
| 默认口令 | 无 `admin/admin`、无空密码 | 首登强制改密，初始化脚本随机生成 |
| 调试端口 | 6060/9090/6379 外网不可达 | 绑 `127.0.0.1` 或加 NetworkPolicy |
| 错误页 | 500 页面无堆栈、无 SQL、无路径 | 统一错误处理，堆栈只进日志 |
| 备份加密 | 备份文件已加密且能恢复 | 加密后再上传，季度做一次恢复演练 |
| 密钥来源 | 全部来自 Secret Manager 或环境变量 | 清理代码与镜像里的明文 |
| 管理入口 | 独立域名 + 白名单 | 拆域名，加 VPN 或 IP 白名单 |
| 审计日志 | 敏感数据访问有 uid、时间、对象 | 统一在出口埋点，保留 180 天 |
| 回滚预案 | 安全配置有开关，5 分钟内能退回 | 配置化，别硬编码进镜像 |

### 坑

| 坑 | 后果 | 正确做法 |
|---|---|---|
| JWT 存 localStorage | 一个 XSS 就把 token 带走，且无法加 HttpOnly | 存 `HttpOnly; Secure; SameSite=Lax` 的 Cookie |
| CORS 写 `*` | 任意站点能读你的响应；带 Cookie 时浏览器直接拒绝，白忙 | 白名单精确到域名，回 `Vary: Origin` |
| 内网服务裸奔 | 以为"外网访问不到"，一台被钓鱼的开发机就能横向 | 服务间也鉴权，NetworkPolicy + mTLS |
| 日志打完整身份证 | 日志系统权限比数据库松，等于给全公司开了查询权限 | 结构化日志 + 字段级脱敏，禁 `%+v` 打结构体 |
| 测试环境用生产数据 | 测试库权限松、备份不加密，一次拖库就是真泄露 | 脱敏后导入或造数，生产数据不出生产账号 |
| 用"ID 猜不到"当权限 | UUID 一泄露（分享链接、截图）就等于授权 | 归属校验照做，UUID 只当不可枚举，不当鉴权 |

🔑 安全评审真正会问的就三句：身份从哪来、这条数据归谁、出事怎么退。答不上这三句，其他都是装饰。

---

## 本章自检

**Q1** 网关已经统一鉴权，`GET /orders/{id}` 仍能拿到别人的订单，最可能漏了哪一步？
▸ 答案：缺资源归属校验——网关只证明"登录了"，没证明"这单是你的"。在服务层比对 `order.user_id == 会话 uid`，或 SQL 里直接带 `AND user_id = ?`。

**Q2** 老板要求"封号后 10 秒内所有端下线"，纯 JWT 为什么做不到？给出两套改法。
▸ 答案：JWT 无状态，签发后只能等过期。改法一：access 15 分钟 + refresh 存服务端，封号删 refresh；改法二：token 带版本号，验签后查一次 Redis 比对。要 10 秒级只能选二。

**Q3** 生产日志里出现完整身份证号，除了改代码，还有哪两处要一起查？
▸ 答案：日志采集与存储链路（ES 里的存量数据要清），以及导出/对账文件和 `%+v` 打结构体的位置。只改一处，存量还在。
