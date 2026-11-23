// 实验 A：消息五段接力模拟（上行 ACK / 服务端持久化 / 下行投递 / 离线存储 / 推送通道）
// 虚拟时钟离散事件模拟（1 个单位 = 1ms），Redis 操作是真实执行（本机 16382）。
// 目的：证明「任意一段失败，端到端只能 at-least-once；去重键 + 会话 seq 让它业务等价 exactly-once」。
package main

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	N               = 10000
	sendIntervalMs  = 10
	netMs           = 15   // 单程网络耗时
	offlineStart1   = 25000 // 25s 起用户下线
	offlineEnd1     = 40000
	offlineStart2   = 70000 // 70s 起再次下线
	offlineEnd2     = 85000
	simEnd          = 200000
	retryBaseMs     = 200 // 客户端上行重试：200ms 起指数退避
	retryCapMs      = 3200
	maxUpAttempts   = 8
	downRetxMs      = 300 // 服务端下行未收到 ACK 的重传间隔
	maxDownRetx     = 10
	pullPageSize    = 200
	pushBatchMs     = 500
	wakePullDelayMs = 3000

	// 各段故障概率（每次尝试独立掷骰）
	pUpReqLoss   = 0.02 // 上行请求在网络上丢了：服务端根本没收到
	pUpRespLoss  = 0.02 // 服务端已持久化，但 ACK 回包丢了：客户端会重发
	pStoreFail   = 0.01 // 持久化失败：服务端 NACK，要求重发
	pDownLoss    = 0.02 // 下行推送包丢了：客户端没收到
	pDownAckLoss = 0.03 // 客户端收到了，但下行 ACK 丢了：服务端会重传
	pPullPage    = 0.05 // 拉取分页请求失败
	pCursorFail  = 0.02 // 客户端把游标写本地库失败：下一分页会重拉
	pPushDrop    = 0.05 // 推送通道丢通知：叫不醒，但消息仍在收件箱
)

var ctx = context.Background()

// ---------- 事件堆（虚拟时钟） ----------

type event struct {
	at    int64
	order int
	fn    func()
}

type evHeap []event

