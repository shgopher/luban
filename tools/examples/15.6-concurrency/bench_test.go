package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- 计数器：同一份工作，三种同步方式 ----------

type CounterMutex struct {
	mu sync.Mutex
	n  int64
}

func (c *CounterMutex) Inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

type CounterAtomic struct {
	n atomic.Int64
}

func (c *CounterAtomic) Inc() { c.n.Add(1) }

// CounterChannel 用「一个 goroutine 独占计数器 + 有缓冲 channel 收指令」实现。
// 它演示的是所有权交接，不是推荐用法：所有权集中了，但每次自增都要过一遍调度。
type CounterChannel struct {
	inc  chan struct{}
	n    int64
	done chan struct{}
}

func NewCounterChannel() *CounterChannel {
	c := &CounterChannel{inc: make(chan struct{}, 1024), done: make(chan struct{})}
	go func() {
		for range c.inc {
			c.n++
		}
		close(c.done)
	}()
	return c
}

func (c *CounterChannel) Inc() { c.inc <- struct{}{} }

func (c *CounterChannel) Close() {
	close(c.inc)
	<-c.done
}

func (c *CounterChannel) Value() int64 { return c.n }

func BenchmarkCounterMutex(b *testing.B) {
	var c CounterMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkCounterAtomic(b *testing.B) {
	var c CounterAtomic
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkCounterChannel(b *testing.B) {
	c := NewCounterChannel()
	b.Cleanup(c.Close)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkMutexUncontended(b *testing.B) {
	var mu sync.Mutex
	for i := 0; i < b.N; i++ {
		mu.Lock()
		mu.Unlock()
	}
}

// ---------- 读多写少：RWMutex 到底划不划算 ----------

func BenchmarkMutexReadOnly(b *testing.B) {
	var mu sync.Mutex
	var x int
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			_ = x
			mu.Unlock()
		}
	})
}

func BenchmarkRWMutexReadOnly(b *testing.B) {
	var mu sync.RWMutex
	var x int
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			_ = x
			mu.RUnlock()
		}
	})
}

func BenchmarkMutexMixed9To1(b *testing.B) {
	var mu sync.Mutex
	m := make(map[int]int, 1024)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if i%10 == 0 {
				mu.Lock()
				m[i] = i
				mu.Unlock()
				continue
			}
			mu.Lock()
			_ = m[i]
			mu.Unlock()
		}
	})
}

func BenchmarkRWMutexMixed9To1(b *testing.B) {
	var mu sync.RWMutex
	m := make(map[int]int, 1024)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if i%10 == 0 {
				mu.Lock()
				m[i] = i
				mu.Unlock()
				continue
			}
			mu.RLock()
			_ = m[i]
			mu.RUnlock()
		}
	})
}

// ---------- sync.Map 的两个卖点：读多写少、写不相交 ----------

func BenchmarkSyncMapMixed9To1(b *testing.B) {
	var m sync.Map
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if i%10 == 0 {
				m.Store(i%1024, i)
				continue
			}
			m.Load(i % 1024)
		}
	})
}

func BenchmarkMapRWMutexMixed9To1(b *testing.B) {
	var mu sync.RWMutex
	m := make(map[int]int, 1024)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			if i%10 == 0 {
				mu.Lock()
				m[i%1024] = i
				mu.Unlock()
				continue
			}
			mu.RLock()
			_ = m[i%1024]
			mu.RUnlock()
		}
	})
}

func BenchmarkSyncMapDisjointWrites(b *testing.B) {
	var m sync.Map
	var nextID atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		// 每个 goroutine 领一个独立 id，把 key 空间切成互不相交的段
		base := nextID.Add(1) << 32
		var i int64
		for pb.Next() {
			m.Store(base+i, i)
			i++
		}
	})
}

func BenchmarkMapRWMutexDisjointWrites(b *testing.B) {
	var mu sync.RWMutex
	m := make(map[int64]int64, 1024)
	var nextID atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		base := nextID.Add(1) << 32
		var i int64
		for pb.Next() {
			mu.Lock()
			m[base+i] = i
			mu.Unlock()
			i++
		}
	})
}

// ---------- 分配：make vs sync.Pool ----------

var sink any

func BenchmarkMake4K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := make([]byte, 0, 4096)
		sink = p
	}
}

var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

func BenchmarkPool4K(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp := bufPool.Get().(*[]byte)
		*bp = (*bp)[:0]
		sink = bp
		bufPool.Put(bp)
	}
}

// ---------- channel 交接的单价 ----------

func BenchmarkChanHandoffBuffered1024(b *testing.B) {
	ch := make(chan int, 1024)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case ch <- 1:
			case <-done:
				return
			}
		}
	}()
	b.Cleanup(func() { close(done) })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-ch
	}
}

func BenchmarkChanHandoffUnbuffered(b *testing.B) {
	ch := make(chan int)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case ch <- 1:
			case <-done:
				return
			}
		}
	}()
	b.Cleanup(func() { close(done) })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		<-ch
	}
}

// ---------- context：创建、取消与 Value 查找的单价 ----------

type ctxKey int

const ctxKeyTrace ctxKey = iota

func BenchmarkContextWithCancel(b *testing.B) {
	parent := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, cancel := context.WithCancel(parent)
		cancel()
	}
}

func BenchmarkContextWithTimeout(b *testing.B) {
	parent := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, cancel := context.WithTimeout(parent, time.Minute)
		cancel()
	}
}

func BenchmarkContextValueDepth1(b *testing.B) {
	ctx := context.WithValue(context.Background(), ctxKeyTrace, "trace")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ctx.Value(ctxKeyTrace)
	}
}

func BenchmarkContextValueDepth4(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		ctx = context.WithValue(ctx, ctxKey(i), i)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ctx.Value(ctxKey(0)) // 最老的一层：要穿过 4 层
	}
}

func BenchmarkContextValueDepth16(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < 16; i++ {
		ctx = context.WithValue(ctx, ctxKey(i), i)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ctx.Value(ctxKey(0))
	}
}

// ---------- 分片计数：单原子 vs 8 分片（有/无 padding） ----------

type paddedInt64 struct {
	_ [56]byte
	n atomic.Int64
}

type ShardedCounter struct {
	shards [8]paddedInt64
}

func (c *ShardedCounter) Inc(shard int) {
	c.shards[shard&7].n.Add(1)
}

func BenchmarkCounterShardedPadded(b *testing.B) {
	var c ShardedCounter
	var next atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		id := int(next.Add(1))
		for pb.Next() {
			c.Inc(id)
		}
	})
}

type tightCounter struct {
	shards [8]atomic.Int64
}

func (c *tightCounter) Inc(shard int) {
	c.shards[shard&7].Add(1)
}

func BenchmarkCounterShardedTight(b *testing.B) {
	var c tightCounter
	var next atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		id := int(next.Add(1))
		for pb.Next() {
			c.Inc(id)
		}
	})
}
