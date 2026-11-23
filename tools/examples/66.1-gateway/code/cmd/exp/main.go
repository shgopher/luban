// 实验驱动：一个二进制覆盖 A/B/C/D 四组实测，避免多份重复代码。
//
//	go run ./cmd/exp -mode=a -n 4000 ...   单机连接成本
//	go run ./cmd/exp -mode=b ...           跨网关卡投递
//	go run ./cmd/exp -mode=c ...           重连风暴
//	go run ./cmd/exp -mode=d ...           建连成本拆解
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"im-gw/internal/resp"
)

var (
	mode    = flag.String("mode", "a", "实验模式 a/b/c/d")
	n       = flag.Int("n", 4000, "连接数 / 消息数 / 客户端数")
	gw1     = flag.String("gw1", "127.0.0.1:18081", "网关1 地址")
	gw2     = flag.String("gw2", "127.0.0.1:18082", "网关2 地址")
	redis   = flag.String("redis", "127.0.0.1:16381", "Redis 地址")
	readBuf = flag.Int("readbuf", 4096, "客户端读缓冲")
	storm   = flag.String("storm-mode", "fixed", "重连策略 fixed|backoff")
	spread  = flag.Bool("spread", false, "压测客户端是否分散源 IP（macOS 只认已配置地址，默认关）")
	waitSec = flag.Int("wait", 12, "重连风暴观测秒数")
	settle  = flag.Duration("settle", 400*time.Millisecond, "连接建立后的静置时间")
	desc    = flag.String("desc", "", "本次配置说明，原样打印")
	setLimit = flag.Int("set-limit", 0, "实验 C：断开后把网关建连限速设为 N/s（0=不限）")
)

func main() {
	flag.Parse()
	var err error
	switch *mode {
	case "a":
		err = expA()
	case "b":
		err = expB()
	case "c":
		err = expC()
	case "d":
		err = expD()
	case "e":
		err = expE()
	case "f":
		err = expF()
	default:
		err = fmt.Errorf("unknown mode %s", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

// ---------- 公共小工具 ----------

// 票据写入共用一个长连接。反例：每张票新建一条 TCP，压测时先在客户端侧
// 耗光 16384 个临时端口（实测报 "can't assign requested address"）。
var (
	tkOnce sync.Once
	tkConn *resp.Conn
	tkMu   sync.Mutex
)

func mustTicket(uid string) string {
	tkOnce.Do(func() {
		c, err := resp.Dial(*redis, 2*time.Second)
		if err != nil {
			panic(err)
		}
		tkConn = c
	})
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	nonce := fmt.Sprintf("%x-%d", b, time.Now().UnixNano())
	tkMu.Lock()
	_, err := tkConn.Do("SET", "im:ticket:"+nonce, uid, "PX", 60000)
	tkMu.Unlock()
	if err != nil {
		panic(err)
	}
	return nonce
}

func wsURL(addr, ticket string) string {
	return "ws://" + addr + "/ws?ticket=" + ticket
}

type frame struct {
	Type string `json:"type"`
	Seq  int64  `json:"seq"`
	Body string `json:"body"`
	Peers []string `json:"peers"`
}

type client struct {
	ws   *websocket.Conn
	msgs chan frame
	done chan struct{}
	once sync.Once
}

// srcIdx 让压测客户端分散到 127.0.0.0/8 的不同源地址上。
// 单源地址只有 16384 个临时端口，4000 条连接跑两轮就 TIME_WAIT 耗尽（实测报
// "can't assign requested address"）——这是压测客户端自己的上限，不是网关的。
var srcIdx atomic.Int64

func dialWS(addr, ticket string, rbuf int) (*client, error) {
	d := websocket.Dialer{ReadBufferSize: rbuf, WriteBufferSize: rbuf, HandshakeTimeout: 10 * time.Second}
	if *spread {
		i := srcIdx.Add(1)
		ip := net.ParseIP(fmt.Sprintf("127.0.%d.%d", (i/250)%250+1, i%250+1))
		d.NetDial = func(network, address string) (net.Conn, error) {
			nd := net.Dialer{LocalAddr: &net.TCPAddr{IP: ip}, Timeout: 5 * time.Second}
			return nd.Dial(network, address)
		}
	}
	ws, resp, err := d.Dial(wsURL(addr, ticket), nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("%w status=%d", err, resp.StatusCode)
		}
		return nil, err
	}
	c := &client{ws: ws, msgs: make(chan frame, 256), done: make(chan struct{})}
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				c.close()
				return
			}
			var f frame
			if json.Unmarshal(data, &f) == nil {
				select {
				case c.msgs <- f:
				case <-c.done:
					return
				}
			}
		}
	}()
	return c, nil
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s -> %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}

