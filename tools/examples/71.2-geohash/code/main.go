package main

import (
	"fmt"
	"math"
	"os"

	"github.com/golang/geo/s2"
	"github.com/uber/h3-go/v4"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: ch71s702 <a|b|c|d|e>")
		return
	}
	switch os.Args[1] {
	case "a":
		expA()
	case "b":
		expB()
	case "c":
		expC()
	case "d":
		expD()
	case "e":
		expE()
	case "f":
		expF()
	default:
		fmt.Println("unknown exp")
	}
}

type runStat struct {
	name                                     string
	sumCand, sumRes, sumHits, empty, queries int
	meanA, recall                            float64
	p50, p99, meanCells                      float64
}

func (s *runStat) print() {
	fmt.Printf("%-22s %8.2f %8.2f%% %9.0f %9.0f %9.1f %7d\n",
		s.name, s.meanA, s.recall*100, s.p50, s.p99, s.meanCells, s.empty)
}

// evalIndex 跑一遍查询集：A = 候选数/结果数（只统计有结果的查询，空结果单列计数）。
func evalIndex(name string, qs []Query, truths [][]int32, query func(q Query) ([]int32, int)) runStat {
	st := runStat{name: name, queries: len(qs)}
	times := make([]float64, 0, len(qs))
	const reps = 16 // macOS 的单调钟粒度约 1 µs，批量跑再平均才能到 ns 分辨率
	for i, q := range qs {
		var cands []int32
		var cells int
		t0 := nowNanos()
		for k := 0; k < reps; k++ {
			cands, cells = query(q)
		}
		times = append(times, float64(elapsedNanos(t0))/float64(reps))
		truth := truths[i]
		if len(truth) == 0 {
			st.empty++
			continue
		}
		set := make(map[int32]struct{}, len(truth))
		for _, id := range truth {
			set[id] = struct{}{}
		}
		hits := 0
		for _, id := range cands {
			if _, ok := set[id]; ok {
				hits++
			}
		}
		st.sumCand += len(cands)
		st.sumRes += len(truth)
		st.sumHits += hits
		st.meanCells += float64(cells)
	}
	st.p50 = percentile(times, 0.5)
	st.p99 = percentile(times, 0.99)
	nonEmpty := st.queries - st.empty
	if st.sumRes > 0 {
		st.meanA = float64(st.sumCand) / float64(st.sumRes)
		st.recall = float64(st.sumHits) / float64(st.sumRes)
	}
	if nonEmpty > 0 {
		st.meanCells /= float64(nonEmpty)
	}
	return st
}

func evalTableHeader(title string) {
	fmt.Printf("\n### %s\n", title)
	fmt.Printf("%-22s %8s %9s %9s %9s %9s %7s\n", "index", "A", "recall", "P50ns", "P99ns", "cells/q", "empty")
}

func meanTruth(truths [][]int32) float64 {
	tot := 0
	for _, t := range truths {
		tot += len(t)
	}
	return float64(tot) / float64(len(truths))
}

func ghPrecFor(r float64) int {
	switch {
	case r <= 150:
		return 7
	case r <= 700:
		return 6
	case r <= 3000:
		return 6
	default:
		return 5
	}
}

func s2LevelFor(r float64) int {
	lvl := s2.AvgEdgeMetric.ClosestLevel(r / earthR) // r 米 → 弧度
	if lvl < 1 {
		lvl = 1
	}
	if lvl > 20 {
		lvl = 20
	}
	return lvl
}

func s2AvgEdgeM(lvl int) float64 {
	return s2.AvgEdgeMetric.Value(lvl) * earthR
}

func reportIndex(name string, idx interface {
	Query(lat, lng, r float64) ([]int32, int)
}, qs []Query, truths [][]int32, memBytes uint64, n int) {
	st := evalIndex(name, qs, truths, qf(idx.Query))
	st.print()
	fmt.Printf("    → 索引内存 %.2f MB (%.1f B/点)\n", float64(memBytes)/1024/1024, float64(memBytes)/float64(n))
}

// ============ 实测 A ============

