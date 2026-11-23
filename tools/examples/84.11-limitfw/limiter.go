package limitfw

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Decision 是一次限流判断的结果；RetryAfter 只在校验失败时有意义。
type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

// Limiter 是框架对外的唯一接口：按 key 判断，允许返回错误（分布式实现会失败）。
type Limiter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}

// Config 是唯一入口参数；非法配置在构造时直接拒绝，不带病运行。
type Config struct {
	Rate  float64       // 每秒补充的令牌数
	Burst float64       // 桶容量，决定允许的瞬时突发
	TTL   time.Duration // key 空闲多久后被回收
	Now   func() time.Time
}

func (c Config) validate() error {
	switch {
	case c.Rate <= 0:
		return errors.New("limit: rate must be > 0")
	case c.Burst < 1:
		return errors.New("limit: burst must be >= 1")
	case c.TTL <= 0:
		return errors.New("limit: ttl must be > 0")
	}
	return nil
}

// bucket 是令牌桶：允许突发、按固定速率补充。
type bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func newBucket(rate, burst float64, now func() time.Time) *bucket {
	return &bucket{rate: rate, burst: burst, tokens: burst, last: now(), now: now}
}

func (b *bucket) allow() Decision {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens += el * b.rate
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return Decision{Allowed: true, Remaining: int(b.tokens)}
	}
	need := (1 - b.tokens) / b.rate
	return Decision{Allowed: false, RetryAfter: time.Duration(need * float64(time.Second))}
}

// entry 记一次访问时间，供 TTL 回收使用。
type entry struct {
	b    *bucket
	seen time.Time
}

// Keyed 是单机、按 key 隔离的限流器：一个全局 map + 每个 key 一个桶。
type Keyed struct {
	mu     sync.Mutex
	items  map[string]*entry
	cfg    Config
	closed bool
}

// NewKeyed 校验配置并启动回收协程。
func NewKeyed(cfg Config) (*Keyed, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	k := &Keyed{items: make(map[string]*entry), cfg: cfg}
	go k.gc(cfg.TTL / 2)
	return k, nil
}

func (k *Keyed) gc(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		k.mu.Lock()
		if k.closed {
			k.mu.Unlock()
			return
		}
		cut := k.cfg.Now().Add(-k.cfg.TTL)
		for key, e := range k.items {
			if e.seen.Before(cut) {
				delete(k.items, key)
			}
		}
		k.mu.Unlock()
	}
}

// Close 停止回收协程；调用后 Allow 仍可工作，但不再清理空闲 key。
func (k *Keyed) Close() {
	k.mu.Lock()
	k.closed = true
	k.mu.Unlock()
}

func (k *Keyed) Allow(_ context.Context, key string) (Decision, error) {
	k.mu.Lock()
	e := k.items[key]
	if e == nil {
		e = &entry{b: newBucket(k.cfg.Rate, k.cfg.Burst, k.cfg.Now)}
		k.items[key] = e
	}
	e.seen = k.cfg.Now()
	k.mu.Unlock()
	return e.b.allow(), nil
}

// Len 返回当前活跃 key 数，用于观测与测试。
func (k *Keyed) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.items)
}

// RetryAfterSeconds 把等待时长向上取整到秒，避免客户端拿到 0 立刻重试。
func RetryAfterSeconds(d time.Duration) string {
	s := int(d / time.Second)
	if d%time.Second != 0 {
		s++
	}
	if s < 1 {
		s = 1
	}
	return strconv.Itoa(s)
}