func postJSON(url string, in any, out any) error {
	b, _ := json.Marshal(in)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s -> %d %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func metrics(addr string) map[string]int64 {
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		return map[string]int64{}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	m := map[string]int64{}
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		var v int64
		fmt.Sscan(f[1], &v)
		m[f[0]] = v
	}
	return m
}

func gcStats(addr string) map[string]int64 {
	var out map[string]any
	_ = getJSON("http://"+addr+"/debug/gc", &out)
	m := map[string]int64{}
	for k, v := range out {
		if f, ok := v.(float64); ok {
			m[k] = int64(f)
		}
	}
	return m
}

func redisCmdCalls(addr string) map[string]int64 {
	c, err := resp.Dial(addr, 2*time.Second)
	if err != nil {
		return nil
	}
	defer c.Close()
	v, err := c.Do("INFO", "commandstats")
	if err != nil {
		return nil
	}
	s, _ := v.(string)
	out := map[string]int64{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "cmdstat_") {
			continue
		}
		// cmdstat_get:calls=1000,usec=...,usec_per_call=...
		name := strings.TrimPrefix(strings.SplitN(line, ":", 2)[0], "cmdstat_")
		if i := strings.Index(line, "calls="); i >= 0 {
			rest := line[i+len("calls="):]
			if j := strings.IndexByte(rest, ','); j >= 0 {
				rest = rest[:j]
			}
			var v int64
			fmt.Sscan(rest, &v)
			out[name] = v
		}
	}
	return out
}

