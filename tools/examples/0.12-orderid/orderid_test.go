package orderid

import (
	"testing"
	"time"
)

// fakeClock 每次读都前进 step，行为确定、不需要 sleep。
type fakeClock struct {
	t    time.Time
	step time.Duration
}

func (c *fakeClock) Now() time.Time { c.t = c.t.Add(c.step); return c.t }

func TestGenUnique(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(epoch + 1000), step: time.Microsecond}
	g := NewWithClock(7, clk.Now)
	const n = 20000
	var prev int64 = -1
	seen := make(map[int64]struct{}, n)
	for i := 0; i < n; i++ {
		id := g.Next()
		if id <= prev {
			t.Fatalf("not increasing at %d: %d <= %d", i, id, prev)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate at %d: %d", i, id)
		}
		seen[id] = struct{}{}
		prev = id
	}
}

func TestNodeInPayload(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(epoch + 1000), step: time.Microsecond}
	g := NewWithClock(513, clk.Now)
	if got := (g.Next() >> seqBits) & nodeMask; got != 513 {
		t.Fatalf("node bits = %d, want 513", got)
	}
}

var sink int64

func BenchmarkV0(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = NextV0()
	}
}

func BenchmarkFuncClock(b *testing.B) {
	g := NewWithClock(1, time.Now)
	for i := 0; i < b.N; i++ {
		sink = g.Next()
	}
}

func BenchmarkIfaceClock(b *testing.B) {
	g := NewIface(1, clockFunc(time.Now))
	for i := 0; i < b.N; i++ {
		sink = g.Next()
	}
}

type frozenClock struct{ t time.Time }

func (c *frozenClock) Now() time.Time { return c.t }

// 冻结时钟：真实时间一秒都不走，也能测出 4096 序列用尽后的回绕。
func TestGenFrozenRollover(t *testing.T) {
	clk := &frozenClock{t: time.UnixMilli(epoch + 1000)}
	g := NewWithClock(1, func() time.Time { return clk.t })
	var prev int64 = -1
	for i := 0; i < 10000; i++ {
		id := g.Next()
		if id <= prev {
			t.Fatalf("not increasing at %d: %d <= %d", i, id, prev)
		}
		prev = id
	}
}

// 时钟回拨：ID 仍必须单调递增。
func TestGenClockRollback(t *testing.T) {
	base := time.UnixMilli(epoch + 5000)
	step := 0
	now := func() time.Time {
		step++
		if step%2 == 0 {
			return base.Add(-time.Second) // 隔一次回拨 1 s
		}
		return base
	}
	g := NewWithClock(1, now)
	var prev int64 = -1
	for i := 0; i < 1000; i++ {
		id := g.Next()
		if id <= prev {
			t.Fatalf("not increasing at %d: %d <= %d", i, id, prev)
		}
		prev = id
	}
}