func expA() {
	const n = 100000
	pts := GenPoints(n, 5000, 42)
	fmt.Printf("# 实测 A：10 万合成点 × 1000 次随机半径查询（4 种索引对比）\n")
	fmt.Printf("环境: macOS arm64 / Go %s / 数据 n=%d（20 城市 Gaussian σ=0.02° + 5000 均匀噪声）\n", goVer(), n)
	fmt.Printf("距离: haversine R=6371 km；查询中心 = 随机取数据集中的点；每档半径 250 次\n")

	radii := []float64{100, 500, 1000, 5000}
	rng := newRNG(20261121)
	for _, r := range radii {
		qs := SampleQueries(pts, r, 250, rng)
		truths := BruteTruth(pts, qs)
		p := ghPrecFor(r)
		cellM := r
		s2lvl := s2LevelFor(r)
		fmt.Printf("\n--- r = %.0f m：真值平均 %.1f 个结果，空结果 %d/250；参数 geohash p=%d，grid cell=%.0f m，S2 level=%d (avg edge %.0f m)\n",
			r, meanTruth(truths), countEmpty(truths), p, cellM, s2lvl, s2AvgEdgeM(s2lvl))
		evalTableHeader(fmt.Sprintf("r=%.0f m", r))

		var gh *GeoHashIndex
		memGH := measureHeap(func() { gh = NewGeoHashIndex(pts, p, true) })
		reportIndex(fmt.Sprintf("GeoHash p=%d + ring", p), gh, qs, truths, memGH, n)

		var gi *GridIndex
		memGI := measureHeap(func() { gi = NewGridIndex(pts, cellM) })
		reportIndex(fmt.Sprintf("UniformGrid %.0fm", cellM), gi, qs, truths, memGI, n)

		var si *S2Index
		memSI := measureHeap(func() { si = NewS2Index(pts, s2lvl, 8) })
		reportIndex(fmt.Sprintf("S2 L%d maxCells=8", s2lvl), si, qs, truths, memSI, n)

		var rt *RTree
		memRT := measureHeap(func() { rt = NewSTRTree(pts) })
		reportIndex("STR-RTree M=32", rt, qs, truths, memRT, n)
	}
}

func countEmpty(truths [][]int32) int {
	c := 0
	for _, t := range truths {
		if len(t) == 0 {
			c++
		}
	}
	return c
}

// ============ 实测 B：放大率—层级曲线 ============

func expB() {
	const n = 100000
	pts := GenPoints(n, 5000, 42)
	fmt.Printf("# 实测 B：固定半径扫层级（放大率 A / P50 / P99 / 命中格数）\n")
	fmt.Printf("环境: macOS arm64 / Go %s / 数据 n=%d\n", goVer(), n)
	rng := newRNG(777)
	for _, r := range []float64{1000, 100} {
		qs := SampleQueries(pts, r, 250, rng)
		truths := BruteTruth(pts, qs)
		fmt.Printf("\n--- r = %.0f m，真值平均 %.1f 个结果，空结果 %d/250\n", r, meanTruth(truths), countEmpty(truths))
		evalTableHeader("GeoHash：p = 3..8（格子越细越多格）")
		for p := 3; p <= 8; p++ {
			idx := NewGeoHashIndex(pts, p, true)
			// 用北京纬度报告格子尺寸
			cw, ch := idx.CellSizeAt(39.9042)
			st := evalIndex(fmt.Sprintf("p=%d c=%.0fx%.0fm", p, cw, ch), qs, truths, qf(idx.Query))
			st.print()
		}
		evalTableHeader("S2：index level = 10..17，RegionCoverer maxCells=8")
		for lvl := 10; lvl <= 17; lvl++ {
			idx := NewS2Index(pts, lvl, 8)
			st := evalIndex(fmt.Sprintf("L%d edge=%.0fm", lvl, s2AvgEdgeM(lvl)), qs, truths, qf(idx.Query))
			st.print()
		}
		evalTableHeader("UniformGrid：cell = 不同边长")
		for _, c := range []float64{200, 500, 1000, 2000, 4000} {
			idx := NewGridIndex(pts, c)
			st := evalIndex(fmt.Sprintf("grid=%.0fm", c), qs, truths, qf(idx.Query))
			st.print()
		}
	}
}

// ============ 实测 C：S2 vs GeoHash，9/25 格与边界 ============