func pct(v []time.Duration) (p50, p99, p999, max time.Duration) {
	if len(v) == 0 {
		return
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	pick := func(p float64) time.Duration {
		idx := int(float64(len(v)) * p)
		if idx >= len(v) {
			idx = len(v) - 1
		}
		return v[idx]
	}
	return pick(0.50), pick(0.99), pick(0.999), v[len(v)-1]
}

// ---------- 实验 A：单机连接成本 ----------

func expA() error {
	// 1) 先测本进程真实 fd 上限：这是「单机连接数」的第一个硬顶。
	fdCap, err := probeFD()
	if err != nil {
		return err
	}
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1\n")
	fmt.Printf("FD_CEILING\t%d\n", fdCap)

	before := gcStats(*gw1)
	base := metrics(*gw1)

	// 2) 预置一次性票据（真实系统由登录服务写），避免把 Redis 写入算进建连速率。
	tickets := make([]string, *n)
	for i := 0; i < *n; i++ {
		tickets[i] = mustTicket(fmt.Sprintf("u%d", i))
	}

	clients := make([]*client, *n)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 256) // 同时在建的连接数
	var dialErr atomic.Int64
	var failMu sync.Mutex
	fails := map[string]int{}
	start := time.Now()
	for i := 0; i < *n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			c, err := dialWS(*gw1, tickets[i], *readBuf)
			if err != nil {
				dialErr.Add(1)
				failMu.Lock()
				fails[err.Error()]++
				failMu.Unlock()
				return
			}
			clients[i] = c
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	time.Sleep(*settle) // 等 metrics / GC 稳定
	after := gcStats(*gw1)
	m := metrics(*gw1)

	ok := *n - int(dialErr.Load())
	dialFailStr := ""
	for k, v := range fails {
		dialFailStr += fmt.Sprintf("DIAL_FAIL\t%d x %s\n", v, k)
	}
	if ok == 0 {
		return fmt.Errorf("0 successful connections\n%s", dialFailStr)
	}
	perConnHeap := int64(0)
	perConnSys := int64(0)
	if ok > 0 {
		perConnHeap = (after["heap_alloc"] - before["heap_alloc"]) / int64(ok)
		perConnSys = (after["sys"] - before["sys"]) / int64(ok)
	}
	fmt.Printf("CONNS\twant=%d ok=%d dialerr=%d\n", *n, ok, dialErr.Load())
	fmt.Print(dialFailStr)
	fmt.Printf("CONNECT_RATE\t%.0f conns/s (total %v)\n", float64(ok)/elapsed.Seconds(), elapsed.Round(time.Millisecond))
	fmt.Printf("SERVER_GOROUTINES\tbefore=%d after=%d delta_per_conn=%.2f\n",
		before["goroutines"], after["goroutines"], float64(after["goroutines"]-before["goroutines"])/float64(ok))
	fmt.Printf("SERVER_HEAP\tbefore=%d after=%d bytes/conn=%d\n", before["heap_alloc"], after["heap_alloc"], perConnHeap)
	fmt.Printf("SERVER_SYS\tbefore=%d after=%d bytes/conn=%d\n", before["sys"], after["sys"], perConnSys)
	fmt.Printf("ONLINE_METRIC\t%d\n", m["im_gw_online_conns"])
	fmt.Printf("AUTH_AVG_US\t%.1f\n", float64(m["im_gw_auth_ns_total"])/float64(max64(m["im_gw_auth_count"], 1))/1000)

	// 3) 内核 socket 缓冲：客户端侧 getsockopt，证明「每连接内存」不止用户态那份。
	var rcv, snd int
	if clients[0] != nil {
		raw := clients[0].ws.UnderlyingConn()
		if tc, ok := raw.(*net.TCPConn); ok {
			if sc, err := tc.SyscallConn(); err == nil {
				_ = sc.Control(func(fd uintptr) {
					rcv, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
					snd, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
				})
			}
		}
	}
	fmt.Printf("KERNEL_SOCKBUF\tSO_RCVBUF=%d SO_SNDBUF=%d (both ends, ×2 processes)\n", rcv, snd)

	// 4) 100 万连接的推算（同机线性外推，仅作量级）
	if perConnHeap > 0 {
		fmt.Printf("EXTRAPOLATE_1M\theap=%.1f GiB sys=%.1f GiB fds_needed=1000000\n",
			float64(perConnHeap)*1e6/(1<<30), float64(perConnSys)*1e6/(1<<30))
	}
	fmt.Printf("BASELINE_ONLINE\t%d\n", base["im_gw_online_conns"])

	// 收尾顺序很关键：先让服务端主动断（TIME_WAIT 落在网关侧），
	// 压测端的临时端口才能立刻复用，否则下一轮 4000 连接会报
	// "can't assign requested address"（源端口耗尽）。
	_, _ = http.Get("http://" + *gw1 + "/admin/kick-all")
	time.Sleep(500 * time.Millisecond)
	for _, c := range clients {
		if c != nil {
			c.close()
		}
	}
	time.Sleep(time.Second)
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func probeFD() (int, error) {
	var fds []*os.File
	limit := 200000
	for i := 0; i < limit; i++ {
		f, err := os.OpenFile("/dev/null", os.O_RDONLY, 0)
		if err != nil {
			break
		}
		fds = append(fds, f)
	}
	n := len(fds)
	for _, f := range fds {
		_ = f.Close()
	}
	return n, nil
}

// ---------- 实验 B：跨网关卡投递 ----------

func expB() error {
	uid := fmt.Sprintf("route-%d", time.Now().UnixNano())
	fmt.Printf("ROUTE_UID\t%s\n", uid)
	ticket := mustTicket(uid)
	c, err := dialWS(*gw1, ticket, 4096)
	if err != nil {
		return fmt.Errorf("connect gw1: %w", err)
	}
	defer c.close()
	time.Sleep(300 * time.Millisecond)

	before := redisCmdCalls(*redis)
	t0 := make(map[int64]time.Time, *n)
	recv := make(map[int64]time.Duration, *n)
	var mu sync.Mutex
	collector := make(chan struct{})
	go func() {
		for {
			select {
			case f := <-c.msgs:
				now := time.Now()
				mu.Lock()
				if st, ok := t0[f.Seq]; ok {
					recv[f.Seq] = now.Sub(st)
				}
				mu.Unlock()
			case <-collector:
				return
			}
		}
	}()

	// 业务侧把下行 push 打到 gw2：gw2 上并没有这条连接，必须走路由表。
	for i := int64(0); i < int64(*n); i++ {
		mu.Lock()
		t0[i] = time.Now()
		mu.Unlock()
		var out map[string]any
		if err := postJSON("http://"+*gw2+"/api/send", map[string]any{
			"uid": uid, "seq": i, "body": "hello",
		}, &out); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(recv)
		mu.Unlock()
		if got >= *n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(collector)
	mu.Lock()
	lat := make([]time.Duration, 0, len(recv))
	for _, d := range recv {
		lat = append(lat, d)
	}
	got := len(recv)
	mu.Unlock()
	after := redisCmdCalls(*redis)
	m := metrics(*gw2)

	p50, p99, p999, mx := pct(lat)
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1 / 网关与 Redis 同机\n")
	fmt.Printf("DELIVER\twant=%d got=%d lost=%d\n", *n, got, *n-got)
	fmt.Printf("E2E_LATENCY\tp50=%v p99=%v p99.9=%v max=%v\n", p50.Round(time.Microsecond), p99.Round(time.Microsecond), p999.Round(time.Microsecond), mx.Round(time.Microsecond))
	fmt.Printf("CONFIG\t%s\n", *desc)
	fmt.Printf("REDIS_GET_DELTA\t%d (messages=%d)\n", after["get"]-before["get"], *n)
	fmt.Printf("REDIS_PUBLISH_DELTA\t%d\n", after["publish"]-before["publish"])
	fmt.Printf("GW2_ROUTE_LOOKUP\t%d cache_hit=%d delivered=%d route_miss=%d offline=%d\n",
		m["im_gw_route_lookup_total"], m["im_gw_route_cache_hit_total"], m["im_gw_push_delivered_total"],
		m["im_gw_route_miss_total"], m["im_gw_offline_total"])

	// 离线路径：uid 压根没有路由
	var off map[string]any
	_ = postJSON("http://"+*gw2+"/api/send", map[string]any{"uid": "nobody-" + uid, "seq": 999999, "body": "x"}, &off)
	fmt.Printf("OFFLINE_PATH\tresp=%v\n", off)

	// 无路由层对照：直接推 gw2 本机连接 → 必然 miss
	var loc map[string]any
	_ = postJSON("http://"+*gw2+"/api/send-local?uid="+uid, map[string]any{}, &loc)
	fmt.Printf("NO_ROUTING_LAYER\tsend-local-on-gw2=%v\n", loc)

	// 路由失效窗口：让 gw1 的这条连接失效但路由表残留（模拟目标网关进程崩溃），
	// 直接删掉 Redis 里的连接登记不现实，这里用「杀掉订阅者」等价路径：
	// 让 gw1 断开且不执行清理 —— 通过 Redis 侧伪造一条指向不存在的网关的路由。
	if cdb, err := resp.Dial(*redis, 2*time.Second); err == nil {
		_, _ = cdb.Do("SET", "im:route:ghost-"+uid, "gw-dead", "PX", 60000)
		cdb.Close()
	}
	var stale map[string]any
	_ = postJSON("http://"+*gw2+"/api/send", map[string]any{"uid": "ghost-" + uid, "seq": 1, "body": "x"}, &stale)
	fmt.Printf("STALE_ROUTE\tresp=%v\n", stale)
	// 清理前核对 Redis 里的路由表（这才是「可验」要粘贴的东西）
	if cdb, err := resp.Dial(*redis, 2*time.Second); err == nil {
		v, _ := cdb.Do("GET", "im:route:"+uid)
		keys, _ := cdb.Do("KEYS", "im:route:*")
		if arr, ok := keys.([]any); ok {
			fmt.Printf("ROUTE_CHECK\tGET im:route:%s -> %v ; keys(im:route:*)=%d\n", uid, v, len(arr))
		}
		cdb.Close()
	}
	fmt.Printf("FAIL_MODE\t路由指向已死网关时，PUBLISH 返回 0 个订阅者 = 这 60s 内的推送全部静默丢失\n")
	_, _ = http.Get("http://" + *gw1 + "/admin/kick-all")
	time.Sleep(200 * time.Millisecond)
	c.close()
	return nil
}

// ---------- 实验 C：重连风暴 ----------

func expC() error {
	total := *n
	const tk = 6 // 每客户端 6 张一次性票：首连 1 张，最多 5 次重连尝试
	tickets := make([][tk]string, total)
	for i := 0; i < total; i++ {
		uid := fmt.Sprintf("storm%d-%d", time.Now().UnixNano(), i)
		for j := 0; j < tk; j++ {
			tickets[i][j] = mustTicket(uid)
		}
	}
	clients := make([]*client, total)
	sem := make(chan struct{}, 128)
	var wg sync.WaitGroup
	var connectErr atomic.Int64
	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			c, err := dialWS(*gw1, tickets[i][0], 4096)
			if err != nil {
				connectErr.Add(1)
				return
			}
			clients[i] = c
		}(i)
	}
	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	pre := metrics(*gw1)
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1 / 同机并行实验\n")
	fmt.Printf("PRECONDITION\tonline=%d connect_err=%d storm_mode=%s interval_base=1s wait=%ds\n",
		pre["im_gw_online_conns"], connectErr.Load(), *storm, *waitSec)

	// 服务端主动断开：等价于网关进程重启 / 网络分区。
	// 由服务端先发 FIN，压测端不会留 2000 个 TIME_WAIT 端口，重连才测得准。
	_, _ = http.Get("http://" + *gw1 + "/admin/kick-all")
	time.Sleep(500 * time.Millisecond)
	kicked := metrics(*gw1)
	fmt.Printf("AFTER_DISCONNECT\tonline=%d accepted=%d\n", kicked["im_gw_online_conns"], kicked["im_gw_conn_accepted_total"])
	if *setLimit > 0 {
		var lim map[string]any
		_ = postJSON(fmt.Sprintf("http://%s/admin/limit?rate=%d&burst=%d", *gw1, *setLimit, *setLimit), map[string]any{}, &lim)
		fmt.Printf("RATE_LIMIT\tset %v\n", lim)
	}

	// 采样网关计数（每 250ms 一次），用来还原每秒 accept 数。
	type sample struct {
		at       time.Duration
		accepted int64
		rejected int64
		online   int64
	}
	var samples []sample
	var sampleMu sync.Mutex
	stopSample := make(chan struct{})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		start := time.Now()
		for {
			select {
			case <-t.C:
				m := metrics(*gw1)
				if len(m) == 0 {
					continue // 高负载下偶发拿不到 metrics，跳过而不是记 0
				}
				sampleMu.Lock()
				samples = append(samples, sample{time.Since(start), m["im_gw_conn_accepted_total"],
					m["im_gw_conn_rejected_total"], m["im_gw_online_conns"]})
				sampleMu.Unlock()
			case <-stopSample:
				return
			}
		}
	}()

	var reOK, reFail atomic.Int64
	var failMu sync.Mutex
	fails := map[string]int{}
	start := time.Now()
	recovered := make(chan struct{})
	var done atomic.Int64
	for i := 0; i < total; i++ {
		if clients[i] == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if done.Add(1) == int64(total-int(connectErr.Load())) {
					close(recovered)
				}
			}()
			for attempt := 1; attempt < tk; attempt++ {
				var delay time.Duration
				if *storm == "fixed" {
					delay = time.Second // 反模式：所有人固定 1s，等于把风暴重新排一次
				} else {
					// 指数退避 + full jitter：delay ∈ [0, min(60s, 1s*2^(n-1))]
					cc := 60 * time.Second
					d := time.Second << (attempt - 1)
					if d > cc {
						d = cc
					}
					delay = time.Duration(mrand.Int63n(int64(d) + 1))
				}
				time.Sleep(delay)
				c, err := dialWS(*gw1, tickets[i][attempt], 4096)
				if err != nil {
					reFail.Add(1)
					failMu.Lock()
					fails[err.Error()]++
					failMu.Unlock()
					continue
				}
				clients[i] = c
				reOK.Add(1)
				return
			}
		}(i)
	}
	select {
	case <-recovered:
	case <-time.After(time.Duration(*waitSec) * time.Second):
	}
	recoveryDur := time.Since(start)
	time.Sleep(300 * time.Millisecond)
	close(stopSample)
	time.Sleep(300 * time.Millisecond)

	sampleMu.Lock()
	perSec := map[int]int64{}
	for i := 1; i < len(samples); i++ {
		sec := int(samples[i].at / time.Second)
		perSec[sec] += samples[i].accepted - samples[i-1].accepted
	}
	maxSec := 0
	for sec := range perSec {
		if sec > maxSec {
			maxSec = sec
		}
	}
	if lastSec := int(recoveryDur / time.Second); lastSec > maxSec {
		maxSec = lastSec
	}
	sampleMu.Unlock()
	fmt.Printf("RECONNECT\ttotal=%d ok=%d fail_attempts=%d all_recovered_in=%v\n", total, reOK.Load(), reFail.Load(), recoveryDur.Round(100*time.Millisecond))
	for sec := 0; sec <= maxSec && sec < *waitSec+3; sec++ {
		fmt.Printf("ACCEPT_PER_SEC\ts=%ds count=%d\n", sec, perSec[sec])
	}
	peak := int64(0)
	for _, v := range perSec {
		if v > peak {
			peak = v
		}
	}
	end := metrics(*gw1)
	failMu.Lock()
	for k, v := range fails {
		fmt.Printf("RETRY_FAIL\t%d x %s\n", v, k)
	}
	failMu.Unlock()
	fmt.Printf("SUMMARY\tonline=%d accepted_delta=%d rejected_delta=%d\n",
		end["im_gw_online_conns"],
		end["im_gw_conn_accepted_total"]-kicked["im_gw_conn_accepted_total"],
		end["im_gw_conn_rejected_total"]-kicked["im_gw_conn_rejected_total"])
	fmt.Printf("PEAK_ACCEPT_PER_SEC\t%d\n", peak)
	// 服务端先断，压测端端口立即可复用，便于连续跑多组
	_, _ = http.Get("http://" + *gw1 + "/admin/kick-all")
	time.Sleep(300 * time.Millisecond)
	for _, c := range clients {
		if c != nil {
			c.close()
		}
	}
	return nil
}

