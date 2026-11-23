package main

// hot.go: 实验 C —— 热点群的影响。
//   C1 读热点：200 个并发读者读"最后 50 条"——全部打 1 个群时间线（10 万条）vs 分散到 200 个群时间线（各 1000 条）。
//   C2 混部：1 个 1 万人群全在线写扩散，与 100 个百人群的扇出同时跑，对比该热点群单独跑时的延迟分布。
import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func hot(concurrency int) {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)
	payload := strings.Repeat("x", 256)

	// ---------- C1 读热点 ----------
	// 冷：200 个时间线，各 1000 条；热：1 个时间线，20 万条（总量对齐到 20 万条）
	pe := newPipeExec(c, ctx, 4000)
	for g := 0; g < 200; g++ {
		key := fmt.Sprintf("tl:cold%03d", g)
		for i := 0; i < 1000; i++ {
			pe.add(func(pl redis.Pipeliner) { pl.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: payload}) })
		}
	}
	hotKey := "tl:hot"
	for i := 0; i < 200000; i++ {
		i := i
		pe.add(func(pl redis.Pipeliner) {
			pl.ZAdd(ctx, hotKey, redis.Z{Score: float64(i), Member: payload})
		})
	}
	pe.finish()

	readRun := func(hotMode bool) []time.Duration {
		lat := make([]time.Duration, 0, concurrency)
		var mu sync.Mutex
		var wg sync.WaitGroup
		start := make(chan struct{})
		for k := 0; k < concurrency; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				cl := newClient()
				<-start
				t0 := time.Now()
				if hotMode {
					cl.ZRangeByScore(ctx, hotKey, &redis.ZRangeBy{Min: "199000", Max: "+inf", Offset: 0, Count: 50}).Result()
				} else {
					off := rand.Intn(950)
					cl.ZRangeByScore(ctx, fmt.Sprintf("tl:cold%03d", k%200), &redis.ZRangeBy{Min: fmt.Sprintf("%d", off), Max: "+inf", Offset: 0, Count: 50}).Result()
				}
				d := time.Since(t0)
				mu.Lock()
				lat = append(lat, d)
				mu.Unlock()
			}(k)
		}
		close(start)
		wg.Wait()
		return lat
	}
	// 预热
	readRun(true)
	readRun(false)
	latHot := readRun(true)
	latCold := readRun(false)
	fmt.Printf("C1 read_hotspot concurrency=%d\n", concurrency)
	fmt.Printf("  hot  key(1 x 200k entries): p50=%v p99=%v max=%v\n",
		pct(latHot, 0.5), pct(latHot, 0.99), maxDur(latHot))
	fmt.Printf("  cold keys(200 x 1k entries): p50=%v p99=%v max=%v\n",
		pct(latCold, 0.5), pct(latCold, 0.99), maxDur(latCold))

	// ---------- C2 热点群写扩散混部 ----------
	runGroup := func(gid string, n, m int, lat *[]time.Duration, mu *sync.Mutex) {
		cl := newClient()
		payload := strings.Repeat("x", 256)
		pe := newPipeExec(cl, ctx, 4000)
		members := make([]string, n)
		for i := range members {
			members[i] = fmt.Sprintf("u%07d", i)
		}
		for i := 1; i <= m; i++ {
			t0 := time.Now()
			for _, u := range members {
				u := u
				pe.add(func(pl redis.Pipeliner) { pl.ZAdd(ctx, "in:"+gid+":"+u, redis.Z{Score: float64(i), Member: payload}) })
			}
			// 只测收件箱写：在线推送与收件箱写是两个独立资源（网关出口 vs Redis），这里隔离出写竞争
			pe.finish()
			mu.Lock()
			*lat = append(*lat, time.Since(t0))
			mu.Unlock()
		}
	}

	// 基线：热点群（1 万人、全在线）单独跑 100 条
	c.FlushAll(ctx)
	var mu sync.Mutex
	var base []time.Duration
	runGroup("hot", 10000, 100, &base, &mu)

	// 混部：热点群 100 条 + 100 个百人群各 100 条同时跑
	c.FlushAll(ctx)
	var hotMix []time.Duration
	var smallLat []time.Duration
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mux := &sync.Mutex{}
		runGroup("hot", 10000, 100, &hotMix, mux)
	}()
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			mux := &sync.Mutex{}
			runGroup(fmt.Sprintf("g%03d", g), 100, 100, &smallLat, mux)
		}(g)
	}
	wg.Wait()
	fmt.Printf("C2 hot_mixed_baseline hotgroup(1w members,all online,100 msgs)\n")
	fmt.Printf("  alone: p50=%v p99=%v max=%v\n", pct(base, 0.5), pct(base, 0.99), maxDur(base))
	fmt.Printf("  mixed with 100 small groups: p50=%v p99=%v max=%v\n", pct(hotMix, 0.5), pct(hotMix, 0.99), maxDur(hotMix))
	fmt.Printf("  small groups under mix: p50=%v p99=%v max=%v (n=%d samples)\n",
		pct(smallLat, 0.5), pct(smallLat, 0.99), maxDur(smallLat), len(smallLat))
	c.FlushAll(ctx)
}

func maxDur(ds []time.Duration) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}