func expC() {
	const n = 100000
	pts := GenPoints(n, 5000, 42)
	fmt.Printf("# 实测 C：S2 RegionCoverer vs GeoHash 强制 9/25 格；跨 180°/极区\n")
	fmt.Printf("环境: macOS arm64 / Go %s / 数据 n=%d\n", goVer(), n)
	rng := newRNG(909)

	for _, r := range []float64{1000, 100} {
		qs := SampleQueries(pts, r, 250, rng)
		truths := BruteTruth(pts, qs)
		fmt.Printf("\n--- r = %.0f m，真值平均 %.1f 个结果\n", r, meanTruth(truths))
		fmt.Printf("%-26s %8s %9s %8s %8s %8s\n", "scheme", "A", "recall", "P50ns", "P99ns", "cells/q")
		for p := 5; p <= 8; p++ {
			idx := NewGeoHashIndex(pts, p, true)
			cw, ch := idx.CellSizeAt(39.9042)
			st9 := evalIndex("", qs, truths, func(q Query) ([]int32, int) { return idx.QueryFixedCells(q.Lat, q.Lng, 1) })
			fmt.Printf("%-26s %8.2f %8.2f%% %8.0f %8.0f %8.1f\n",
				fmt.Sprintf("gh p=%d 9cell %.0fx%.0fm", p, cw, ch), st9.meanA, st9.recall*100, st9.p50, st9.p99, st9.meanCells)
			st25 := evalIndex("", qs, truths, func(q Query) ([]int32, int) { return idx.QueryFixedCells(q.Lat, q.Lng, 2) })
			fmt.Printf("%-26s %8.2f %8.2f%% %8.0f %8.0f %8.1f\n",
				fmt.Sprintf("gh p=%d 25cell", p), st25.meanA, st25.recall*100, st25.p50, st25.p99, st25.meanCells)
			sta := evalIndex("", qs, truths, qf(idx.Query))
			fmt.Printf("%-26s %8.2f %8.2f%% %8.0f %8.0f %8.1f\n",
				fmt.Sprintf("gh p=%d adaptive", p), sta.meanA, sta.recall*100, sta.p50, sta.p99, sta.meanCells)
		}
		lvl := s2LevelFor(r)
		for _, mc := range []int{4, 8, 16, 32, 64} {
			idx := NewS2Index(pts, lvl, mc)
			st := evalIndex("", qs, truths, qf(idx.Query))
			fmt.Printf("%-26s %8.2f %8.2f%% %8.0f %8.0f %8.1f\n",
				fmt.Sprintf("S2 L%d maxCells=%d", lvl, mc), st.meanA, st.recall*100, st.p50, st.p99, st.meanCells)
		}
	}

	// ---- 跨 180° 经线 ----
	fmt.Printf("\n--- 边界 1：跨 180° 经线（2000 点集中在 ±180° 两侧 4 km 内，lat≈-17°）\n")
	am := genAntimeridian(2000, 1234)
	amAll := SampleQueries(am, 1000, 300, newRNG(55))
	amAllTruth := BruteTruth(am, amAll)
	// 只统计「查询圆与 ±180° 相交」的查询：这才是 9 格语义真正踩边的情况
	var amq []Query
	var amTruth [][]int32
	for i, q := range amAll {
		dMeridian := (180.0 - math.Abs(q.Lng)) * 111320.0 * math.Cos(q.Lat*deg)
		if dMeridian < q.R {
			amq = append(amq, q)
			amTruth = append(amTruth, amAllTruth[i])
		}
	}
	fmt.Printf("2000 点密集分布在 ±180° 两侧 4 km 内；300 次查询中「圆跨经线」%d 次，真值平均 %.1f 个结果\n",
		len(amq), meanTruth(amTruth))
	fmt.Printf("%-30s %8s %9s %8s %8s\n", "scheme", "A", "recall", "P50ns", "cells/q")
	ghNoWrap := NewGeoHashIndex(am, 6, false)
	st := evalIndex("", amq, amTruth, qf(ghNoWrap.Query))
	fmt.Printf("%-30s %8.2f %8.2f%% %8.1f %8.1f\n", "GeoHash p=6 不回绕 9 格", st.meanA, st.recall*100, st.p50, st.meanCells)
	ghWrap := NewGeoHashIndex(am, 6, true)
	st = evalIndex("", amq, amTruth, qf(ghWrap.Query))
	fmt.Printf("%-30s %8.2f %8.2f%% %8.1f %8.1f\n", "GeoHash p=6 回绕", st.meanA, st.recall*100, st.p50, st.meanCells)
	si := NewS2Index(am, s2LevelFor(1000), 8)
	st = evalIndex("", amq, amTruth, qf(si.Query))
	fmt.Printf("%-30s %8.2f %8.2f%% %8.1f %8.1f\n", "S2 (face 分界自动处理)", st.meanA, st.recall*100, st.p50, st.meanCells)
	gi := NewGridIndex(am, 1000)
	st = evalIndex("", amq, amTruth, qf(gi.Query))
	fmt.Printf("%-30s %8.2f %8.2f%% %8.1f %8.1f\n", "UniformGrid 1000m（回绕）", st.meanA, st.recall*100, st.p50, st.meanCells)
	rt := NewSTRTree(am)
	st = evalIndex("", amq, amTruth, qf(rt.Query))
	fmt.Printf("%-30s %8.2f %8.2f%% %8.1f %8.1f\n", "STR-RTree", st.meanA, st.recall*100, st.p50, st.meanCells)

	// ---- 极区 ----
	fmt.Printf("\n--- 边界 2：极区（2000 点，中心 89.9°N，2 km 内；查询 r=1 km）\n")
	pol := genPolar(2000, 99)
	pq := SampleQueries(pol, 1000, 100, newRNG(66))
	pTruth := BruteTruth(pol, pq)
	fmt.Printf("真值平均 %.1f 个结果，空 %d/100\n", meanTruth(pTruth), countEmpty(pTruth))
	fmt.Printf("%-38s %8s %9s %8s %8s\n", "scheme", "A", "recall", "P50ns", "cells/q")
	ghP := NewGeoHashIndex(pol, 6, true)
	cw, ch := ghP.CellSizeAt(89.9)
	kNeed := ghP.RingFor(89.9, 1000)
	fmt.Printf("# geohash p=6 在 lat 89.9 的格子: 经向 %.2f m × 纬向 %.0f m；覆盖 r=1km 需要 k=%d → %.0f 格\n",
		cw, ch, kNeed, float64((2*kNeed+1)*(2*kNeed+1)))
	for _, kk := range []int{1, 2, 3} {
		st := evalIndex("", pq, pTruth, func(q Query) ([]int32, int) { return ghP.QueryFixedCells(q.Lat, q.Lng, kk) })
		fmt.Printf("%-38s %8.2f %8.2f%% %8.1f %8.1f\n", fmt.Sprintf("GeoHash p=6 固定 %d 格 (%d格)", kk, (2*kk+1)*(2*kk+1)), st.meanA, st.recall*100, st.p50, st.meanCells)
	}
	ghP2 := NewGeoHashIndex(pol, 3, true) // 极区把层级压粗，靠更大格子兜
	cw2, ch2 := ghP2.CellSizeAt(89.9)
	k2 := ghP2.RingFor(89.9, 1000)
	fmt.Printf("# geohash p=3 在 lat 89.9: 经向 %.2f m × 纬向 %.0f m，覆盖 1km 需 k=%d\n", cw2, ch2, k2)
	if k2 <= 20 {
		st := evalIndex("", pq, pTruth, qf(ghP2.Query))
		fmt.Printf("%-38s %8.2f %8.2f%% %8.1f %8.1f\n", fmt.Sprintf("GeoHash p=3 adaptive (k=%d)", k2), st.meanA, st.recall*100, st.p50, st.meanCells)
	}
	siP := NewS2Index(pol, s2LevelFor(1000), 8)
	st = evalIndex("", pq, pTruth, qf(siP.Query))
	fmt.Printf("%-38s %8.2f %8.2f%% %8.1f %8.1f\n", "S2 L? maxCells=8", st.meanA, st.recall*100, st.p50, st.meanCells)
	rtP := NewSTRTree(pol)
	st = evalIndex("", pq, pTruth, qf(rtP.Query))
	fmt.Printf("%-38s %8.2f %8.2f%% %8.1f %8.1f\n", "STR-RTree", st.meanA, st.recall*100, st.p50, st.meanCells)
}

