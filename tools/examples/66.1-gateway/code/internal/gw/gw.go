// Package gw 实现一个「只做连接 + 寻址 + 转发」的长连接网关最小可用版。
// 刻意不做的事：消息存储、群扩散、已读回执、ACK——那些属于逻辑层（66.2/66.5）。
package gw

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"im-gw/internal/resp"
)

// Config 网关启动参数。
type Config struct {
	ID           string        // 网关实例 ID，如 gw1
	Addr         string        // 监听地址，如 127.0.0.1:18081
	Advertise    string        // 注册到 Redis 的对外地址，如 127.0.0.1:18081
	RedisAddr    string        // Redis 地址
	ReadBuf      int           // websocket 读缓冲，字节
	WriteBuf     int           // websocket 写缓冲，字节
	Heartbeat    time.Duration // 心跳间隔
	HeartbeatAck time.Duration // 心跳应答窗口（读超时）
	RouteTTL     time.Duration // 路由表条目 TTL，也是崩溃场景下的失效窗口
	RouteCacheTTL time.Duration // 网关本地路由缓存 TTL
	DrainTimeout time.Duration // 优雅下线排空窗口
	LimitPerSec  int           // 建连限速：每秒补充令牌数，0 表示不限
	LimitBurst   int           // 建连限速：桶容量
	RouteCache   bool          // 下行路由是否使用本地缓存 + 失效通知
}

// Gateway 网关实例。
type Gateway struct {
	cfg Config
	srv *http.Server

	mu    sync.RWMutex
	conns map[string]*wsConn // uid -> 连接

	pool *redisPool

	draining atomic.Bool

	// 可观测计数，全部是可加的原子量，/metrics 直接读
	online        atomic.Int64
	accepted      atomic.Int64
	rejected      atomic.Int64
	authFail      atomic.Int64
	authNS        atomic.Int64
	authCount     atomic.Int64
	upgradeNS     atomic.Int64
	pingTimeout   atomic.Int64
	routeSet      atomic.Int64
	routeLookup   atomic.Int64
	routeCacheHit atomic.Int64
	routeMiss     atomic.Int64 // 拿到路由但本机没有连接（路由表陈旧）
	offline       atomic.Int64 // 根本没路由：用户不在线
	stale         atomic.Int64 // 路由指向已死网关：PUBLISH 无人接收
	delivered     atomic.Int64
	slowDrop      atomic.Int64

	limMu     sync.Mutex
	tokens    float64
	lastFill  time.Time
	limRate   atomic.Int64 // 运行时可调的建连限速（次/秒），0=不限
	limBurst  atomic.Int64
}

type wsConn struct {
	uid    string
	ws     *websocket.Conn
	send   chan []byte
	dead   chan struct{}
	once   sync.Once
	closed atomic.Bool
}

func (c *wsConn) close() {
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.dead)
		_ = c.ws.Close()
	})
}

type redisPool struct {
	addr string
	ch   chan *resp.Conn
}

func newPool(addr string, n int) *redisPool {
	p := &redisPool{addr: addr, ch: make(chan *resp.Conn, n)}
	for i := 0; i < n; i++ {
		if c, err := resp.Dial(addr, 2*time.Second); err == nil {
			p.ch <- c
		}
	}
	return p
}

func (p *redisPool) get() (*resp.Conn, error) {
	select {
	case c := <-p.ch:
		return c, nil
	default:
		return resp.Dial(p.addr, time.Second)
	}
}

func (p *redisPool) put(c *resp.Conn) {
	// 池满时直接关掉多出来的连接：绝不能阻塞在归还路径上，
	// 否则一次建连风暴就会把 HTTP handler 全部卡死（真实故障里这类 bug 极难查）。
	select {
	case p.ch <- c:
	default:
		_ = c.Close()
	}
}

// New 创建网关。
func New(cfg Config) *Gateway {
	if cfg.ReadBuf == 0 {
		cfg.ReadBuf = 4096
	}
	if cfg.WriteBuf == 0 {
		cfg.WriteBuf = 4096
	}
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 20 * time.Second
	}
	if cfg.HeartbeatAck == 0 {
		cfg.HeartbeatAck = 60 * time.Second
	}
	if cfg.RouteTTL == 0 {
		cfg.RouteTTL = 60 * time.Second
	}
	if cfg.RouteCacheTTL == 0 {
		cfg.RouteCacheTTL = 2 * time.Second
	}
	if cfg.DrainTimeout == 0 {
		cfg.DrainTimeout = 10 * time.Second
	}
	g := &Gateway{
		cfg:      cfg,
		conns:    make(map[string]*wsConn),
		pool:     newPool(cfg.RedisAddr, 8),
		tokens:   float64(cfg.LimitBurst),
		lastFill: time.Now(),
	}
	g.limRate.Store(int64(cfg.LimitPerSec))
	g.limBurst.Store(int64(cfg.LimitBurst))
	return g
}

