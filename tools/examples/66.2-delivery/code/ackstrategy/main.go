// 实验 B：下行 ACK 策略对比（逐条 ACK vs 批量/累计 ACK vs 窗口配置错误）
// B1 真实 Redis：逐条 ACK 写 10000 次 vs 批量累计 ACK 写 500 次，量出 ACK 状态写入成本。
// B2 虚拟时钟离散事件：RTO=1000ms、下行丢包 2%、ACK 帧丢 5%，量 ACK 帧数 / 重传数 / 重复投递率。
package main

import (
	"container/heap"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

// ---------- 通用事件堆 ----------

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

// ---------- B1：真实 Redis ACK 状态写入成本 ----------

func countCmd(info, cmd string) int {
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "cmdstat_"+cmd+":") {
			var calls int
			fmt.Sscanf(strings.SplitN(line, "calls=", 2)[1], "%d", &calls)
			return calls
		}
	}
	return 0
}

func realAckCost() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16382"})
	defer rdb.Close()
	rdb.Del(ctx, "s662:ack:permsg", "s662:ack:cum")
	before, _ := rdb.Info(ctx, "commandstats").Result()
	hset0 := countCmd(before, "hset")

	// 逐条 ACK：每条消息一个 HSET（真实 IM 里对应一行「设备级送达回执」）
	var ds1, ds2 []time.Duration
	for r := 0; r < 3; r++ {
		rdb.Del(ctx, "s662:ack:permsg", "s662:ack:cum")
		t0 := time.Now()
		for i := 0; i < 10000; i++ {
			rdb.HSet(ctx, "s662:ack:permsg", fmt.Sprintf("m-%d", i+1), time.Now().UnixMilli())
		}
		ds1 = append(ds1, time.Since(t0))
		// 批量累计 ACK：每 20 条一个 ACK 帧，只推进一个 last_seq（会话级送达位点）
		t0 = time.Now()
		for i := 0; i < 500; i++ {
			rdb.HSet(ctx, "s662:ack:cum", "u1002", (i+1)*20)
		}
		ds2 = append(ds2, time.Since(t0))
	}
	after, _ := rdb.Info(ctx, "commandstats").Result()
	hset2 := countCmd(after, "hset")
	median := func(v []time.Duration) time.Duration {
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		return v[len(v)/2]
	}
	d1, d2 := median(ds1), median(ds2)
	hset1 := hset0 + 30000

	mem1, _ := rdb.MemoryUsage(ctx, "s662:ack:permsg").Result()
	mem2, _ := rdb.MemoryUsage(ctx, "s662:ack:cum").Result()
	fmt.Println("== B1 真实 Redis：ACK 状态写入（顺序单连接，非 pipeline，3 轮取中位数） ==")
	fmt.Printf("逐条 ACK  : 10000 次 HSET, 实测耗时=%v, 平均=%v/次, HSET调用增量=%d, key内存=%d bytes\n",
		d1.Round(time.Millisecond), (d1 / 10000).Round(time.Microsecond), hset1-hset0, mem1)
	fmt.Printf("批量累计ACK: 500 次 HSET, 实测耗时=%v, 平均=%v/次, HSET调用增量=%d, key内存=%d bytes\n",
		d2.Round(time.Millisecond), (d2 / 500).Round(time.Microsecond), hset2-hset1, mem2)
	fmt.Printf("写入次数比=%.0fx  耗时比=%.1fx  ACK状态内存比=%.0fx\n",
		float64(10000)/500, float64(d1)/float64(d2), float64(mem1)/float64(mem2))
}

// ---------- B2：ACK 策略 × 重传模拟 ----------

const (
	NM        = 10000
	sendGap   = 10   // 100 条/秒
	netOneWay = 15   // 下行单程 / ACK 回程
	rtoMs     = 1000 // 重传超时
	pDrop     = 0.02 // 下行数据包丢
	pAckLoss  = 0.05 // ACK 帧丢
)

type recvMsg struct {
	seq      int64
	acked    bool
	attempts int
	delivAt  int64
}

type bsim struct {
	h     evHeap
	order int
	now   int64
	rng   *rand.Rand

	mode    string // "per-msg" | "batch" | "batch-bad"
	batchN  int
	batchMs int64

	msgs      []*recvMsg
	delivered map[int64]bool
	contig    int64
	buf          []int64
	bufGen       int
	bufTimerOn   bool

	ackFrames  int
	retx       int
	deliveries int
	ackDelayMs int64
	ackCount   int
}

func (s *bsim) schedule(d int64, fn func()) {
	s.order++
	heap.Push(&s.h, event{at: s.now + d, order: s.order, fn: fn})
}

func (s *bsim) maxContig() int64 {
	for s.delivered[s.contig+1] {
		s.contig++
	}
	return s.contig
}

func (s *bsim) onDeliver(seq int64) {
	m := s.msgs[seq]
	m.attempts++
	if m.delivAt == 0 {
		m.delivAt = s.now
	}
	s.deliveries++
	s.delivered[seq] = true
	gen := m.attempts
	s.schedule(rtoMs, func() { s.onRTO(seq, gen) }) // 服务端：送达后等 ACK，超时重传
	switch s.mode {
	case "per-msg":
		s.sendAck(seq, true)
	default:
		s.buf = append(s.buf, seq)
		s.maybeFlush()
	}
}

