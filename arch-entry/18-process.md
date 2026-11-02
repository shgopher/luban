# 第 18 章 研发流程：让架构落得下去

> **本章一句话**：架构不是画在文档里的，是长在流程里的；流程卡不住的东西，架构上迟早会烂掉。

**读完你会：**
- 选一套适合团队规模的分支与发布节奏
- 用 Code Review 和 CI 把规范变成门禁
- 把测试从「有空再写」变成「流水线必须过」
- 组织一次有改进项、能闭环的复盘

---

## 18.1 分支策略

**结论先行**：分支活得越短，合并越不痛；主干开发 + 短分支 + 发布分支，能覆盖 90% 的团队。

| 团队规模 | 策略 | 发布节奏 |
|---|---|---|
| 5 人以内 | 主干开发，功能分支 < 2 天，合并即发 | 每天多次 |
| 30 人左右 | 主干开发 + release 分支 + hotfix | 每周 1–2 次 |
| 多团队/多版本 | 主干 + release + 特性开关隔离 | 按需 |

```bash
git switch -c feat/order-timeout main
# ... 开发、提交 ...
git fetch origin && git rebase origin/main
git push -u origin feat/order-timeout
# 提 MR，评审通过后 squash merge 到 main
git switch main && git pull --ff-only
```

| 规则 | 原因 |
|---|---|
| 分支存活 < 2 天 | 越久冲突越多，评审越难 |
| 一个分支只做一件事 | 回滚时能整块回退 |
| hotfix 从 release 拉，修完回合 main | 避免修复丢失 |
| 禁止直接 push main | 绕过评审和 CI |

---

## 18.2 Code Review：看什么、不看什么

**结论先行**：Review 看「会不会出事」，不看「你喜不喜欢」；格式和命名交给 linter。

| 看 | 不看 |
|---|---|
| 正确性与边界（空值、越界、并发） | 缩进、空格、命名风格 |
| 错误处理与超时 | 个人偏好（用 map 还是 switch） |
| SQL 与索引、N+1 | 注释格式 |
| 日志与敏感信息 | 无关文件的重排 |
| 测试是否覆盖失败路径 | 提交信息措辞 |
| 安全（鉴权、注入、越权） | |

| 指标 | 建议 |
|---|---|
| PR 大小 | < 400 行，超过就拆 |
| 首次响应 | 4 小时内 |
| 评审轮次 | ≤ 2 轮，超过说明需求没对齐 |
| 必须项 | 至少 1 人 approve，核心模块 2 人 |

---

## 18.3 CI 流水线最小集

**结论先行**：CI 只做四件事——能编译、能过测、能构建、能扫描；这四件不过，不允许合并。