func genAntimeridian(n int, seed int64) []Pt {
	rng := newRNG(seed)
	const clat = -17.0 // 斐济纬度
	pts := make([]Pt, 0, n)
	for len(pts) < n {
		lat := clat + (rng.Float64()*2-1)*0.018 // ±2 km
		dLng := (rng.Float64()*2 - 1) * 0.018  // ±~1.9 km
		lng := 180.0 + dLng
		if lng >= 180.0 {
			lng -= 360.0
		}
		pts = append(pts, Pt{ID: int32(len(pts)), Lat: lat, Lng: lng})
	}
	return pts
}

func genPolar(n int, seed int64) []Pt {
	rng := newRNG(seed)
	const clat = 89.9
	cosl := math.Cos(clat * deg)
	pts := make([]Pt, 0, n)
	for len(pts) < n {
		x := (rng.Float64()*2 - 1) * 2000
		y := (rng.Float64()*2 - 1) * 2000
		if x*x+y*y > 4e6 {
			continue
		}
		lat := clat + y/111320.0
		lng := x / (111320.0 * cosl)
		pts = append(pts, Pt{ID: int32(len(pts)), Lat: lat, Lng: lng})
	}
	return pts
}

// ============ 实测 E：H3 分辨率与 k-ring ============

type H3Index struct {
	Res   int
	edgeM float64
	m     map[h3.Cell][]int32
}