func (h evHeap) Len() int { return len(h) }
func (h evHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	return h[i].order < h[j].order
}
func (h evHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *evHeap) Push(x interface{}) { *h = append(*h, x.(event)) }
func (h *evHeap) Pop() interface{} {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

// ---------- 消息与模拟状态 ----------

type msg struct {
	i           int
	cmid        string // 客户端生成，重试必须复用
	seq         int64  // 服务端分配：会话内单调
	msgID       string // 服务端全局 ID（这里用 m-<seq> 简化）
	sentAt      int64  // 第一次发送时刻
	ackedAt     int64  // 收到上行 ACK 的时刻
	firstDeliv  int64
	deliveries  int
	viaPull     bool
	wasOffline  bool
	upAttempts  int
	downRetx    int
	storedAt    int64
}

type sim struct {
	h        evHeap
	order    int
	now      int64
	rng      *rand.Rand
	rdb      *redis.Client
	dedupe   *redis.Script
	msgs     []*msg
	bySeq    map[int64]*msg
	conv     string
	sender   string
	receiver string

	// 客户端侧
	received    map[int64]bool
	cursor      int64 // 已连续收到的最大 seq：只能按「连续前缀」推进
	pulling     bool
	pullQueued  bool
	pullPages   int
	pullMsgs    int
	pullKind    string

	// 服务端侧
	pendingPush   []*msg
	pushTimerOn   bool
	offlineMsgs   int
	pushAttempts  int
	pushDrops     int
	pushDroppedMs int

	// 统计
	stUpReqLoss   int
	stUpRespLoss  int
	stStoreFail   int
	stDownLoss    int
	stDownAckLoss int
	stPullPage    int
	stCursorFail  int
	stPullDup     int
	stDedupeHit   int
	stNewStored   int
	rawDeliveries int
	dupByRetx     int
	dupByPull     int
}

func (s *sim) schedule(delta int64, fn func()) {
	s.order++
	heap.Push(&s.h, event{at: s.now + delta, order: s.order, fn: fn})
}

func (s *sim) receiverOnline() bool {
	if (s.now >= offlineStart1 && s.now < offlineEnd1) || (s.now >= offlineStart2 && s.now < offlineEnd2) {
		return false
	}
	return true
}

func msgKey(conv string) string  { return "s662:{" + conv + "}:msgs" }
func seqKey(conv string) string  { return "s662:{" + conv + "}:seq" }
func dedupeKey(conv, cmid string) string { return "s662:{" + conv + "}:dedupe:" + cmid }
func inboxKey(user string) string { return "s662:inbox:" + user }

// ---------- 五段接力的实现 ----------

// 第 1 段（上行）：客户端发送 + 重试。请求丢失直接退避重发。
func (s *sim) clientSend(m *msg) {
	m.upAttempts++
	if s.rng.Float64() < pUpReqLoss { // 段①失败：请求丢了
		s.stUpReqLoss++
		s.retryUp(m)
		return
	}
	s.schedule(netMs, func() { s.serverReceive(m) })
}

func (s *sim) retryUp(m *msg) {
	if m.upAttempts >= maxUpAttempts {
		fmt.Printf("FATAL: msg %d 超过最大上行重试次数\n", m.i)
		os.Exit(1)
	}
	b := int64(retryBaseMs) << (m.upAttempts - 1)
	if b > retryCapMs {
		b = retryCapMs
	}
	s.schedule(b, func() { s.clientSend(m) })
}

// 第 2 段（持久化 + 上行幂等）：Lua 脚本原子完成「去重查询 → 分配 seq → 落库 → 写收件箱」。
// 关键：重发带同一个 client_msg_id，命中已有去重键时返回同一条 msg_id，绝不新建。
func (s *sim) serverReceive(m *msg) {
	if s.rng.Float64() < pStoreFail { // 段②失败：落库失败，NACK
		s.stStoreFail++
		s.schedule(netMs, func() { s.retryUp(m) })
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"sender": s.sender, "conv": s.conv, "cmid": m.cmid, "sent_at": m.sentAt, "body": fmt.Sprintf("消息正文 #%d", m.i),
	})
	mid, err := s.dedupe.Run(ctx, s.rdb,
		[]string{dedupeKey(s.conv, m.cmid), seqKey(s.conv), msgKey(s.conv), inboxKey(s.receiver)},
		string(payload)).Text()
	if err != nil {
		fmt.Println("redis err:", err)
		os.Exit(1)
	}
	if m.msgID == "" { // 第一次落库
		m.msgID = mid
		n, _ := strconv.ParseInt(strings.TrimPrefix(mid, "m-"), 10, 64)
		m.seq = n
		m.storedAt = s.now
		s.bySeq[n] = m
		s.stNewStored++
	} else { // 重发命中幂等：必须是同一条 msg_id，否则就是重复入库
		if m.msgID != mid {
			fmt.Printf("FATAL: 幂等失效 cmid=%s 旧=%s 新=%s\n", m.cmid, m.msgID, mid)
			os.Exit(1)
		}
		s.stDedupeHit++
	}
	// 上行 ACK：也可能在回程丢
	if s.rng.Float64() < pUpRespLoss {
		s.stUpRespLoss++
		s.retryUp(m)
		return
	}
	s.schedule(netMs, func() { s.clientGotAck(m) })
}

func (s *sim) clientGotAck(m *msg) {
	m.ackedAt = s.now
	s.schedule(netMs, func() { s.downlink(m) })
}

// 第 3 段（下行投递）+ 第 5 段（推送通道）
func (s *sim) downlink(m *msg) {
	if !s.receiverOnline() { // 第 4 段：离线，入收件箱等拉取
		m.wasOffline = true
		s.offlineMsgs++
		s.enqueuePush(m)
		return
	}
	if m.downRetx >= maxDownRetx {
		fmt.Printf("FATAL: msg %d 下行重传超限\n", m.i)
		os.Exit(1)
	}
	if s.rng.Float64() < pDownLoss { // 段③失败：推送包丢了
		s.stDownLoss++
		m.downRetx++
		s.schedule(downRetxMs, func() { s.downlink(m) })
		return
	}
	if m.deliveries > 0 {
		s.dupByRetx++ // 服务端重传：同一条消息再次落地客户端
	}
	s.deliver(m)
	if s.rng.Float64() < pDownAckLoss { // 段③失败：ACK 丢了 → 服务端重传 → 重复投递
		s.stDownAckLoss++
		m.downRetx++
		s.schedule(downRetxMs, func() { s.downlink(m) })
	}
}

