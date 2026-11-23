package main

import (
	"fmt"
	"math"
	"sort"
	"time"
)

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	c := append([]time.Duration(nil), d...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	idx := int(math.Ceil(p/100*float64(len(c)))) - 1
	if idx < 0 {
		idx = 0
	}
	return c[idx]
}

func stats(orig, dec []Pt) (maxErr, meanErr float64) {
	var sum float64
	for i := range orig {
		e := haversine(orig[i].Lng, orig[i].Lat, dec[i].Lng, dec[i].Lat)
		if e > maxErr {
			maxErr = e
		}
		sum += e
	}
	return maxErr, sum / float64(len(orig))
}

func expA() {
	const n = 1_000_000
	pts := genTrajectory(n, 42, baseLng, baseLat, 6000, 3)
	fmt.Printf("A points=%d seed=42 box=6km noise_sigma=3m sample=1-5s\n", n)

	type row struct {
		name   string
		enc    func([]Pt) []byte
		dec    func([]byte) []Pt
		note   string
		withTS bool
	}
	rows := []row{
		{"① CSV 文本 ts,lng,lat(7 位)", encCSV, decCSV, "文本可读，索引差", true},
		{"② 二进制 int64 ts + float64 lng/lat", encBin, decBin, "无压缩", true},
		{"③ delta-ts varint + zigzag-delta 坐标 1e-6", func(p []Pt) []byte { return encDelta(p, 1e6) }, func(b []byte) []Pt { return decDelta(b, 1e6) }, "生产候选", true},
		{"④ 同 ③ 但量化 1e-5", func(p []Pt) []byte { return encDelta(p, 1e5) }, func(b []byte) []Pt { return decDelta(b, 1e5) }, "体积略小误差略大", true},
		{"⑤ Google Polyline 1e-5（坐标，不含 ts）", encPolyline, decPolyline, "丢时间戳，需另存", false},
	}
	fmt.Printf("%-46s %14s %10s %12s %12s %12s %12s\n", "编码", "总字节", "字节/点", "最大误差m", "平均误差m", "编码ms", "解码ms")
	for _, r := range rows {
		t0 := time.Now()
		b := r.enc(pts)
		encMs := float64(time.Since(t0).Microseconds()) / 1000
		t0 = time.Now()
		dec := r.dec(b)
		decMs := float64(time.Since(t0).Microseconds()) / 1000
		if !r.withTS { // 补时间戳：delta varint
			extra := 0
			prev := int64(0)
			for i, p := range pts {
				if i == 0 {
					extra += uvarintLen(uint64(p.TS))
				} else {
					extra += uvarintLen(uint64(p.TS - prev))
				}
				prev = p.TS
			}
			fmt.Printf("%-46s %14d %10.2f %12s %12s %12.2f %12.2f  (坐标 %d + ts %d = %.2f B/点)\n",
				r.name, len(b)+extra, float64(len(b)+extra)/n, "—", "—", encMs, decMs, len(b), extra, float64(len(b)+extra)/n)
			continue
		}
		maxE, meanE := stats(pts, dec)
		fmt.Printf("%-46s %14d %10.2f %12.2f %12.2f %12.2f %12.2f\n",
			r.name, len(b), float64(len(b))/n, maxE, meanE, encMs, decMs)
	}
	// 仅坐标 polyline 体积
	pb := encPolyline(pts)
	fmt.Printf("polyline 仅坐标: %d B, %.2f B/点\n", len(pb), float64(len(pb))/n)
}

func uvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}

// ---- 实测 B：DP 抽稀 / 在线距离阈值 ----