// ---------- 实验 E：优雅下线（drain + redirect）----------

func expE() error {
	total := *n
	tickets := make([][2]string, total)
	for i := 0; i < total; i++ {
		uid := fmt.Sprintf("drain%d-%d", time.Now().UnixNano(), i)
		tickets[i][0] = mustTicket(uid)
		tickets[i][1] = mustTicket(uid)
	}
	clients := make([]*client, total)
	sem := make(chan struct{}, 128)
	var wg sync.WaitGroup
	var connErr atomic.Int64
	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			c, err := dialWS(*gw1, tickets[i][0], 4096)
			if err != nil {
				connErr.Add(1)
				return
			}
			clients[i] = c
		}(i)
	}
	wg.Wait()
	time.Sleep(300 * time.Millisecond)
	pre1, pre2 := metrics(*gw1), metrics(*gw2)
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1 / gw1+gw2 同机\n")
	fmt.Printf("BEFORE\tgw1_online=%d gw2_online=%d connect_err=%d\n", pre1["im_gw_online_conns"], pre2["im_gw_online_conns"], connErr.Load())

	var redirected, reconnected atomic.Int64
	start := time.Now()
	drainDone := make(chan map[string]any, 1)
	go func() {
		var out map[string]any
		_ = postJSON("http://"+*gw1+"/admin/drain", map[string]any{}, &out)
		drainDone <- out
	}()
	// 客户端侧：收到 redirect 就按新网关地址重连（每客户端第二张票）。
	for i := 0; i < total; i++ {
		if clients[i] == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case f := <-clients[i].msgs:
				if f.Type != "redirect" {
					return
				}
				redirected.Add(1)
				clients[i].close()
				if len(f.Peers) == 0 {
					return
				}
				nc, err := dialWS(strings.Split(f.Peers[0], ":")[0]+":"+strings.Split(f.Peers[0], ":")[1], tickets[i][1], 4096)
				if err != nil {
					return
				}
				clients[i] = nc
				reconnected.Add(1)
			case <-time.After(5 * time.Second):
			}
		}(i)
	}
	out := <-drainDone
	drainDur := time.Since(start)
	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	post1, post2 := metrics(*gw1), metrics(*gw2)
	fmt.Printf("DRAIN_RESP\t%v\n", out)
	fmt.Printf("DRAIN_DURATION\t%v\n", drainDur.Round(10*time.Millisecond))
	fmt.Printf("REDIRECT\tsent=%d redirected=%d reconnected=%d\n", total, redirected.Load(), reconnected.Load())
	fmt.Printf("AFTER\tgw1_online=%d gw2_online=%d gw1_draining=%d\n", post1["im_gw_online_conns"], post2["im_gw_online_conns"], post1["im_gw_draining"])
	fmt.Printf("LOSS\t掉线未重连=%d\n", int64(total)-reconnected.Load())
	_, _ = http.Get("http://" + *gw2 + "/admin/kick-all")
	time.Sleep(200 * time.Millisecond)
	return nil
}

