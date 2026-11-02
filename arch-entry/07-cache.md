# 第 7 章 缓存：加一层，少一半请求

> **本章一句话**：缓存的收益来自命中率，代价来自一致性；先算清「省了多少数据库查询」，再决定加不加、加在哪一层。

**读完你会：**
- 判断一个接口该不该加缓存
- 用 Cache Aside 写出正确的读写路径
- 处理穿透、击穿、雪崩和一致性四类问题
- 用命令查出大 key、热 key 并处理

---

## 7.1 什么该缓存，什么不该

**结论先行**：读多写少、能容忍短暂不一致、单次查询成本高的数据才值得缓存；强一致和写多读少的数据，缓存只会制造 bug。

| 数据类型 | 该不该缓存 | 理由 |
|---|---|---|
| 商品详情、配置、字典 | 该 | 读远多于写，变更少 |
| 用户会话、登录态 | 该 | 读非常频繁，天然可过期 |
| 库存、余额 | 谨慎 | 必须有过期与版本控制，不能只靠缓存 |
| 订单状态 | 谨慎 | 状态机严格，缓存要随写失效 |
| 报表、聚合结果 | 该 | 计算贵，允许分钟级延迟 |
| 强一致账务流水 | 不该 | 缓存不一致的代价是资损 |

算一笔账再决定：

```text
缓存收益 ≈ 命中率 × 单次查询成本 × 请求量
缓存成本 ≈ 内存 + 失效逻辑 + 不一致排查 + 运维
```

命中率低于 50% 的缓存，通常不值得维护。

---

## 7.2 Cache Aside：90% 的场景用这一种

**结论先行**：读时先查缓存、没有再查库并回填；写时先更新数据库、再删除缓存。**是删缓存，不是更新缓存。**

```go
func (s *Service) GetProduct(ctx context.Context, id int64) (*Product, error) {
	key := fmt.Sprintf("product:%d", id)

	// 1. 读缓存
	if raw, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
		var p Product
		if json.Unmarshal(raw, &p) == nil {
			return &p, nil
		}
	}

	// 2. 回源数据库
	p, err := s.repo.FindProduct(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 空值也缓存，防穿透；TTL 要短
			_ = s.rdb.Set(ctx, key, []byte("null"), 60*time.Second).Err()
			return nil, ErrNotFound
		}
		return nil, err
	}

	// 3. 回填，TTL 加随机抖动，防同一时刻集体过期
	ttl := 30*time.Minute + time.Duration(rand.Intn(180))*time.Second
	raw, _ := json.Marshal(p)
	_ = s.rdb.Set(ctx, key, raw, ttl).Err()
	return p, nil
}

func (s *Service) UpdateProduct(ctx context.Context, p *Product) error {
	if err := s.repo.UpdateProduct(ctx, p); err != nil {
		return err
	}
	// 先改库，再删缓存；删除失败可以重试，不能忽略
	return s.rdb.Del(ctx, fmt.Sprintf("product:%d", p.ID)).Err()
}
```

为什么是删除而不是更新：

| 做法 | 问题 |
|---|---|
| 更新缓存 | 两个并发写，后到的可能覆盖先到的，缓存留下旧值 |
| 删除缓存 | 下一次读自然回源，简单且不容易错 |

为什么先改库再删缓存：反过来的话，删完缓存、改库之前，有读请求把旧值回填，缓存就长期是旧的。

---

## 7.3 穿透、击穿、雪崩：三张药方

**结论先行**：穿透是「查不存在的数据」，击穿是「热点 key 过期瞬间」，雪崩是「大量 key 同时过期」；三者药方完全不同。

| 问题 | 现象 | 药方 |
|---|---|---|
| 穿透 | 大量请求查不存在的 ID，全部打到库 | 空值缓存（60s）、布隆过滤器、参数校验 |
| 击穿 | 单个热点 key 过期，瞬间几千请求回源 | 互斥锁 / singleflight，只放一个请求回源 |
| 雪崩 | 大批 key 同一时刻过期，或缓存整体挂掉 | TTL 随机抖动、多级缓存、熔断降级、集群高可用 |

击穿用 singleflight 最省事：