```yaml
name: ci
on:
  pull_request:
  push: { branches: [main] }

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod, cache: true }
      - run: go mod download
      - run: go vet ./...
      - run: go test ./... -race -count=1 -coverprofile=cover.out
      - run: go tool cover -func=cover.out | tail -1
      - uses: golangci/golangci-lint-action@v6
        with: { version: v1.61 }
      - run: CGO_ENABLED=0 go build -trimpath -o bin/app ./cmd/api

  image:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    permissions: { contents: read, packages: write }
    steps:
      - uses: actions/checkout@v4
      - uses: docker/build-push-action@v6
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

| 阶段 | 必须 | 耗时目标 |
|---|---|---|
| lint + vet | 是 | < 1 分钟 |
| 单测 | 是 | < 5 分钟 |
| 构建镜像 | 是 | < 5 分钟 |
| 依赖/镜像扫描 | 是（高危阻断） | < 3 分钟 |
| 部署 staging | main 分支 | < 3 分钟 |
| 集成测试 | main 分支 | < 10 分钟 |
| 生产发布 | 人工审批 | — |

---

## 18.4 测试策略：金字塔怎么落地

**结论先行**：单元测试保正确性、集成测试保「连起来能用」、E2E 只保核心链路；比例大约 70/20/10。

| 层 | 测什么 | 数量 | 运行时机 |
|---|---|---|---|
| 单元 | 分支、边界、错误路径 | 多 | 每次提交 |
| 集成 | 真 DB、真 MQ、真 Redis | 中 | 每次合并 |
| E2E | 登录→下单→支付 | 少 | 发布前 |

必须有集成测试的三类：**SQL 与事务**、**消息消费与幂等**、**第三方适配（签名/序列化）**。

```go
func TestCreateOrder(t *testing.T) {
	h := NewHandler(newFakeService())
	body := `{"sku_id":1,"qty":2}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateOrder(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got Order
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Qty != 2 {
		t.Fatalf("qty=%d, want 2", got.Qty)
	}
}
```

集成测试用 Testcontainers 或 docker compose 起真依赖，别用内存数据库替代 MySQL——SQL 方言和事务行为对不上，测了等于没测。

---

## 18.5 制品与版本

**结论先行**：制品不可变、版本可追溯、构建可复现；同一个 commit 构建两次，结果必须一致。

| 项 | 规则 |
|---|---|
| 镜像 tag | `git sha` 前 7–12 位，禁止 `latest` |
| 版本号 | 语义化：`MAJOR.MINOR.PATCH` |
| 仓库 | 私有 registry，保留最近 30 个版本 |
| 构建 | `-trimpath`、锁定 `go.sum`、固定基础镜像 digest |
| 变更记录 | 每个版本写清「改了什么、怎么回滚」 |

```bash
docker build --build-arg VERSION=$(git describe --tags --always) \
  -t registry.example.com/order-api:$(git rev-parse --short HEAD) .
docker push registry.example.com/order-api:$(git rev-parse --short HEAD)
```

---

## 18.6 值班与事故响应

**结论先行**：值班不是「接电话」，是「有人对故障负责」；没有升级路径的值班，等于把事故压在一个人身上。

| 角色 | 职责 | 谁担任 |
|---|---|---|
| 指挥 | 判断影响面、决定止血方案 | 值班负责人 / TL |
| 执行 | 执行回滚、切流、扩容 | 当班工程师 |
| 沟通 | 对内同步、对外公告 | 产品 / 项目经理 |

| 级别 | 判定 | 响应时限 | 升级 |
|---|---|---|---|
| P0 | 核心链路不可用、资损 | 5 分钟 | 立即叫 TL + 总监 |
| P1 | 部分用户受影响、错误率 > 5% | 15 分钟 | 30 分钟未恢复升级 |
| P2 | 非核心功能异常 | 1 小时 | 次日处理 |

升级规则：**15 分钟没有明确结论就升级**，宁可多叫一个人，不要让故障在一个人手里转。

---

## 18.7 复盘：从事故到改进项

**结论先行**：复盘只回答三件事——发生了什么、为什么会发生、怎么保证不再发生；不追责，但要追改进项。

```markdown
# 事故复盘：2026-11-02 订单接口 5xx 升高

## 时间线（精确到分钟）
- 10:02 发布 order-api v1.8.3
- 10:05 错误率从 0.1% 升到 12%
- 10:07 值班确认，回滚
- 10:09 错误率恢复

## 根因
新版本把库存查询改成了批量接口，但缓存 key 未加版本号，
旧缓存结构与新代码不兼容，反序列化失败。

## 改进项
| 改进 | 负责人 | 截止 | 状态 |
|---|---|---|---|
| 缓存 key 加 schema 版本 | 张三 | 11-05 | 进行中 |
| 发布前增加缓存兼容检查 | 李四 | 11-10 | 待办 |
| 金丝雀观察窗口从 10 分钟调到 30 分钟 | 王五 | 11-03 | 完成 |
```

复盘的三条纪律：对事不对人；改进项必须有负责人和日期；下一次复盘先检查上次改进项是否完成。

---

## 本章自检

**Q1** 为什么分支要活得短？
▸ 答案：分支存活越久，与主干的差异越大，合并冲突和评审成本呈指数上升；短分支让每次变更小、可评审、可回滚。

**Q2** 单元测试覆盖率 90%，为什么线上还是出问题？
▸ 答案：覆盖率只说明代码被执行过，不说明断言有效。常见缺口是集成层（SQL、序列化、消息）和失败路径没测；补集成测试比追覆盖率更有用。

**Q3** 复盘写了 20 条改进项，三个月后一条没做，问题出在哪？
▸ 答案：改进项没有负责人和截止日期，也没进迭代排期。复盘输出必须收敛到 3–5 条、明确 owner、纳入下周计划，并在下次复盘验收。