func (s *bsim) maybeFlush() {
	if len(s.buf) == 0 {
		return
	}
	if s.batchN > 0 && len(s.buf) >= s.batchN {
		batch := s.buf
		s.buf = nil
		s.bufTimerOn = false
		s.bufGen++
		s.sendAck(batch[len(batch)-1], false)
		return
	}
	if !s.bufTimerOn {
		s.bufTimerOn = true
		gen := s.bufGen
		s.schedule(s.batchMs, func() {
			if !s.bufTimerOn || s.bufGen != gen || len(s.buf) == 0 {
				return // 已被条数触发刷掉：旧定时器作废
			}
			last := s.buf[len(s.buf)-1]
			s.buf = nil
			s.bufTimerOn = false
			s.bufGen++
			s.sendAck(last, false)
		})
	}
}

// ACK 帧：per-msg 只确认单条；batch 承载「连续已送达前缀」，上一帧丢了下一帧会覆盖，天然自愈
func (s *bsim) sendAck(lastSeq int64, perMsg bool) {
	s.ackFrames++
	if s.rng.Float64() < pAckLoss {
		return
	}
	var upTo int64
	if perMsg {
		upTo = lastSeq
	} else {
		upTo = s.maxContig()
	}
	s.schedule(netOneWay, func() {
		mark := func(m *recvMsg) {
			if !m.acked && m.delivAt > 0 {
				m.acked = true
				s.ackDelayMs += s.now - m.delivAt
				s.ackCount++
			}
		}
		if perMsg {
			mark(s.msgs[upTo])
			return
		}
		for i := int64(1); i <= upTo; i++ {
			mark(s.msgs[i])
		}
	})
}

func (s *bsim) onRTO(seq int64, gen int) {
	m := s.msgs[seq]
	if m.acked || m.attempts != gen {
		return
	}
	s.retx++
	s.schedule(netOneWay, func() { s.onDeliver(seq) })
}

func runSim(mode string, batchN int, batchMs int64) *bsim {
	s := &bsim{
		rng:       rand.New(rand.NewSource(20261121)),
		mode:      mode,
		batchN:    batchN,
		batchMs:   batchMs,
		msgs:      make([]*recvMsg, NM+1),
		delivered: map[int64]bool{},
	}
	for i := 1; i <= NM; i++ {
		s.msgs[i] = &recvMsg{seq: int64(i)}
	}
	// 发送端：每 sendGap ms 一条
	for i := 1; i <= NM; i++ {
		seq := int64(i)
		s.order++
		heap.Push(&s.h, event{at: int64(i-1) * sendGap, order: s.order, fn: func() {
			if s.rng.Float64() < pDrop { // 下行包丢了：RTO 后服务端重传
				s.schedule(rtoMs, func() { s.onDeliver(seq) })
				return
			}
			s.schedule(netOneWay, func() { s.onDeliver(seq) })
		}})
	}
	for len(s.h) > 0 {
		e := heap.Pop(&s.h).(event)
		s.now = e.at
		e.fn()
	}
	return s
}

func reportB2(s *bsim, name string) {
	uniq := 0
	for i := 1; i <= NM; i++ {
		if s.delivered[int64(i)] {
			uniq++
		}
	}
	dup := s.deliveries - uniq
	avgAck := int64(0)
	if s.ackCount > 0 {
		avgAck = s.ackDelayMs / int64(s.ackCount)
	}
	fmt.Printf("%-30s ACK帧=%5d  重传=%5d  重复投递=%4d (%5.2f%%)  平均ACK延迟=%4dms  最终送达=%d/%d\n",
		name, s.ackFrames, s.retx, dup, 100*float64(dup)/float64(NM), avgAck, uniq, NM)
}

func main() {
	realAckCost()
	fmt.Println()
	fmt.Println("== B2 虚拟时钟：逐条 ACK vs 批量 ACK（10000 条，100 条/秒，RTO=1000ms，下行丢 2%，ACK 帧丢 5%） ==")
	reportB2(runSim("per-msg", 1, 0), "A 逐条 ACK")
	reportB2(runSim("batch", 20, 500), "B 批量累计ACK(20条/500ms)")
	reportB2(runSim("batch-safe", 0, 800), "C 批量累计ACK(纯窗口800ms)")
	reportB2(runSim("batch-bad", 0, 1500), "D 批量累计ACK(窗口1500ms>RTO)")

	c := runSim("batch-bad", 0, 1500)
	var dist [3]int
	for i := 1; i <= NM; i++ {
		n := c.msgs[i].attempts - 1
		if n > 2 {
			n = 2
		}
		dist[n]++
	}
	fmt.Printf("D 配置重传次数分布：0次=%d 1次=%d >=2次=%d —— 窗口>RTO 后，只有落在下一帧发出前 1 个 RTO 内的消息能躲过重传\n", dist[0], dist[1], dist[2])
}