```go
var g singleflight.Group

func (s *Service) GetHot(ctx context.Context, id int64) (*Product, error) {
	key := fmt.Sprintf("product:%d", id)
	v, err, _ := g.Do(key, func() (any, error) {
		// 只有一个 goroutine 真正回源，其余等待它的结果
		return s.loadFromDB(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return v.(*Product), nil
}
```

布隆过滤器用于「确定不存在」的拦截：把全量合法 ID 放进布隆，查之前先问一句，说不存在就直接返回。注意它只能判「不存在」，不能判「存在」，且有误判率（1% 左右可接受）。

---

## 7.4 缓存一致性：延迟双删与订阅

**结论先行**：缓存和数据库没有免费的一致性；能接受秒级延迟就用「删缓存 + 延迟补偿」，要求高就订阅 binlog 异步失效。

| 方案 | 一致性 | 成本 | 适用 |
|---|---|---|---|
| 先改库再删缓存 | 秒级 | 低 | 绝大多数业务 |
| 延迟双删 | 秒级，抗并发回填 | 低 | 写后立刻读的场景 |
| 订阅 binlog 删缓存 | 准实时 | 中 | 多写入源、跨服务 |
| 写穿 + 分布式锁 | 强 | 高 | 库存、余额等 | 

延迟双删的两次删除：

```go
func (s *Service) UpdateWithDoubleDelete(ctx context.Context, p *Product) error {
	key := fmt.Sprintf("product:%d", p.ID)
	_ = s.rdb.Del(ctx, key)          // 第一次：先清掉旧值
	if err := s.repo.UpdateProduct(ctx, p); err != nil {
		return err
	}
	// 第二次：等读请求可能回填的窗口过去再删
	time.AfterFunc(500*time.Millisecond, func() {
		_ = s.rdb.Del(context.Background(), key).Err()
	})
	return nil
}
```

延迟取「主从延迟 + 一次读耗时」，经验值 300–500ms；不要用固定 1 秒，主从延迟可能到几秒。

订阅 binlog 的做法：Canal / Debezium 解析 MySQL binlog，把变更投递到 MQ，消费者按表名和主键删缓存。好处是不依赖业务代码，坏处是多了一套组件要维护。

⚠️ 缓存一致性出问题时，先确认「写路径是不是只在业务代码里删缓存」——定时任务、后台脚本、DBA 手工改库都会绕过它。

---

## 7.5 key 规范与 TTL 策略

**结论先行**：key 用冒号分层、带业务前缀、长度控制在 128 字节内；TTL 必须设，且加随机抖动。

```text
业务:对象:ID[:字段]
product:detail:10086
user:session:u_123
order:status:202611020001
```

| 规则 | 做法 | 原因 |
|---|---|---|
| 前缀 | 服务名或业务域 | 便于按前缀统计、清理 |
| 分隔 | 冒号 | Redis 生态工具默认识别 |
| 长度 | ≤ 128 字节 | 长 key 浪费内存、慢查询日志难读 |
| 字符 | 英文、数字、下划线 | 避免转义与乱码 |
| TTL | 必设，基础值 ± 10% | 防集中过期、防僵尸数据 |

TTL 参考值：

| 数据 | TTL |
|---|---|
| 商品详情 | 30 分钟 ± 3 分钟 |
| 用户会话 | 30 分钟（滑动续期） |
| 配置/字典 | 5 分钟 |
| 空值缓存 | 60 秒 |
| 验证码 | 5 分钟 |

> 💡 热点数据可以「逻辑过期」：value 里带过期时间，物理上不设 TTL，读到过期值时异步刷新。代价是刷新窗口内返回旧值。

---

## 7.6 大 key、热 key：怎么查怎么治

**结论先行**：大 key 拖慢网络与删除，热 key 打爆单分片；先查出来，再决定拆、压、迁。

判定标准：

| 类型 | 阈值 |
|---|---|
| String 大 key | value > 10KB |
| 集合大 key | 元素数 > 5000，或总大小 > 1MB |
| 热 key | 单 key QPS 占实例 10% 以上 |