// Start 打开监听，注册自身地址，启动订阅协程。
func (g *Gateway) Start() error {
	g.registerSelf()
	g.subscribe()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", g.handleWS)
	mux.HandleFunc("/api/send", g.handleSend)
	mux.HandleFunc("/api/send-local", g.handleSendLocal)
	mux.HandleFunc("/metrics", g.handleMetrics)
	mux.HandleFunc("/debug/gc", g.handleGC)
	mux.HandleFunc("/admin/drain", g.handleDrain)
	mux.HandleFunc("/admin/kick-all", g.handleKickAll)
	mux.HandleFunc("/admin/limit", g.handleLimit)
	g.srv = &http.Server{Addr: g.cfg.Addr, Handler: mux}
	return g.srv.ListenAndServe()
}

// Stop 关闭监听并摘除注册。
func (g *Gateway) Stop() {
	_ = g.srv.Close()
	if c, err := g.pool.get(); err == nil {
		_, _ = c.Do("HDEL", "im:gw:addr", g.cfg.ID)
		g.pool.put(c)
	}
}

func (g *Gateway) registerSelf() {
	if c, err := g.pool.get(); err == nil {
		_, _ = c.Do("HSET", "im:gw:addr", g.cfg.ID, g.cfg.Advertise)
		g.pool.put(c)
	}
}

// ---- 鉴权：一次性票据。登录服务写 im:ticket:<nonce>=uid，网关 GETDEL 原子消费 ----

func (g *Gateway) authTicket(ticket string) (string, bool) {
	start := time.Now()
	c, err := g.pool.get()
	if err != nil {
		return "", false
	}
	v, err := c.Do("GETDEL", "im:ticket:"+ticket)
	g.pool.put(c)
	g.authNS.Add(int64(time.Since(start)))
	g.authCount.Add(1)
	if err != nil || v == nil {
		return "", false
	}
	uid, _ := v.(string)
	return uid, uid != ""
}

func (g *Gateway) allowConn() bool {
	rate := g.limRate.Load()
	if rate <= 0 {
		return true
	}
	burst := g.limBurst.Load()
	now := time.Now()
	g.limMu.Lock()
	defer g.limMu.Unlock()
	g.tokens += now.Sub(g.lastFill).Seconds() * float64(rate)
	if g.tokens > float64(burst) {
		g.tokens = float64(burst)
	}
	g.lastFill = now
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	return true
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // 实验环境：跨源放行
}