func (s *sim) deliver(m *msg) {
	m.deliveries++
	s.rawDeliveries++
	if m.firstDeliv < 0 {
		m.firstDeliv = s.now
	}
	// 客户端按 msg_id/seq 去重 + 游标按「连续前缀」推进
	s.received[m.seq] = true
	for s.received[s.cursor+1] {
		s.cursor++
	}
}

// 第 5 段：推送通道。批量（500ms 一次），丢 5% 叫不醒；叫醒也只触发「拉」，不携带消息本体。
func (s *sim) enqueuePush(m *msg) {
	s.pendingPush = append(s.pendingPush, m)
	if s.pushTimerOn {
		return
	}
	s.pushTimerOn = true
	s.schedule(pushBatchMs, func() { s.pushBatch() })
}

func (s *sim) pushBatch() {
	batch := s.pendingPush
	s.pendingPush = nil
	s.pushTimerOn = false
	if len(batch) == 0 {
		return
	}
	s.pushAttempts++
	if s.rng.Float64() < pPushDrop {
		s.pushDrops++
		s.pushDroppedMs += len(batch)
		return // 叫不醒：消息仍在收件箱，等下一次自然打开 App
	}
	// 叫醒成功：设备 3s 后打开 App 拉一次
	s.schedule(wakePullDelayMs, func() { s.startPull("push-wake") })
}

// 第 4 段：离线收件箱分页拉取（seq 游标分页）
func (s *sim) startPull(kind string) {
	if s.pulling {
		s.pullQueued = true
		return
	}
	s.pulling = true
	s.pullKind = kind
	s.pullPage()
}

func (s *sim) pullPage() {
	if s.rng.Float64() < pPullPage { // 拉取请求失败：保持游标重试
		s.stPullPage++
		s.schedule(200, func() { s.pullPage() })
		return
	}
	min := "(" + strconv.FormatInt(s.cursor, 10)
	if s.cursor == 0 {
		min = "-inf"
	}
	members, err := s.rdb.ZRangeByScore(ctx, inboxKey(s.receiver), &redis.ZRangeBy{Min: min, Max: "+inf", Count: pullPageSize}).Result()
	if err != nil {
		fmt.Println("zrange err:", err)
		os.Exit(1)
	}
	s.pullPages++
	for _, mem := range members {
		seq, _ := strconv.ParseInt(strings.TrimPrefix(mem, "m-"), 10, 64)
		if mm, ok := s.bySeq[seq]; ok {
			if s.received[seq] {
				s.stPullDup++
				s.dupByPull++
			}
			if mm.firstDeliv < 0 {
				mm.viaPull = true
				mm.firstDeliv = s.now
			}
			mm.deliveries++
			s.rawDeliveries++
			s.received[seq] = true
		}
	}
	if len(members) > 0 {
		s.pullMsgs += len(members)
		last, _ := strconv.ParseInt(strings.TrimPrefix(members[len(members)-1], "m-"), 10, 64)
		// 游标写本地库失败 → 不推进 → 下一分页会重拉这一页（重复投递）
		if s.rng.Float64() < pCursorFail {
			s.stCursorFail++
		} else {
			for s.received[s.cursor+1] {
				s.cursor++
			}
			_ = last
		}
	}
	if len(members) == pullPageSize {
		s.schedule(5, func() { s.pullPage() })
		return
	}
	s.pulling = false
	if s.pullQueued {
		s.pullQueued = false
		s.schedule(1, func() { s.startPull("queued") })
	}
}

// ---------- 主流程 ----------

type data struct {
	i    int
	cmid string
}

func pct(v []int64, p float64) int64 {
	if len(v) == 0 {
		return 0
	}
	sort.Slice(v, func(a, b int) bool { return v[a] < v[b] })
	idx := int(float64(len(v)-1) * p)
	return v[idx]
}