// ---------- 实验 F：心跳与「假在线」检测 ----------
// 需要网关用短心跳启动：-heartbeat 1s -hb-ack 2s
func expF() error {
	tk := mustTicket(fmt.Sprintf("hb-%d", time.Now().UnixNano()))
	// 故意不启动读循环：gorilla 只在 ReadMessage 期间自动回 pong。
	// 这样 TCP 还活着、但客户端永不应答心跳 —— 等价于 NAT 映射被回收 / App 进程假死。
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := d.Dial(wsURL(*gw1, tk), nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	pre := metrics(*gw1)
	start := time.Now()
	var detected time.Duration
	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		m := metrics(*gw1)
		if m["im_gw_ping_timeout_total"] > pre["im_gw_ping_timeout_total"] {
			detected = time.Since(start)
			break
		}
	}
	m := metrics(*gw1)
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1 / gw1 -heartbeat 1s -hb-ack 2s\n")
	fmt.Printf("HEARTBEAT_DETECT\tping_timeout_total %d -> %d, 检测耗时=%v\n",
		pre["im_gw_ping_timeout_total"], m["im_gw_ping_timeout_total"], detected.Round(100*time.Millisecond))
	fmt.Printf("DERIVE\t假在线检测延迟上限 ≈ hb-ack（本实验 2s；生产 60s = 用户最多 60s 还在被投递）\n")
	return nil
}