func dpKeep(pts []Pt, eps float64) []bool {
	keep := make([]bool, len(pts))
	keep[0] = true
	keep[len(pts)-1] = true
	type seg struct{ a, b int }
	stack := []seg{{0, len(pts) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		maxD := 0.0
		idx := -1
		for i := s.a + 1; i < s.b; i++ {
			d := perpDist(pts[i], pts[s.a], pts[s.b])
			if d > maxD {
				maxD, idx = d, i
			}
		}
		if idx >= 0 && maxD > eps {
			keep[idx] = true
			stack = append(stack, seg{s.a, idx}, seg{idx, s.b})
		}
	}
	return keep
}

func onlineKeep(pts []Pt, th float64) []bool {
	keep := make([]bool, len(pts))
	keep[0] = true
	last := 0
	for i := 1; i < len(pts); i++ {
		if haversine(pts[last].Lng, pts[last].Lat, pts[i].Lng, pts[i].Lat) >= th {
			keep[i] = true
			last = i
		}
	}
	if len(pts) > 1 {
		keep[len(pts)-1] = true
	}
	return keep
}

func keptPts(pts []Pt, keep []bool) []Pt {
	out := make([]Pt, 0, len(pts)/4)
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

func mileage(pts []Pt) float64 {
	var sum float64
	for i := 1; i < len(pts); i++ {
		sum += haversine(pts[i-1].Lng, pts[i-1].Lat, pts[i].Lng, pts[i].Lat)
	}
	return sum
}

// 逐点误差：原始点到抽稀后折线的距离；同时统计抽稀前后里程
func simplifyErr(orig, simp []Pt) (maxPerp, meanPerp, mileOrig, mileSimp float64) {
	// 建立原始点到折线段的映射（simp 是 orig 的子序列）
	j := 0
	var sum float64
	for i := 1; i < len(orig)-1; i++ {
		for j+1 < len(simp) && simp[j+1].TS <= orig[i].TS {
			j++
		}
		if j+1 >= len(simp) {
			j = len(simp) - 2
		}
		d := perpDist(orig[i], simp[j], simp[j+1])
		if d > maxPerp {
			maxPerp = d
		}
		sum += d
	}
	meanPerp = sum / float64(len(orig)-2)
	return maxPerp, meanPerp, mileage(orig), mileage(simp)
}

func expB() {
	const n = 1_000_000
	pts := genTrajectory(n, 42, baseLng, baseLat, 6000, 3)
	fmt.Printf("B points=%d（与 A 同一份合成轨迹）\n", n)
	fmt.Printf("%-10s %10s %10s %14s %14s %14s %14s %14s\n",
		"阈值m", "保留点", "保留率%", "最大垂距m", "平均垂距m", "体积B", "原始里程km", "里程误差%")
	for _, th := range []float64{2, 5, 10, 20, 50} {
		t0 := time.Now()
		keep := dpKeep(pts, th)
		dpMs := float64(time.Since(t0).Microseconds()) / 1000
		simp := keptPts(pts, keep)
		maxP, meanP, mOrig, mSimp := simplifyErr(pts, simp)
		vol := len(encDelta(simp, 1e6))
		fmt.Printf("DP %-7.0f %10d %10.2f %14.2f %14.2f %14d %14.1f %14.3f  (耗时 %.0f ms)\n",
			th, len(simp), 100*float64(len(simp))/n, maxP, meanP, vol, mOrig/1000, 100*math.Abs(mSimp-mOrig)/mOrig, dpMs)
	}
	for _, th := range []float64{2, 5, 10, 20, 50} {
		keep := onlineKeep(pts, th)
		simp := keptPts(pts, keep)
		maxP, meanP, mOrig, mSimp := simplifyErr(pts, simp)
		vol := len(encDelta(simp, 1e6))
		fmt.Printf("在线距离 %-4.0f %10d %10.2f %14.2f %14.2f %14d %14.1f %14.3f\n",
			th, len(simp), 100*float64(len(simp))/n, maxP, meanP, vol, mOrig/1000, 100*math.Abs(mSimp-mOrig)/mOrig)
	}
}

// ---- 实测 D：里程虚高 ----

func gate3(pts []Pt, R float64) float64 {
	mile := 0.0
	anchor := pts[0]
	stationary := true
	inside := 0
	var last Pt
	for i := 1; i < len(pts); i++ {
		p := pts[i]
		d := haversine(anchor.Lng, anchor.Lat, p.Lng, p.Lat)
		if stationary {
			if d > R {
				stationary = false
				anchor = p
				last = p
				inside = 0
			}
		} else {
			mile += haversine(last.Lng, last.Lat, p.Lng, p.Lat)
			last = p
			if d <= R {
				inside++
				if inside >= 60 {
					stationary = true
					anchor = p
				}
			} else {
				inside = 0
			}
		}
	}
	return mile
}

func expD() {
	const n = 3600
	fmt.Printf("D stationary=1h, 1 Hz, n=%d, true mileage=0\n", n)
	fmt.Printf("%-6s %10s %10s %10s %10s %12s %12s %12s %12s\n",
		"sigma", "每步均位移m", "朴素km", "闸1 acc>50", "闸1+2 速度", "闸3 R=20m", "闸3 R=50m", "R50/朴素%", "步>5m占比")
	for _, sigma := range []float64{2, 5, 10} {
		pts, acc := genStationary(n, int64(100+int(sigma)), sigma)
		naive := mileage(pts)
		stepSum := 0.0
		over5 := 0
		for i := 1; i < len(pts); i++ {
			d := haversine(pts[i-1].Lng, pts[i-1].Lat, pts[i].Lng, pts[i].Lat)
			stepSum += d
			if d > 5 {
				over5++
			}
		}
		keep1 := make([]Pt, 0, n)
		for i, p := range pts {
			if acc[i] <= 50 {
				keep1 = append(keep1, p)
			}
		}
		m1 := mileage(keep1)
		keep2 := make([]Pt, 0, len(keep1))
		keep2 = append(keep2, keep1[0])
		for i := 1; i < len(keep1); i++ {
			dt := float64(keep1[i].TS - keep1[i-1].TS)
			if dt <= 0 {
				continue
			}
			v := haversine(keep1[i-1].Lng, keep1[i-1].Lat, keep1[i].Lng, keep1[i].Lat) / dt
			if v <= 200.0/3.6 {
				keep2 = append(keep2, keep1[i])
			}
		}
		m2 := mileage(keep2)
		m3a := gate3(keep2, 20)
		m3b := gate3(keep2, 50)
		fmt.Printf("%-6.0f %10.2f %10.3f %10.3f %10.3f %12.3f %12.3f %12.2f %12.1f%%\n",
			sigma, stepSum/float64(n-1), naive/1000, m1/1000, m2/1000, m3a/1000, m3b/1000, 100*m3b/naive, 100*float64(over5)/float64(n-1))
	}
}
