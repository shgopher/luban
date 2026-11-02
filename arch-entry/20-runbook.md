# 第 20 章 故障处置手册：凌晨三点的操作清单

> **本章一句话**：故障时最贵的不是不会修，是「不知道先看什么」；把处置步骤写成清单，半夜也能照着做。

**读完你会：**
- 用统一流程判断「先止血还是先定位」
- 对六类高频故障有可执行的动作
- 知道每条命令的预期输出
- 在压力下做出「回滚还是继续修」的决定

---

## 20.1 通用排障流程

**结论先行**：先确认影响面，再判断要不要止血，最后才定位根因；顺序反了，会在定位中扩大故障。

| 步骤 | 动作 | 时限 |
|---|---|---|
| 1 确认影响 | 哪些接口/用户/地域、错误率、是否资损 | 2 分钟 |
| 2 判断止血 | 最近有变更？能回滚？能切流？ | 3 分钟 |
| 3 执行止血 | 回滚/降级/扩容/切流，保留现场 | 5 分钟 |
| 4 定位根因 | 按依赖→资源→代码逐层排查 | 30 分钟 |
| 5 恢复与复盘 | 确认指标恢复，记录时间线 | 事后 |

三条纪律：**能回滚先回滚**；**一次只改一个变量**；**所有操作记录时间与命令**。

---

## 20.2 接口变慢 / 超时

**结论先行**：先分清「全部慢」还是「部分慢」，再看是自己慢、下游慢，还是资源打满。

```bash
# 看耗时分布：DNS、连接、首字节、总耗时
curl -o /dev/null -s -w 'dns=%{time_namelookup} conn=%{time_connect} ttfb=%{time_starttransfer} total=%{time_total}\n' \
  https://api.example.com/orders/1

# 看 Pod 资源与重启
kubectl -n prod top pod -l app=order-api
kubectl -n prod get pods -l app=order-api

# 抓 30 秒 CPU 火焰图
go tool pprof -seconds=30 http://127.0.0.1:6060/debug/pprof/profile
(pprof) top -cum 20
```

| 现象 | 可能原因 | 处置 |
|---|---|---|
| 全部接口慢 | 资源打满、DB 慢、网络 | 扩容、限流、切流 |
| 部分接口慢 | 慢 SQL、N+1、大对象 | 看 trace、加索引 |
| 间歇性慢 | GC、锁、连接池等待 | 看 block/mutex profile |
| 只有某地域慢 | 机房/运营商/DNS | 切流量、检查 SLB |

---

## 20.3 数据库 CPU 打满

**结论先行**：先找「谁在跑」，再决定「杀、限、加索引」；不要一上来重启数据库。

```sql
-- 谁在跑、跑了多久、在等什么
SHOW FULL PROCESSLIST;

-- 看锁与事务
SELECT * FROM information_schema.innodb_trx
ORDER BY trx_started LIMIT 10;

SHOW ENGINE INNODB STATUS\G

-- 慢查询（需开启 slow_query_log）
SELECT * FROM mysql.slow_log ORDER BY start_time DESC LIMIT 20;
```

```bash
# 确认是 CPU 还是 IO
iostat -x 1 5
```

| 原因 | 特征 | 处置 |
|---|---|---|
| 慢查询 | 单条 SQL 扫描行数巨大 | kill 掉，补索引，改 SQL |
| 锁等待 | 大量 `Waiting for lock` | 找阻塞源，kill 长事务 |
| 连接数打满 | `Too many connections` | 限流、调连接池、加只读 |
| 备份/DDL | 定时任务时段 | 错峰、用 gh-ost |

```sql
-- 杀之前先确认：是不是核心事务、回滚要多久
KILL 123456;
```

⚠️ 大事务 kill 会触发回滚，可能比原事务更久；先看 `trx_rows_modified` 再动手。

---

## 20.4 缓存雪崩 / 热 key

**结论先行**：缓存问题的第一动作是「保护数据库」，不是「恢复缓存」；先限流/降级，再修缓存。

```bash
redis-cli -h 10.0.0.12 INFO stats | egrep 'keyspace_hits|keyspace_misses|expired_keys'
redis-cli -h 10.0.0.12 INFO memory | egrep 'used_memory_human|maxmemory_human|evicted_keys'
redis-cli -h 10.0.0.12 --hotkeys
redis-cli -h 10.0.0.12 LATENCY DOCTOR
```