func (g *Gateway) handleWS(w http.ResponseWriter, r *http.Request) {
	// ① 摘流量期间的连接直接拒绝，让客户端去别处；不做升级，省下每连接缓冲。
	if g.draining.Load() {
		g.rejected.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	}
	// ② 建连限速放在最前面：风暴时 CPU 花在鉴权和 TLS 之前就挡掉。
	if !g.allowConn() {
		g.rejected.Add(1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	// ③ 先鉴权再升级：无效连接不分配 websocket 缓冲。
	uid, ok := g.authTicket(r.URL.Query().Get("ticket"))
	if !ok {
		g.authFail.Add(1)
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return
	}
	up := upgrader
	up.ReadBufferSize = g.cfg.ReadBuf
	up.WriteBufferSize = g.cfg.WriteBuf
	t0 := time.Now()
	ws, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	g.upgradeNS.Add(int64(time.Since(t0)))

	c := &wsConn{uid: uid, ws: ws, send: make(chan []byte, 256), dead: make(chan struct{})}
	g.register(c)
	g.accepted.Add(1)
	g.online.Add(1)

	go g.writePump(c)
	g.readPump(c)
}

func (g *Gateway) register(c *wsConn) {
	g.mu.Lock()
	old := g.conns[c.uid]
	g.conns[c.uid] = c
	g.mu.Unlock()
	if old != nil {
		old.close() // 同账号后连接顶掉前连接
	}
	if cdb, err := g.pool.get(); err == nil {
		_, _ = cdb.Do("SET", "im:route:"+c.uid, g.cfg.ID, "PX", int(g.cfg.RouteTTL/time.Millisecond))
		g.pool.put(cdb)
		g.routeSet.Add(1)
		g.invalidate(c.uid, g.cfg.ID)
	}
}

func (g *Gateway) unregister(c *wsConn) {
	g.mu.Lock()
	if cur, ok := g.conns[c.uid]; ok && cur == c {
		delete(g.conns, c.uid)
	}
	g.mu.Unlock()
	c.close()
	g.online.Add(-1)
	// 只有当路由还指着我时才删；否则可能已经迁移到别的网关，删掉就是误删。
	if cdb, err := g.pool.get(); err == nil {
		script := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`
		_, _ = cdb.Do("EVAL", script, 1, "im:route:"+c.uid, g.cfg.ID)
		g.pool.put(cdb)
		g.invalidate(c.uid, g.cfg.ID)
	}
}

func (g *Gateway) readPump(c *wsConn) {
	defer g.unregister(c)
	c.ws.SetReadLimit(1 << 20)
	_ = c.ws.SetReadDeadline(time.Now().Add(g.cfg.HeartbeatAck))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(g.cfg.HeartbeatAck))
	})
	for {
		if _, _, err := c.ws.ReadMessage(); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				g.pingTimeout.Add(1)
			}
			return
		}
	}
}

func (g *Gateway) writePump(c *wsConn) {
	tick := time.NewTicker(g.cfg.Heartbeat)
	defer tick.Stop()
	for {
		select {
		case msg := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.close()
				return
			}
		case <-tick.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
			// 心跳顺带续期路由 TTL：在线用户的失效窗口 = RouteTTL - Heartbeat。
			if cdb, err := g.pool.get(); err == nil {
				_, _ = cdb.Do("SET", "im:route:"+c.uid, g.cfg.ID, "PX", int(g.cfg.RouteTTL/time.Millisecond))
				g.pool.put(cdb)
				g.routeSet.Add(1)
			}
		case <-c.dead:
			return
		}
	}
}

// ---- 下行路径：逻辑层 -> 路由查询 -> 目标网关 -> 连接 ----

func (g *Gateway) lookupRoute(uid string) (gwid, source string) {
	if g.cfg.RouteCache {
		g.mu.RLock()
		if c, ok := g.conns[uid]; ok && c != nil {
			// 本机有连接：零次路由查询，这是最热的一条路径。
			g.mu.RUnlock()
			g.routeCacheHit.Add(1)
			return g.cfg.ID, "local"
		}
		g.mu.RUnlock()
	}
	if gwid, ok := g.cacheGet(uid); ok {
		g.routeCacheHit.Add(1)
		return gwid, "cache"
	}
	g.routeLookup.Add(1)
	c, err := g.pool.get()
	if err != nil {
		return "", "error"
	}
	v, err := c.Do("GET", "im:route:"+uid)
	g.pool.put(c)
	if err != nil || v == nil {
		return "", "miss"
	}
	gwid, _ = v.(string)
	g.cacheSet(uid, gwid)
	return gwid, "redis"
}

type cacheEnt struct {
	gw  string
	exp time.Time
}

var routeCache = struct {
	sync.RWMutex
	m map[string]cacheEnt
}{m: make(map[string]cacheEnt)}

func (g *Gateway) cacheGet(uid string) (string, bool) {
	if !g.cfg.RouteCache {
		return "", false
	}
	routeCache.RLock()
	e, ok := routeCache.m[uid]
	routeCache.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return "", false
	}
	return e.gw, true
}

func (g *Gateway) cacheSet(uid, gwid string) {
	if !g.cfg.RouteCache {
		return
	}
	routeCache.Lock()
	routeCache.m[uid] = cacheEnt{gw: gwid, exp: time.Now().Add(g.cfg.RouteCacheTTL)}
	routeCache.Unlock()
}

func (g *Gateway) invalidate(uid, gwid string) {
	if c, err := g.pool.get(); err == nil {
		_, _ = c.Do("PUBLISH", "im:route:inv", uid+" "+gwid)
		g.pool.put(c)
	}
}

func (g *Gateway) subscribe() {
	c, err := resp.Dial(g.cfg.RedisAddr, 2*time.Second)
	if err != nil {
		return
	}
	if err := c.Send("PSUBSCRIBE", "gw:"+g.cfg.ID, "im:route:inv"); err != nil {
		return
	}
	go func() {
		for {
			v, err := c.ReadValue()
			if err != nil {
				return
			}
			arr, ok := v.([]any)
			if !ok {
				continue
			}
			kind, _ := arr[0].(string)
			switch kind {
			case "pmessage":
				// 注意：PSUBSCRIBE 推送的 pmessage 是 4 元组
				// ["pmessage", pattern, channel, payload]，不是 3 元组。
				// 按 3 元组解析会把 channel 名当成 payload，下行静默全丢。
				if len(arr) < 4 {
					continue
				}
				pattern, _ := arr[1].(string)
				payload, _ := arr[3].(string)
				if strings.HasPrefix(pattern, "gw:") {
					g.deliverLocal(payload)
				} else {
					var uid, gwid string
					fmt.Sscanf(payload, "%s %s", &uid, &gwid)
					routeCache.Lock()
					delete(routeCache.m, uid)
					routeCache.Unlock()
				}
			}
		}
	}()
}

func (g *Gateway) deliverLocal(payload string) {
	var m struct {
		UID  string `json:"uid"`
		Seq  int64  `json:"seq"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return
	}
	g.mu.RLock()
	c := g.conns[m.UID]
	g.mu.RUnlock()
	if c == nil {
		g.routeMiss.Add(1) // 路由说在这台机器，但连接已经没了：失效窗口
		return
	}
	frame, _ := json.Marshal(map[string]any{"type": "msg", "seq": m.Seq, "body": m.Body})
	select {
	case c.send <- frame:
		g.delivered.Add(1)
	default:
		g.slowDrop.Add(1) // 慢消费者：宁可断开也不让缓冲无限涨
		c.close()
	}
}

func (g *Gateway) handleSend(w http.ResponseWriter, r *http.Request) {
	var m struct {
		UID  string `json:"uid"`
		Seq  int64  `json:"seq"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	gwid, src := g.lookupRoute(m.UID)
	if gwid == "" {
		g.offline.Add(1)
		writeJSON(w, map[string]any{"code": "offline", "route": src})
		return
	}
	payload, _ := json.Marshal(map[string]any{"uid": m.UID, "seq": m.Seq, "body": m.Body})
	var recv int64
	if c, err := g.pool.get(); err == nil {
		v, _ := c.Do("PUBLISH", "gw:"+gwid, string(payload))
		recv, _ = v.(int64)
		g.pool.put(c)
	}
	// PUBLISH 返回订阅者数：0 表示目标网关已经没了但路由还没过期 —— 这就是失效窗口里的静默丢失。
	if recv == 0 {
		g.stale.Add(1)
		writeJSON(w, map[string]any{"code": "stale", "gw": gwid, "route": src, "receivers": recv})
		return
	}
	writeJSON(w, map[string]any{"code": "ok", "gw": gwid, "route": src, "receivers": recv})
}

// handleSendLocal 故意不做路由查询，只查本机连接：用来证明「逻辑层直接推」必然 miss。
func (g *Gateway) handleSendLocal(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	g.mu.RLock()
	c := g.conns[uid]
	g.mu.RUnlock()
	if c == nil {
		g.routeMiss.Add(1)
		writeJSON(w, map[string]any{"code": "miss"})
		return
	}
	frame, _ := json.Marshal(map[string]any{"type": "msg", "seq": 0, "body": "local"})
	c.send <- frame
	writeJSON(w, map[string]any{"code": "ok"})
}

// ---- 优雅下线：摘流量 -> 删路由 -> redirect -> 排空 -> 强断 ----

func (g *Gateway) handleDrain(w http.ResponseWriter, r *http.Request) {
	g.draining.Store(true)
	peers := g.peers()
	g.mu.RLock()
	local := make([]*wsConn, 0, len(g.conns))
	for _, c := range g.conns {
		local = append(local, c)
	}
	g.mu.RUnlock()
	for _, c := range local {
		if cdb, err := g.pool.get(); err == nil {
			script := `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`
			_, _ = cdb.Do("EVAL", script, 1, "im:route:"+c.uid, g.cfg.ID)
			g.pool.put(cdb)
		}
		g.invalidate(c.uid, g.cfg.ID)
		frame, _ := json.Marshal(map[string]any{"type": "redirect", "peers": peers})
		select {
		case c.send <- frame:
		default:
		}
	}
	// 排空：给客户端时间收到 redirect 并主动断开，超时后强断。
	deadline := time.Now().Add(g.cfg.DrainTimeout)
	for time.Now().Before(deadline) {
		g.mu.RLock()
		n := len(g.conns)
		g.mu.RUnlock()
		if n == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	g.mu.RLock()
	left := len(g.conns)
	g.mu.RUnlock()
	g.forceCloseAll()
	writeJSON(w, map[string]any{"drained": left, "redirected": len(local)})
}

func (g *Gateway) peers() []string {
	c, err := g.pool.get()
	if err != nil {
		return nil
	}
	v, _ := c.Do("HGETALL", "im:gw:addr")
	g.pool.put(c)
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr)/2)
	for i := 0; i+1 < len(arr); i += 2 {
		if id, _ := arr[i].(string); id != g.cfg.ID {
			if addr, _ := arr[i+1].(string); addr != "" {
				out = append(out, addr)
			}
		}
	}
	return out
}

// handleLimit 运行时调整建连限速：rate=0 等于关掉限速（降级开关）。
func (g *Gateway) handleLimit(w http.ResponseWriter, r *http.Request) {
	var rate, burst int
	fmt.Sscan(r.URL.Query().Get("rate"), &rate)
	fmt.Sscan(r.URL.Query().Get("burst"), &burst)
	if burst <= 0 {
		burst = rate
	}
	g.limMu.Lock()
	g.tokens = float64(burst)
	g.lastFill = time.Now()
	g.limMu.Unlock()
	g.limRate.Store(int64(rate))
	g.limBurst.Store(int64(burst))
	writeJSON(w, map[string]any{"rate": rate, "burst": burst})
}

// handleKickAll 模拟进程被 kill / 网络分区：不发 redirect，直接断。
func (g *Gateway) handleKickAll(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	n := len(g.conns)
	local := make([]*wsConn, 0, n)
	for _, c := range g.conns {
		local = append(local, c)
	}
	g.mu.RUnlock()
	for _, c := range local {
		c.close()
	}
	// 故意不清路由表：整机被杀时路由只能靠 TTL 过期，这正是失效窗口的来源。
	writeJSON(w, map[string]any{"kicked": n})
}

func (g *Gateway) forceCloseAll() {
	g.mu.Lock()
	local := make([]*wsConn, 0, len(g.conns))
	for _, c := range g.conns {
		local = append(local, c)
	}
	g.mu.Unlock()
	for _, c := range local {
		c.close()
	}
}

func (g *Gateway) handleGC(w http.ResponseWriter, r *http.Request) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	writeJSON(w, map[string]any{
		"heap_alloc": ms.HeapAlloc, "heap_inuse": ms.HeapInuse, "sys": ms.Sys,
		"goroutines": runtime.NumGoroutine(), "num_gc": ms.NumGC,
	})
}

func (g *Gateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("gc") == "1" {
		runtime.GC()
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	pairs := []struct {
		name string
		v    int64
	}{
		{"im_gw_online_conns", g.online.Load()},
		{"im_gw_conn_accepted_total", g.accepted.Load()},
		{"im_gw_conn_rejected_total", g.rejected.Load()},
		{"im_gw_auth_fail_total", g.authFail.Load()},
		{"im_gw_auth_ns_total", g.authNS.Load()},
		{"im_gw_auth_count", g.authCount.Load()},
		{"im_gw_upgrade_ns_total", g.upgradeNS.Load()},
		{"im_gw_ping_timeout_total", g.pingTimeout.Load()},
		{"im_gw_route_set_total", g.routeSet.Load()},
		{"im_gw_route_lookup_total", g.routeLookup.Load()},
		{"im_gw_route_cache_hit_total", g.routeCacheHit.Load()},
		{"im_gw_route_miss_total", g.routeMiss.Load()},
		{"im_gw_offline_total", g.offline.Load()},
		{"im_gw_route_stale_total", g.stale.Load()},
		{"im_gw_push_delivered_total", g.delivered.Load()},
		{"im_gw_slow_drop_total", g.slowDrop.Load()},
		{"im_gw_goroutines", int64(runtime.NumGoroutine())},
		{"im_gw_heap_alloc_bytes", int64(ms.HeapAlloc)},
		{"im_gw_sys_bytes", int64(ms.Sys)},
		{"im_gw_draining", boolToInt(g.draining.Load())},
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, p := range pairs {
		fmt.Fprintf(w, "%s %d\n", p.name, p.v)
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