func NewH3Index(pts []Pt, res int) *H3Index {
	e, _ := h3.HexagonEdgeLengthAvgM(res)
	idx := &H3Index{Res: res, edgeM: e, m: make(map[h3.Cell][]int32, len(pts)/3+16)}
	for i := range pts {
		c, err := h3.LatLngToCell(h3.LatLng{Lat: pts[i].Lat, Lng: pts[i].Lng}, res)
		if err != nil {
			continue
		}
		idx.m[c] = append(idx.m[c], pts[i].ID)
	}
	return idx
}

func (h *H3Index) QueryK(lat, lng float64, k int) ([]int32, int) {
	c, err := h3.LatLngToCell(h3.LatLng{Lat: lat, Lng: lng}, h.Res)
	if err != nil {
		return nil, 0
	}
	disk, err := h3.GridDisk(c, k)
	if err != nil {
		return nil, 0
	}
	var out []int32
	for _, cell := range disk {
		out = append(out, h.m[cell]...)
	}
	return out, len(disk)
}

func expE() {
	const n = 100000
	pts := GenPoints(n, 5000, 42)
	fmt.Printf("# 实测 E：H3（github.com/uber/h3-go/v4 v4.6.0，纯 Go 无 cgo）分辨率表 + k-ring 覆盖\n")
	fmt.Printf("环境: macOS arm64 / Go %s / 数据 n=%d\n", goVer(), n)
	fmt.Printf("\n--- H3 官方分辨率表（库内常量 HexagonEdgeLengthAvgM / HexagonAreaAvgKm2）\n")
	fmt.Printf("%4s %14s %14s %16s\n", "res", "avgEdge(m)", "avgArea(km2)", "totalCells")
	for res := 0; res <= 15; res++ {
		e, _ := h3.HexagonEdgeLengthAvgM(res)
		a, _ := h3.HexagonAreaAvgKm2(res)
		total := math.Pow(7, float64(res))*120 + 2
		fmt.Printf("%4d %14.2f %14.4f %16.0f\n", res, e, a, total)
	}

	fmt.Printf("\n--- 用 10 万点数据实测：res × 半径 → 最小 k（使召回 100%%）→ 候选放大率 A\n")
	rng := newRNG(321)
	fmt.Printf("%4s %10s %7s %8s %8s %8s %10s %8s\n", "res", "edgeM", "r", "k", "diskCells", "A", "P50ns", "recall")
	for _, res := range []int{7, 8, 9, 10, 11} {
		for _, r := range []float64{100, 500, 1000, 5000} {
			idx := NewH3Index(pts, res)
			qs := SampleQueries(pts, r, 100, rng)
			truths := BruteTruth(pts, qs)
			// 找最小 k 使召回 100%
			bestK, bestSt := 0, runStat{}
			for k := 1; k <= 12; k++ {
				st := evalIndex("", qs, truths, func(q Query) ([]int32, int) { return idx.QueryK(q.Lat, q.Lng, k) })
				if st.recall >= 0.9999 {
					bestK, bestSt = k, st
					break
				}
			}
			if bestK == 0 {
				fmt.Printf("%4d %10.0f %7.0f %8s\n", res, idx.edgeM, r, ">12")
				continue
			}
			fmt.Printf("%4d %10.0f %7.0f %8d %8.0f %8.2f %10.0f %7.2f%%\n",
				res, idx.edgeM, r, bestK, bestSt.meanCells, bestSt.meanA, bestSt.p50, bestSt.recall*100)
		}
	}
}