| 现象 | 判断 | 处置 |
|---|---|---|
| 命中率骤降 | 大量 key 同时过期/实例重启 | 限流 DB、分批预热、TTL 加抖动 |
| 单 key QPS 极高 | 热 key | 本地缓存、key 打散、读副本 |
| 内存满、大量 evict | 容量不足/大 key | 扩容、清理大 key、调淘汰策略 |
| Redis 超时 | 单线程阻塞/网络 | 查 `SLOWLOG`、大命令、连接数 |

---

## 20.5 MQ 积压

**结论先行**：先确认「消费者还活着吗」，再谈扩容；消费者死了，加多少机器都没用。

```bash
kafka-consumer-groups.sh --bootstrap-server broker:9092 \
  --describe --group order-consumer
```

| 输出 | 含义 | 动作 |
|---|---|---|
| `CONSUMER-ID` 为空 | 没有消费者 | 查 Pod/进程、看日志 |
| `LAG` 持续上涨 | 消费速度 < 生产速度 | 扩消费者（≤ 分区数） |
| 频繁 rebalance | 消费者不稳定 | 调 `max.poll.interval`、减少批量 |
| 单条消费耗时高 | 下游慢/逻辑重 | 优化逻辑、异步化 |

处置顺序：确认消费者存活 → 扩到分区数上限 → 降级非核心逻辑 → 临时转存/跳过（需审批）→ 事后补数据。

---

## 20.6 发布后错误率上升

**结论先行**：5 分钟内定位不了就回滚；回滚后再慢慢查，别在线上做实验。

```bash
kubectl -n prod rollout undo deploy/order-api
kubectl -n prod rollout status deploy/order-api --timeout=120s
kubectl -n prod logs deploy/order-api --since=10m | grep '"level":"ERROR"' | head -20
```

| 检查项 | 命令/位置 |
|---|---|
| 新旧版本是否共存 | `kubectl get pods -o wide`，看镜像 tag |
| 错误集中在哪 | 日志按 `path`、`err` 聚合 |
| 是否与 DB 变更同发 | 查发布单与 migration 记录 |
| 是否只影响部分用户 | 按地域/版本/用户分群看 |

回滚后保留现场：Pod 日志、trace、慢查询、变更 diff，复盘时用。

---

## 20.7 依赖方故障

**结论先行**：依赖方挂了，你能做的是「隔离 + 降级 + 限流」，不是「等它恢复」。

```bash
# 确认是对方问题还是自己网络
curl -s -o /dev/null -w '%{http_code} %{time_total}\n' https://pay.example.com/health
dig pay.example.com +short
telnet pay.example.com 443
```

| 情况 | 处置 |
|---|---|
| 对方 5xx/超时 | 熔断打开、走降级、限流保护自己 |
| 对方限流 429 | 降速、排队、错峰重试 |
| 网络不通 | 切备用线路/备用域名 |
| 对方数据错误 | 停止写入、对账、人工介入 |

---

## 20.8 磁盘满 / 内存 OOM

**结论先行**：磁盘满先找「谁在写」，OOM 先看「是泄漏还是峰值」；两者都不要靠重启掩盖。

```bash
df -h
du -sh /var/log/* | sort -h | tail -10
# 已删除但仍被进程占用的文件
lsof +L1 | head

# K8s：看退出码与 OOMKilled
kubectl -n prod describe pod order-api-xxx | grep -A3 'Last State'
kubectl -n prod get pod order-api-xxx -o jsonpath='{.status.containerStatuses[0].lastState}'
```

| 现象 | 原因 | 处置 |
|---|---|---|
| 磁盘满 | 日志未轮转/大文件 | 清理、配 logrotate、调保留期 |
| 内存持续涨 | 泄漏/缓存无上限 | 抓 heap、限制缓存、重启止血 |
| 周期性 OOM | 峰值超 limit | 调 limit、限流、优化大对象 |
| 退出码 137 | 被 SIGKILL（OOM） | 看 limit 与 `GOMEMLIMIT` |

---

## 本章自检

**Q1** 数据库 CPU 100%，你第一件事是重启数据库吗？
▸ 答案：不是。先 `SHOW PROCESSLIST` 找慢查询/阻塞源，能 kill 单条就 kill；重启会中断所有连接、触发恢复，通常让故障更大。

**Q2** 缓存命中率从 95% 掉到 40%，但 Redis 没挂，可能是什么？
▸ 答案：大量 key 同时过期（TTL 没抖动）、实例重启后缓存为空、或 key 规则变更导致查不到；先看 `expired_keys` 与 key 前缀分布，再决定预热或回滚。

**Q3** 发布后错误率上升，但日志里没有明显异常，怎么办？
▸ 答案：先回滚保住业务，再对比新旧版本的配置、依赖版本、DB 变更和流量分布；没有异常日志往往说明是数据兼容或下游行为变化，不是代码抛错。
