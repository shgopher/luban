package limitfw

import (
	"context"
	"hash/fnv"
	"sync"
	"time"
)

type shard struct {
	mu    sync.Mutex
	items map[string]*entry
}

// Sharded 按 key 哈希分片，降低并发下全局锁的争用；语义与 Keyed 相同。
type Sharded struct {
	shards []*shard
	cfg    Config
	closed bool
	mu     sync.Mutex
}

func NewSharded(cfg Config, n int) (*Sharded, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if n < 1 {
		n = 1
	}
	s := &Sharded{shards: make([]*shard, n), cfg: cfg}
	for i := range s.shards {
		s.shards[i] = &shard{items: make(map[string]*entry)}
	}
	go s.gc(cfg.TTL / 2)
	return s, nil
}

func (s *Sharded) pick(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return s.shards[h.Sum32()%uint32(len(s.shards))]
}

func (s *Sharded) Allow(_ context.Context, key string) (Decision, error) {
	sh := s.pick(key)
	sh.mu.Lock()
	e := sh.items[key]
	if e == nil {
		e = &entry{b: newBucket(s.cfg.Rate, s.cfg.Burst, s.cfg.Now)}
		sh.items[key] = e
	}
	e.seen = s.cfg.Now()
	sh.mu.Unlock()
	return e.b.allow(), nil
}

func (s *Sharded) Len() int {
	n := 0
	for _, sh := range s.shards {
		sh.mu.Lock()
		n += len(sh.items)
		sh.mu.Unlock()
	}
	return n
}

func (s *Sharded) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *Sharded) gc(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		cut := s.cfg.Now().Add(-s.cfg.TTL)
		for _, sh := range s.shards {
			sh.mu.Lock()
			for k, e := range sh.items {
				if e.seen.Before(cut) {
					delete(sh.items, k)
				}
			}
			sh.mu.Unlock()
		}
	}
}