// ---------- 实验 D：建连成本拆解 ----------

func expD() error {
	fmt.Printf("ENV\tmacOS 15.6 / arm64 / go1.27.1\n")
	// ① Redis GETDEL（一次性票据核销）
	{
		c, err := resp.Dial(*redis, 2*time.Second)
		if err != nil {
			return err
		}
		defer c.Close()
		var ds []time.Duration
		for i := 0; i < 2000; i++ {
			key := fmt.Sprintf("bench:%d", i)
			_, _ = c.Do("SET", key, "u1", "PX", 60000)
			t := time.Now()
			_, _ = c.Do("GETDEL", key)
			ds = append(ds, time.Since(t))
		}
		p50, p99, _, mx := pct(ds)
		fmt.Printf("REDIS_GETDEL\tp50=%v p99=%v max=%v\n", p50.Round(time.Microsecond), p99.Round(time.Microsecond), mx.Round(time.Microsecond))
	}
	// ② HMAC-SHA256 票据验签（JWT HS256 的核心开销）
	{
		key := []byte("ticket-secret")
		payload := make([]byte, 512)
		_, _ = rand.Read(payload)
		var ds []time.Duration
		for i := 0; i < 200000; i++ {
			t := time.Now()
			mac := hmac.New(sha256.New, key)
			mac.Write(payload)
			_ = mac.Sum(nil)
			ds = append(ds, time.Since(t))
		}
		p50, p99, _, _ := pct(ds)
		fmt.Printf("JWT_HS256_VERIFY\tp50=%v p99=%v (512B payload)\n", p50.Round(10*time.Nanosecond), p99.Round(10*time.Nanosecond))
	}
	// ③ loopback TLS 1.3 握手（自签证书，进程内生成）：RSA-2048 vs ECDSA P-256
	for _, alg := range []struct {
		name string
		gen  func() (tls.Certificate, error)
	}{{"RSA2048", selfSignedRSA}, {"ECDSA-P256", selfSignedECDSA}} {
		cert, err := alg.gen()
		if err != nil {
			return err
		}
		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
		if err != nil {
			return err
		}
		defer ln.Close()
		go func() {
			for {
				cc, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					_ = cc.(*tls.Conn).Handshake()
					cc.Close()
				}()
			}
		}()
		cfg := &tls.Config{InsecureSkipVerify: true}
		var ds []time.Duration
		for i := 0; i < 300; i++ {
			t := time.Now()
			cc, err := tls.Dial("tcp", ln.Addr().String(), cfg)
			if err != nil {
				return err
			}
			_ = cc.Handshake()
			ds = append(ds, time.Since(t))
			cc.Close()
		}
		p50, p99, _, mx := pct(ds)
		fmt.Printf("TLS13_HANDSHAKE_%s\tp50=%v p99=%v max=%v\n", alg.name, p50.Round(time.Microsecond), p99.Round(time.Microsecond), mx.Round(time.Microsecond))
	}
	// ④ websocket 升级（鉴权 + HTTP 101 + 注册路由）
	{
		var ds []time.Duration
		var cls []*client
		for i := 0; i < 500; i++ {
			tk := mustTicket(fmt.Sprintf("hs%d-%d", time.Now().UnixNano(), i))
			t := time.Now()
			c, err := dialWS(*gw1, tk, 4096)
			if err != nil {
				return err
			}
			ds = append(ds, time.Since(t))
			cls = append(cls, c)
		}
		for _, c := range cls {
			c.close()
		}
		p50, p99, _, mx := pct(ds)
		fmt.Printf("WS_UPGRADE_WITH_AUTH\tp50=%v p99=%v max=%v (含 Redis GETDEL+SET 路由)\n", p50.Round(time.Microsecond), p99.Round(time.Microsecond), mx.Round(time.Microsecond))
	}
	fmt.Printf("DERIVE\t单核建连上限 ≈ 1 / 单次建连耗时；实测值见上，限速阈值取 50%% 左右\n")
	return nil
}

func selfSignedECDSA() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "bench.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func selfSignedRSA() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "bench.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}