```bash
# 扫大 key（生产低峰执行，--bigkeys 会遍历，别在高峰期跑）
redis-cli -h 10.0.0.12 -p 6379 --bigkeys

# 看单个 key 的实际内存占用
redis-cli -h 10.0.0.12 MEMORY USAGE product:detail:10086

# 热 key：需要 maxmemory-policy 为 lfu 系列
redis-cli -h 10.0.0.12 --hotkeys

# 慢查询与延迟
redis-cli -h 10.0.0.12 SLOWLOG GET 10
redis-cli -h 10.0.0.12 LATENCY DOCTOR
```

治理手段：

| 问题 | 处理 |
|---|---|
| 大 Hash/List | 按业务拆成多个 key，或分页存储 |
| 大 String | 压缩（gzip）后存，或拆字段 |
| 热 key | 本地缓存（如 bigcache）+ 多副本 key 打散 |
| 删除大 key | 用 `UNLINK` 异步删除，别用 `DEL` 阻塞 |
| 集群倾斜 | 检查 slot 分布，必要时重建分片 |

⚠️ `KEYS *` 会阻塞整个实例，生产禁用；用 `SCAN` 游标遍历。

---

## 7.7 Redis 生产配置清单

**结论先行**：内存上限、淘汰策略、持久化、危险命令、内网绑定，这五项不配，迟早出事。

```conf
# /etc/redis/redis.conf（Redis 7.x）
bind 10.0.0.0/8 127.0.0.1
protected-mode yes
port 6379

maxmemory 8gb
maxmemory-policy allkeys-lru     # 纯缓存；有持久数据用 volatile-lru 并给 key 设 TTL
maxmemory-samples 5

# 持久化：允许丢少量数据用 RDB；不允许丢用 AOF everysec
appendonly yes
appendfsync everysec
save 900 1
save 300 10

timeout 0
tcp-keepalive 300
slowlog-log-slower-than 10000    # 10ms
slowlog-max-len 256

# 用 ACL 收敛权限，而不是只靠密码
aclfile /etc/redis/users.acl
```

```bash
# 创建只允许访问业务前缀、禁用危险命令的账号
redis-cli -a "$REDIS_PASS" ACL SETUSER app on '>AppPass123' '~product:*' +@read +@write -@dangerous
redis-cli -a "$REDIS_PASS" ACL SAVE
# 验证：应当被拒绝
redis-cli -u redis://app:AppPass123@10.0.0.12:6379 --no-auth-warning KEYS '*'
```

预期的拒绝输出：

```text
(error) NOPERM this user has no permissions to run the 'keys' command
```

| 坑 | 后果 | 正确做法 |
|---|---|---|
| maxmemory 不设 | 内存打满，实例被 OOM Killer 杀 | 设上限 + 淘汰策略 |
| `allkeys-lru` 用在持久数据上 | 冷数据被淘汰，业务报错 | 持久数据用 `volatile-*` 并设 TTL |
| 单实例存百 GB | 主从同步慢、故障恢复小时级 | 分片集群，单实例 ≤ 32GB |
| 只设密码不设 ACL | 一个账号能 `FLUSHALL` | ACL 按前缀和命令授权 |
| 没监控内存碎片率 | 实际内存是数据的 2 倍 | 关注 `mem_fragmentation_ratio`，>1.5 考虑重启或迁移 |

---

## 本章自检

**Q1** 更新数据时，为什么推荐「删除缓存」而不是「更新缓存」？
▸ 答案：并发写时更新缓存会乱序覆盖，留下比数据库更旧的值；删除后由下一次读回源，天然拿到最新值，逻辑也简单。

**Q2** 缓存了空值，为什么还要设 60 秒这么短的 TTL？
▸ 答案：空值缓存是为了挡住「查不存在 ID」的穿透；如果数据后来被创建，长 TTL 会让它一直显示不存在。短 TTL 是「挡住攻击」和「尽快可见」的折中。

**Q3** Redis 内存没满，但业务间歇性超时，先查什么？
▸ 答案：先看慢查询和大 key（`SLOWLOG GET`、`--bigkeys`），再看是否执行了 `KEYS`/大集合全量操作，最后看网络与连接数；内存不是唯一瓶颈，单线程阻塞同样会超时。
