package orderid

import (
	"sync"
	"time"
)

const (
	epoch    int64 = 1700000000000 // 2023-11-15 00:00:00 UTC
	nodeBits       = 10
	seqBits        = 12
	seqMask  int64 = 1<<seqBits - 1
	nodeMask int64 = 1<<nodeBits - 1
)

// ---- v0：能用版：全局状态 + 真实时钟 + 无锁 ----
var (
	v0seq  int64
	v0node int64 = 1
)

// NextV0 返回一个"看起来像"雪花号的 ID。
func NextV0() int64 {
	ms := time.Now().UnixMilli() - epoch
	v0seq = (v0seq + 1) & seqMask // 数据竞争：多 goroutine 同时写
	return ms<<(nodeBits+seqBits) | v0node<<seqBits | v0seq
}

// ---- v1：加锁 + 可注入时钟 ----
type Gen struct {
	mu   sync.Mutex
	now  func() time.Time
	node int64
	last int64
	seq  int64
}

// New 用真实时钟构造；node 由调用方传入，不再写死。
func New(node int64) *Gen { return NewWithClock(node, time.Now) }

// NewWithClock 允许测试注入假时钟。
func NewWithClock(node int64, now func() time.Time) *Gen {
	return &Gen{now: now, node: node & nodeMask}
}

func (g *Gen) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := g.now().UnixMilli() - epoch
	if ms <= g.last { // 同一毫秒，或时钟回拨：沿用逻辑毫秒
		g.seq++
		if g.seq > seqMask { // 本毫秒用尽：向未来借 1 ms
			g.last++
			g.seq = 0
		}
		ms = g.last
	} else {
		g.seq = 0
		g.last = ms
	}
	return ms<<(nodeBits+seqBits) | g.node<<seqBits | g.seq
}

// ---- v2：把时钟改成接口（演示"接口 vs 函数值"的差别）----
type Clock interface{ Now() time.Time }

type clockFunc func() time.Time

func (f clockFunc) Now() time.Time { return f() }

type GenIface struct {
	mu   sync.Mutex
	clk  Clock
	node int64
	last int64
	seq  int64
}

func NewIface(node int64, clk Clock) *GenIface {
	return &GenIface{clk: clk, node: node & nodeMask}
}

func (g *GenIface) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := g.clk.Now().UnixMilli() - epoch
	if ms <= g.last {
		g.seq++
		if g.seq > seqMask {
			g.last++
			g.seq = 0
		}
		ms = g.last
	} else {
		g.seq = 0
		g.last = ms
	}
	return ms<<(nodeBits+seqBits) | g.node<<seqBits | g.seq
}
