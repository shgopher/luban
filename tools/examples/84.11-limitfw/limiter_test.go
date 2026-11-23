package limitfw

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newKeyed(t *testing.T, rate, burst float64) (*Keyed, *clock) {
	t.Helper()
	clk := &clock{t: time.Unix(1700000000, 0)}
	k, err := NewKeyed(Config{Rate: rate, Burst: burst, TTL: time.Minute, Now: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(k.Close)
	return k, clk
}

func TestBurstThenDeny(t *testing.T) {
	k, clk := newKeyed(t, 10, 3)
	for i := 0; i < 3; i++ {
		d, _ := k.Allow(context.Background(), "ip:1")
		if !d.Allowed {
			t.Fatalf("request %d denied, want allowed", i)
		}
	}
	d, _ := k.Allow(context.Background(), "ip:1")
	if d.Allowed {
		t.Fatal("4th request allowed, want denied")
	}
	if d.RetryAfter <= 0 || d.RetryAfter > 100*time.Millisecond {
		t.Fatalf("RetryAfter = %v, want (0, 100ms]", d.RetryAfter)
	}
	clk.Advance(100 * time.Millisecond)
	if d, _ := k.Allow(context.Background(), "ip:1"); !d.Allowed {
		t.Fatal("after refill still denied")
	}
}

func TestKeyIsolation(t *testing.T) {
	k, _ := newKeyed(t, 1, 1)
	if d, _ := k.Allow(context.Background(), "a"); !d.Allowed {
		t.Fatal("a should pass")
	}
	if d, _ := k.Allow(context.Background(), "b"); !d.Allowed {
		t.Fatal("b should pass: keys must not share buckets")
	}
	if d, _ := k.Allow(context.Background(), "a"); d.Allowed {
		t.Fatal("a should be limited")
	}
}

func TestConcurrentBurst(t *testing.T) {
	k, _ := newKeyed(t, 1, 10)
	var allowed int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d, _ := k.Allow(context.Background(), "hot"); d.Allowed {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("allowed = %d, want 10 (burst)", allowed)
	}
}

func TestConfigValidation(t *testing.T) {
	for _, cfg := range []Config{
		{Rate: 0, Burst: 1, TTL: time.Minute},
		{Rate: 1, Burst: 0.5, TTL: time.Minute},
		{Rate: 1, Burst: 1, TTL: 0},
	} {
		if _, err := NewKeyed(cfg); err == nil {
			t.Fatalf("cfg %+v accepted, want error", cfg)
		}
	}
}

func TestTTLReclaimsIdleKeys(t *testing.T) {
	clk := &clock{t: time.Unix(1700000000, 0)}
	k, err := NewKeyed(Config{Rate: 1, Burst: 1, TTL: 10 * time.Millisecond, Now: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	k.Allow(context.Background(), "gone")
	clk.Advance(20 * time.Millisecond)
	time.Sleep(20 * time.Millisecond) // 等回收协程跑一轮
	if n := k.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0 after TTL", n)
	}
}

type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string) (Decision, error) {
	return Decision{}, errors.New("redis down")
}

func TestMiddlewareDeny(t *testing.T) {
	k, _ := newKeyed(t, 1, 1)
	h := Middleware(k, func(r *http.Request) string { return r.RemoteAddr }, true)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("first = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 429 {
		t.Fatalf("second = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, want 1", rec.Header().Get("Retry-After"))
	}
}

func TestMiddlewareFailOpen(t *testing.T) {
	h := Middleware(brokenLimiter{}, func(*http.Request) string { return "k" }, true)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || rec.Header().Get("X-RateLimit-Degraded") != "1" {
		t.Fatalf("fail-open: code=%d degraded=%q", rec.Code, rec.Header().Get("X-RateLimit-Degraded"))
	}

	h = Middleware(brokenLimiter{}, func(*http.Request) string { return "k" }, false)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 503 {
		t.Fatalf("fail-closed: code=%d, want 503", rec.Code)
	}
}

var sink Decision

func BenchmarkAllow(b *testing.B) {
	k, err := NewKeyed(Config{Rate: 1000, Burst: 1000, TTL: time.Minute, Now: time.Now})
	if err != nil {
		b.Fatal(err)
	}
	defer k.Close()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, _ = k.Allow(ctx, "ip:10.0.0.1")
	}
}

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "ip:10.0." + strconv.Itoa(i>>8) + "." + strconv.Itoa(i&255)
	}
	return keys
}

func BenchmarkAllowParallel(b *testing.B) {
	k, err := NewKeyed(Config{Rate: 1e9, Burst: 1e9, TTL: time.Minute, Now: time.Now})
	if err != nil {
		b.Fatal(err)
	}
	defer k.Close()
	ctx := context.Background()
	keys := benchKeys(1024)
	var idx atomic.Uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sink, _ = k.Allow(ctx, keys[idx.Add(1)&1023])
		}
	})
}

func TestShardedMatchesKeyed(t *testing.T) {
	k, _ := newKeyed(t, 10, 2)
	s, err := NewSharded(Config{Rate: 10, Burst: 2, TTL: time.Minute, Now: time.Now}, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		d1, _ := k.Allow(ctx, "u1")
		d2, _ := s.Allow(ctx, "u1")
		if d1.Allowed != d2.Allowed {
			t.Fatalf("round %d: keyed=%v sharded=%v", i, d1.Allowed, d2.Allowed)
		}
	}
}

func BenchmarkShardedParallel(b *testing.B) {
	s, err := NewSharded(Config{Rate: 1e9, Burst: 1e9, TTL: time.Minute, Now: time.Now}, 8)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	keys := benchKeys(1024)
	var idx atomic.Uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sink, _ = s.Allow(ctx, keys[idx.Add(1)&1023])
		}
	})
}

func BenchmarkKeyedHotKey(b *testing.B) {
	k, err := NewKeyed(Config{Rate: 1e9, Burst: 1e9, TTL: time.Minute, Now: time.Now})
	if err != nil {
		b.Fatal(err)
	}
	defer k.Close()
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sink, _ = k.Allow(ctx, "ip:10.0.0.1")
		}
	})
}

func BenchmarkShardedHotKey(b *testing.B) {
	s, err := NewSharded(Config{Rate: 1e9, Burst: 1e9, TTL: time.Minute, Now: time.Now}, 8)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sink, _ = s.Allow(ctx, "ip:10.0.0.1")
		}
	})
}
