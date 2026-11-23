package main

import (
	"fmt"
)

// 实测 F：单候选过滤成本 f（决定「A 最小点」是否等于「生产选点」的关键参数）
func expF() {
	fmt.Printf("# 实测 F：单候选过滤成本 f（用于把 A 换算成端到端耗时）\n")
	fmt.Printf("环境: macOS arm64 / Go %s / 2026-11-21\n", goVer())
	rng := newRNG(11)
	const n = 5000000
	lat := make([]float64, n)
	lng := make([]float64, n)
	for i := 0; i < n; i++ {
		lat[i] = -60 + rng.Float64()*120
		lng[i] = -180 + rng.Float64()*360
	}
	qLat, qLng, r := 39.9, 116.4, 1000.0
	// 预热
	sink = 0
	for i := 0; i < 100000; i++ {
		if Haversine(qLat, qLng, lat[i], lng[i]) <= r {
			sink++
		}
	}
	t0 := nowNanos()
	hits := 0
	for i := 0; i < n; i++ {
		if Haversine(qLat, qLng, lat[i], lng[i]) <= r {
			hits++
		}
	}
	d := timeSince(t0)
	fmt.Printf("haversine 过滤 %d 个候选: %v → %.1f ns/候选（命中 %d）\n", n, d, float64(d.Nanoseconds())/n, hits)
	_ = sink

	// 对照：整型 id 的 map 命中判断（便宜过滤的代表）
	m := make(map[int32]struct{}, 100000)
	for i := 0; i < 100000; i++ {
		m[int32(i)] = struct{}{}
	}
	ids := make([]int32, 1000000)
	for i := range ids {
		ids[i] = rng.Int31n(200000)
	}
	t0 = nowNanos()
	c := 0
	for _, id := range ids {
		if _, ok := m[id]; ok {
			c++
		}
	}
	d = timeSince(t0)
	fmt.Printf("map[int32]struct{} 命中判断 %d 次: %v → %.1f ns/候选（命中 %d）\n", len(ids), d, float64(d.Nanoseconds())/float64(len(ids)), c)
}

var sink int