func fenwickInversions(a []int64) int64 {
	// 坐标压缩
	sorted := append([]int64(nil), a...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := map[int64]int{}
	for i, v := range sorted {
		if _, ok := rank[v]; !ok {
			rank[v] = i + 1
		}
	}
	tree := make([]int, len(sorted)+2)
	var sum func(i int) int
	sum = func(i int) int {
		s := 0
		for ; i > 0; i -= i & -i {
			s += tree[i]
		}
		return s
	}
	var inv int64
	for i := len(a) - 1; i >= 0; i-- {
		r := rank[a[i]]
		inv += int64(sum(r - 1))
		for j := r; j < len(tree); j += j & -j {
			tree[j]++
		}
	}
	return inv
}

func main() {
	t0 := time.Now()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16382"})
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Println("redis 不可用:", err)
		os.Exit(1)
	}
	// 清掉本实验的键，保证可重复
	for _, pat := range []string{"s662:{c66}:*", "s662:inbox:*"} {
		keys, _ := rdb.Keys(ctx, pat).Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
	}
	s := &sim{
		rng:      rand.New(rand.NewSource(20261121)),
		rdb:      rdb,
		conv:     "c66",
		sender:   "u1001",
		receiver: "u1002",
		received: map[int64]bool{},
		bySeq:    map[int64]*msg{},
		dedupe: redis.NewScript(`local cur = redis.call('GET', KEYS[1])
if cur then return cur end
local seq = redis.call('INCR', KEYS[2])
local mid = 'm-' .. seq
redis.call('HSET', KEYS[3], seq, ARGV[1])
redis.call('ZADD', KEYS[4], seq, mid)
redis.call('SET', KEYS[1], mid, 'EX', 86400)
return mid`),
	}
	// 生成 N 条消息：cmid 在重试中复用，是上行幂等键
	for i := 0; i < N; i++ {
		m := &msg{i: i, cmid: fmt.Sprintf("cmid-%06d", i), firstDeliv: -1, sentAt: int64(i) * sendIntervalMs}
		s.msgs = append(s.msgs, m)
	}
	// 发送 + 自然重连拉取 + 最终对账
	for _, m := range s.msgs {
		mm := m
		s.schedule(int64(mm.sentAt), func() { s.clientSend(mm) })
	}
	s.schedule(offlineEnd1, func() { s.startPull("reconnect-1") })
	s.schedule(offlineEnd2, func() { s.startPull("reconnect-2") })
	s.schedule(simEnd, func() { s.startPull("final") })
	s.schedule(simEnd+30000, func() { s.report(t0) })

	// 跑事件循环
	for len(s.h) > 0 {
		e := heap.Pop(&s.h).(event)
		s.now = e.at
		e.fn()
	}
}

