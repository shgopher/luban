package main

// 租户路由层的三个硬要求，用代码固定下来：
// 1) 拿不到映射 -> fail closed，绝不落到 default 库；
// 2) 映射走本地缓存（命中 = 内存读），未命中才查元数据表；
// 3) 变更走版本号通知：版本不一致时缓存立刻失效，而不是等 TTL。
import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// route 是元数据表里的一行：租户 -> 数据源。State 用于下线/迁移中的租户。
type route struct {
	Tenant  string
	DSN     string // 真实系统里是数据源 key，不直接放连接串
	State   string // active / migrating / disabled
	Version int64
}

var ErrNoRoute = errors.New("no route for tenant: fail closed")

// metaStore 模拟元数据表：每次查询 2ms（一次索引点查 + 网络往返的量级）。
type metaStore struct {
	mu   sync.Mutex
	rows map[string]route
}

func (m *metaStore) get(tenant string) (route, bool) {
	time.Sleep(2 * time.Millisecond) // 模拟一次远程点查
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[tenant]
	return r, ok
}

func (m *metaStore) set(r route) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[r.Tenant] = r
}

type entry struct {
	dsn     string
	version int64
	expire  time.Time
}

// router：本地缓存 + 版本号。缓存里只放“查到的行”，不放“查不到”。
type router struct {
	meta     *metaStore
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]entry
	routeVer int64 // 由变更通知（订阅元数据表的 binlog / 消息）推进
	hits     int
	misses   int
}

func newRouter(m *metaStore, ttl time.Duration) *router {
	return &router{meta: m, ttl: ttl, cache: make(map[string]entry)}
}

// bumpVersion 模拟收到变更通知：整体或按租户推进版本。
func (r *router) bumpVersion(v int64) {
	r.mu.Lock()
	r.routeVer = v
	r.mu.Unlock()
}

func (r *router) resolve(tenant string) (string, error) {
	r.mu.RLock()
	e, ok := r.cache[tenant]
	ver := r.routeVer
	r.mu.RUnlock()
	if ok && e.version == ver && time.Now().Before(e.expire) {
		r.mu.Lock()
		r.hits++
		r.mu.Unlock()
		return e.dsn, nil
	}

	row, found := r.meta.get(tenant) // 缓存未命中：允许这一次远程点查
	r.mu.Lock()
	r.misses++
	r.mu.Unlock()
	if !found {
		// 关键：查不到就报错。这里如果 return "db-default", nil，就等于把所有
		// 新租户 / 被删租户的数据写进同一个库。
		return "", fmt.Errorf("%w: %s", ErrNoRoute, tenant)
	}
	if row.State != "active" {
		return "", fmt.Errorf("%w: %s state=%s", ErrNoRoute, tenant, row.State)
	}
	r.mu.Lock()
	r.cache[tenant] = entry{dsn: row.DSN, version: r.routeVer, expire: time.Now().Add(r.ttl)}
	r.mu.Unlock()
	return row.DSN, nil
}

func main() {
	meta := &metaStore{rows: map[string]route{
		"t-001": {Tenant: "t-001", DSN: "pool-a", State: "active", Version: 1},
		"t-002": {Tenant: "t-002", DSN: "pool-a", State: "active", Version: 1},
		"t-003": {Tenant: "t-003", DSN: "dedicated-9", State: "active", Version: 1},
		"t-004": {Tenant: "t-004", DSN: "pool-a", State: "migrating", Version: 7},
	}}
	r := newRouter(meta, 30*time.Second)

	fmt.Println("| 请求租户 | 解析出的数据源 | 结果 |")
	fmt.Println("|---|---|---|")
	// t-005 在元数据表里根本没有：这是“新租户还没建库”与“租户已删除”的同一副面孔。
	for _, t := range []string{"t-001", "t-002", "t-003", "t-004", "t-005"} {
		dsn, err := r.resolve(t)
		if err != nil {
			fmt.Printf("| %s | （无） | ⛔ %v |\n", t, err)
			continue
		}
		fmt.Printf("| %s | %s | ✅ 路由 |\n", t, dsn)
	}

	// 版本号通知：t-003 从独立库迁到 pool-b，缓存必须立刻失效。
	r.bumpVersion(2) // 收到变更通知
	meta.set(route{Tenant: "t-003", DSN: "pool-b", State: "active", Version: 2})
	dsn, _ := r.resolve("t-003")
	fmt.Printf("\n收到变更通知后 t-003 -> %s（不依赖 TTL 过期）\n", dsn)

	// 缓存命中 vs 未命中的单次解析耗时。
	const N = 200000
	start := time.Now()
	for i := 0; i < N; i++ {
		_, _ = r.resolve("t-001") // 已缓存
	}
	hit := time.Since(start) / N
	start = time.Now()
	for i := 0; i < 200; i++ {
		r.mu.Lock()
		delete(r.cache, "t-001") // 强制回源
		r.mu.Unlock()
		_, _ = r.resolve("t-001")
	}
	miss := time.Since(start) / 200
	fmt.Printf("缓存命中 %d 次共耗时 %v，单次 %v\n", N, hit*N, hit)
	fmt.Printf("缓存未命中（回源 2ms 模拟）单次 %v，是命中的 %.0f 倍\n", miss, float64(miss)/float64(hit))
	fmt.Printf("命中率 %.4f（%d 命中 / %d 回源）\n", float64(r.hits)/float64(r.hits+r.misses), r.hits, r.misses)
}