func (s *sim) report(t0 time.Time) {
	fmt.Println("================ 实验 A：五段接力（10000 条，5% 级故障） ================")
	delivered, missing := 0, 0
	var rawDuplicates int
	unique := map[int64]bool{}
	var latAll, latOnline, latOffline []int64
	var firstOrder []int64
	for _, m := range s.msgs {
		if m.firstDeliv < 0 {
			missing++
			continue
		}
		delivered++
		unique[m.seq] = true
		rawDuplicates += m.deliveries - 1
		l := m.firstDeliv - m.sentAt
		latAll = append(latAll, l)
		if m.wasOffline {
			latOffline = append(latOffline, l)
		} else {
			latOnline = append(latOnline, l)
		}
		firstOrder = append(firstOrder, m.seq)
	}
	// 按 seq 排序后的完整性：应该正好覆盖 1..N
	seqs := make([]int64, 0, len(unique))
	for q := range unique {
		seqs = append(seqs, q)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	gaps := 0
	for i, q := range seqs {
		if q != int64(i+1) {
			gaps++
		}
	}
	invRow := fenwickInversions(firstOrder)
	sortedOrder := append([]int64(nil), firstOrder...)
	sort.Slice(sortedOrder, func(i, j int) bool { return sortedOrder[i] < sortedOrder[j] })
	invSorted := fenwickInversions(sortedOrder)

	info, _ := s.rdb.Info(ctx, "memory").Result()
	usedMem := ""
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "used_memory:") {
			usedMem = strings.TrimSpace(strings.TrimPrefix(line, "used_memory:"))
		}
	}
	dbsize, _ := s.rdb.DBSize(ctx).Result()
	hkeys, _ := s.rdb.Keys(ctx, "s662:{c66}:dedupe:*").Result()
	msgsLen, _ := s.rdb.HLen(ctx, msgKey(s.conv)).Result()
	zcard, _ := s.rdb.ZCard(ctx, inboxKey(s.receiver)).Result()
	memMsgs, _ := s.rdb.MemoryUsage(ctx, msgKey(s.conv)).Result()
	memInbox, _ := s.rdb.MemoryUsage(ctx, inboxKey(s.receiver)).Result()
	memDedupe, _ := s.rdb.MemoryUsage(ctx, dedupeKey(s.conv, "cmid-000000")).Result()

	fmt.Printf("在途消息总数=%d  送达(至少一次)=%d  未送达=%d\n", N, delivered, missing)
	fmt.Printf("原始投递次数=%d  重复投递=%d（下行重传=%d + 拉取重投=%d）  去重后唯一消息=%d\n", s.rawDeliveries, rawDuplicates, s.dupByRetx, s.dupByPull, len(unique))
	fmt.Printf("会话 seq 覆盖=1..%d 缺号数=%d  幂等键命中(重发)=%d  新建落库=%d\n", len(seqs), gaps, s.stDedupeHit, s.stNewStored)
	fmt.Printf("首送达顺序倒置对数(原始乱序)=%d  按 seq 重排后倒置对数=%d\n", invRow, invSorted)
	fmt.Printf("P50 端到端延迟=%dms  P99=%dms  P999=%dms  max=%dms\n",
		pct(latAll, 0.50), pct(latAll, 0.99), pct(latAll, 0.999), func() int64 { if len(latAll) == 0 { return 0 }; sort.Slice(latAll, func(a, b int) bool { return latAll[a] < latAll[b] }); return latAll[len(latAll)-1] }())
	fmt.Printf("在线消息 P50=%dms P99=%dms (%d 条)  离线消息 P50=%dms P99=%dms (%d 条)\n",
		pct(latOnline, 0.5), pct(latOnline, 0.99), len(latOnline), pct(latOffline, 0.5), pct(latOffline, 0.99), len(latOffline))
	if len(latOffline) > 0 {
		buckets := []int64{100, 500, 1000, 2000, 5000, 15000, 1 << 62}
		names := []string{"<100ms", "100-500ms", "0.5-1s", "1-2s", "2-5s", "5-15s", ">15s"}
		hist := make([]int, len(buckets))
		for _, v := range latOffline {
			for i, b := range buckets {
				if v < b {
					hist[i]++
					break
				}
			}
		}
		fmt.Print("离线消息首送达延迟分布：")
		for i := range buckets {
			if hist[i] > 0 {
				fmt.Printf("%s=%d ", names[i], hist[i])
			}
		}
		fmt.Println()
	}
	fmt.Printf("故障：上行请求丢=%d 上行ACK丢=%d 落库失败=%d 下行丢包=%d 下行ACK丢=%d 拉取页失败=%d 游标写失败=%d 拉取重复=%d\n",
		s.stUpReqLoss, s.stUpRespLoss, s.stStoreFail, s.stDownLoss, s.stDownAckLoss, s.stPullPage, s.stCursorFail, s.stPullDup)
	fmt.Printf("离线收件箱：离线期间入库=%d  推送批次尝试=%d 推送叫不醒=%d 未叫醒消息=%d\n",
		s.offlineMsgs, s.pushAttempts, s.pushDrops, s.pushDroppedMs)
	fmt.Printf("拉取：分页次数=%d 拉到消息条次=%d 最终游标=%d\n", s.pullPages, s.pullMsgs, s.cursor)
	fmt.Printf("Redis：DBSIZE=%d  去重键=%d  msgs哈希字段=%d  收件箱ZSET=%d\n", dbsize, len(hkeys), msgsLen, zcard)
	fmt.Printf("Redis 内存：used_memory=%s bytes  msgs哈希=%d bytes  收件箱=%d bytes  单个去重键=%d bytes\n",
		usedMem, memMsgs, memInbox, memDedupe)
	fmt.Printf("模拟事件数=%d  真实耗时=%v\n", s.order, time.Since(t0).Round(time.Millisecond))
	fmt.Println("---- 断言 ----")
	ok := func(cond bool, name string) {
		st := "PASS"
		if !cond {
			st = "FAIL"
		}
		fmt.Printf("ASSERT %-42s %s\n", name, st)
	}
	ok(delivered == N, "最终每条消息至少送达一次")
	ok(len(unique) == N && rawDuplicates > 0, "去重后恰好一次(原始投递存在重复)")
	ok(gaps == 0, "会话 seq 无缺号(按 seq 重排后连续)")
	ok(s.stNewStored == N, "服务端只落库 N 条(重发不新增)")
	ok(invSorted == 0, "按 seq 重排后乱序为 0")
}
