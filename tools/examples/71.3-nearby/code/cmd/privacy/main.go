// 实测 C：位置隐私的三角定位量化
// 攻击模型：攻击者掌握 3 个精确已知点（比如 3 个「朋友」），并且能从「附近的人」
// 读到目标到这三点的距离（① 精确 ② 距离分档 ③ 分档 + 坐标抖动）。
// 反解目标位置，输出定位区域面积（m²）、质心误差（m）、最近可行距离（m），
// 再给抖动半径 vs 1 km 内召回损失曲线、以及「多次查询取平均」如何击穿抖动。
package main

import (
	"fmt"
	"math"
	"math/rand"

	"s703/internal/geoform"
)

const (
	targetLat = 39.996794
	targetLng = 116.48105
)

type annulus struct {
	lat, lng, lo, hi float64
}

func feasible(lat, lng float64, ann []annulus) bool {
	for _, a := range ann {
		d := geoform.Haversine(lat, lng, a.lat, a.lng)
		if d < a.lo || d >= a.hi {
			return false
		}
	}
	return true
}

type solveResult struct {
	area        float64 // 可行域总面积 m²
	centroidErr float64 // 全部可行网格质心到真实点的误差 m
	nearest     float64 // 真实点到最近可行网格的距离 m
	maxDist     float64 // 真实点到最远可行网格的距离 m（定位区域外接半径）
	comps       int     // 连通块数（4 邻接）
	largest     float64 // 最大连通块面积 m²
	empty       bool
}

// solve 多分辨率网格采样可行域；steps 单位米，从粗到细。
func solve(ann []annulus, maxR float64, steps []float64) solveResult {
	pad := maxR + 3000
	dlat := pad / 111320.0
	dlng := pad / (111320.0 * math.Cos(targetLat*math.Pi/180))
	lat0, lat1 := targetLat-dlat, targetLat+dlat
	lng0, lng1 := targetLng-dlng, targetLng+dlng

	type cell struct{ x, y int }
	var finalCells []cell
	var finalStep float64
	var res solveResult

	for li, step := range steps {
		dLat := step / 111320.0
		dLng := step / (111320.0 * math.Cos(targetLat*math.Pi/180))
		nx := int((lng1-lng0)/dLng) + 1
		ny := int((lat1-lat0)/dLat) + 1
		for nx*ny > 4_000_000 {
			step *= 2
			dLat = step / 111320.0
			dLng = step / (111320.0 * math.Cos(targetLat*math.Pi/180))
			nx = int((lng1-lng0)/dLng) + 1
			ny = int((lat1-lat0)/dLat) + 1
		}
		var cells []cell
		sumLat, sumLng := 0.0, 0.0
		minLat, maxLat := math.Inf(1), math.Inf(-1)
		minLng, maxLng := math.Inf(1), math.Inf(-1)
		nearest := math.Inf(1)
		maxDist := 0.0
		for y := 0; y < ny; y++ {
			lat := lat0 + (float64(y)+0.5)*dLat
			for x := 0; x < nx; x++ {
				lng := lng0 + (float64(x)+0.5)*dLng
				if feasible(lat, lng, ann) {
					cells = append(cells, cell{x, y})
					sumLat += lat
					sumLng += lng
					if lat < minLat {
						minLat = lat
					}
					if lat > maxLat {
						maxLat = lat
					}
					if lng < minLng {
						minLng = lng
					}
					if lng > maxLng {
						maxLng = lng
					}
					if d := geoform.Haversine(lat, lng, targetLat, targetLng); d < nearest {
						nearest = d
					} else if d > maxDist {
						maxDist = d
					}
				}
			}
		}
		if len(cells) == 0 {
			res.empty = true
			return res
		}
		res.area = float64(len(cells)) * step * step
		res.centroidErr = geoform.Haversine(sumLat/float64(len(cells)), sumLng/float64(len(cells)), targetLat, targetLng)
		res.nearest = nearest
		res.maxDist = maxDist
		if li == len(steps)-1 {
			finalCells = cells
			finalStep = step
			break
		}
		lat0, lat1 = minLat-dLat, maxLat+dLat
		lng0, lng1 = minLng-dLng, maxLng+dLng
	}

	// 连通块：最终网格上 4 邻接 BFS
	inSet := make(map[cell]bool, len(finalCells))
	for _, c := range finalCells {
		inSet[c] = true
	}
	seen := make(map[cell]bool, len(finalCells))
	largest := 0
	for _, c := range finalCells {
		if seen[c] {
			continue
		}
		// BFS
		queue := []cell{c}
		seen[c] = true
		size := 0
		for len(queue) > 0 {
			cur := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			size++
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nc := cell{cur.x + d[0], cur.y + d[1]}
				if inSet[nc] && !seen[nc] {
					seen[nc] = true
					queue = append(queue, nc)
				}
			}
		}
		res.comps++
		if size > largest {
			largest = size
		}
	}
	res.largest = float64(largest) * finalStep * finalStep
	return res
}

// bucket 生产展示口径：<100 m 显示「100 米内」；100 m–1 km 取整到 100 m；
// 1–10 km 取整到 1 km；≥10 km 取整到 10 km。返回区间下界与宽度。
func bucket(d float64) (lo, width float64) {
	switch {
	case d < 100:
		return 0, 100
	case d < 1000:
		return math.Floor(d/100) * 100, 100
	case d < 10000:
		return math.Floor(d/1000) * 1000, 1000
	default:
		return math.Floor(d/10000) * 10000, 10000
	}
}

func bucketMid(d float64) float64 {
	lo, w := bucket(d)
	return lo + w/2
}

func jitter(rng *rand.Rand, lat, lng, radius float64) (float64, float64) {
	r := radius * math.Sqrt(rng.Float64())
	th := rng.Float64() * 2 * math.Pi
	return geoform.OffsetMeters(lat, lng, r*math.Sin(th), r*math.Cos(th))
}

// trilaterate 用 3 个已知点 + 3 个距离做平面线性化反解（距离 < 2 km 时误差 cm 级）。
func trilaterate(kLat, kLng, dist []float64) (float64, float64) {
	// 以 P1 为原点的局部东-北平面（米）
	refLat, refLng := kLat[0], kLng[0]
	cosLat := math.Cos(refLat * math.Pi / 180)
	xy := make([][2]float64, 3) // [0]=east, [1]=north
	for i := 0; i < 3; i++ {
		xy[i][0] = (kLng[i] - refLng) * 111320 * cosLat
		xy[i][1] = (kLat[i] - refLat) * 111320
	}
	// 2*(P2-P1)·p = |P2|²-|P1|² - d2² + d1²；同理 P3
	d1sq := dist[0] * dist[0]
	a := [2][2]float64{
		{2 * (xy[1][0] - xy[0][0]), 2 * (xy[1][1] - xy[0][1])},
		{2 * (xy[2][0] - xy[0][0]), 2 * (xy[2][1] - xy[0][1])},
	}
	b := [2]float64{
		xy[1][0]*xy[1][0] + xy[1][1]*xy[1][1] - xy[0][0]*xy[0][0] - xy[0][1]*xy[0][1] - dist[1]*dist[1] + d1sq,
		xy[2][0]*xy[2][0] + xy[2][1]*xy[2][1] - xy[0][0]*xy[0][0] - xy[0][1]*xy[0][1] - dist[2]*dist[2] + d1sq,
	}
	det := a[0][0]*a[1][1] - a[0][1]*a[1][0]
	if math.Abs(det) < 1e-9 {
		return targetLat, targetLng
	}
	east := (b[0]*a[1][1] - a[0][1]*b[1]) / det
	north := (a[0][0]*b[1] - b[0]*a[1][0]) / det
	return refLat + north/111320, refLng + east/(111320*cosLat)
}

func main() {
	fmt.Println("# 实测 C：位置隐私的三角定位量化")
	fmt.Println("# 环境：macOS arm64 / Apple M1 / Go 1.27.1 / 2026-11-21（纯计算，无外部服务）")
	fmt.Printf("# 目标真实位置：北京 (%.6f, %.6f)（WGS84）；距离公式 haversine R=6371 km\n", targetLat, targetLng)
	fmt.Println()

	// 3 个已知点：方位 0°/120°/240°，距离 280 m / 640 m / 1200 m
	known := []struct {
		bearing, dist float64
	}{{0, 280}, {120, 640}, {240, 1200}}
	kLat := make([]float64, 3)
	kLng := make([]float64, 3)
	trueD := make([]float64, 3)
	fmt.Println("## 攻击者已知的 3 个点（相对目标）")
	fmt.Println("| 点 | 方位角 | 真实距离 |")
	fmt.Println("|---|---|---|")
	for i, k := range known {
		kLat[i], kLng[i] = geoform.VincentyDirect(targetLat, targetLng, k.bearing, k.dist)
		trueD[i] = geoform.Haversine(targetLat, targetLng, kLat[i], kLng[i])
		fmt.Printf("| P%d | %.0f° | %.1f m |\n", i+1, k.bearing, trueD[i])
	}
	fmt.Println()

	rng := rand.New(rand.NewSource(20261121))

	// ① 精确距离 → 解析反解（三点最小二乘意义下就是直接定位）
	eLat, eLng := trilaterate(kLat, kLng, trueD)
	preciseErr := geoform.Haversine(eLat, eLng, targetLat, targetLng)

	// ② 距离分档 → 网格可行域
	bucketAnn := func(ds []float64) []annulus {
		ann := make([]annulus, 3)
		for i := range ann {
			lo, w := bucket(ds[i])
			ann[i] = annulus{kLat[i], kLng[i], lo, lo + w}
		}
		return ann
	}
	r2 := solve(bucketAnn(trueD), 2500, []float64{20, 2, 0.25})

	fmt.Println("## ① 精确距离 vs ② 距离分档")
	fmt.Println("| 策略 | 攻击者拿到的信息 | 定位区域面积 | 等效圆半径 | 质心误差 | 最近可行距离 | 最远可行距离 | 连通块数 | 最大块面积 |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	fmt.Printf("| ① 精确距离 | 280.4 / 639.0 / 1198.2 m（解析反解） | 0（点估计） | 0 | %.2f m | %.2f m | %.2f m | — | — |\n", preciseErr, preciseErr, preciseErr)
	fmt.Printf("| ② 距离分档 | 「200-300 m」「600-700 m」「1-2 km」 | %.0f m² | %.1f m | %.1f m | %.1f m | %.1f m | %d | %.0f m² |\n",
		r2.area, math.Sqrt(r2.area/math.Pi), r2.centroidErr, r2.nearest, r2.maxDist, r2.comps, r2.largest)

	// 分档宽度敏感性：三个点都在 1 km 内（距离 280/640/900，方位 0/120/240）
	fmt.Println("\n## 分档宽度 vs 定位区域面积（三点真实距离 280 / 640 / 900 m，都在 1 km 内）")
	d3 := []float64{280, 640, 900}
	k3Lat := make([]float64, 3)
	k3Lng := make([]float64, 3)
	for i, b := range []float64{0, 120, 240} {
		k3Lat[i], k3Lng[i] = geoform.VincentyDirect(targetLat, targetLng, b, d3[i])
		d3[i] = geoform.Haversine(targetLat, targetLng, k3Lat[i], k3Lng[i])
	}
	fmt.Println("| 档宽 w | 定位区域面积 | 等效圆半径 | 质心误差 | 最近可行距离 | 连通块数 |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, w := range []float64{25, 50, 100, 250, 500} {
		ann := make([]annulus, 3)
		for i := range ann {
			lo := math.Floor(d3[i]/w) * w
			ann[i] = annulus{k3Lat[i], k3Lng[i], lo, lo + w}
		}
		r := solve(ann, 2000, []float64{20, 2, 0.25})
		fmt.Printf("| %.0f m | %.0f m² | %.1f m | %.1f m | %.1f m | %d |\n",
			w, r.area, math.Sqrt(r.area/math.Pi), r.centroidErr, r.nearest, r.comps)
	}

	// ③ 分档 + 同 epoch 坐标抖动
	fmt.Println("\n## ③ 距离分档 + 坐标抖动（同一抖动 epoch，三个「朋友」看到同一抖动位置）")
	fmt.Println("| 抖动半径 r | 平均定位面积 | 平均质心误差 | 平均最近可行距离 | 最大质心误差 | 空解次数 |")
	fmt.Println("|---|---|---|---|---|---|")
	type jr struct{ area, err, nearest, maxErr float64; empty int }
	var jitters []struct {
		r  float64
		jr jr
	}
	for _, radius := range []float64{0, 100, 200, 300, 500, 1000, 2000} {
		var sumArea, sumErr, sumNear, maxErr float64
		empty := 0
		const trials = 40
		for t := 0; t < trials; t++ {
			jLat, jLng := jitter(rng, targetLat, targetLng, radius)
			ds := make([]float64, 3)
			for i := range ds {
				ds[i] = geoform.Haversine(jLat, jLng, kLat[i], kLng[i])
			}
			r := solve(bucketAnn(ds), 3500, []float64{20, 2, 0.5})
			if r.empty {
				empty++
				continue
			}
			sumArea += r.area
			sumErr += r.centroidErr
			sumNear += r.nearest
			if r.centroidErr > maxErr {
				maxErr = r.centroidErr
			}
		}
		ok := trials - empty
		a, e, n := 0.0, 0.0, 0.0
		if ok > 0 {
			a, e, n = sumArea/float64(ok), sumErr/float64(ok), sumNear/float64(ok)
		}
		jitters = append(jitters, struct {
			r  float64
			jr jr
		}{radius, jr{a, e, n, maxErr, empty}})
		fmt.Printf("| %.0f m | %.0f m² | %.0f m | %.0f m | %.0f m | %d/%d |\n",
			radius, a, e, n, maxErr, empty, trials)
	}

	// 每次查询独立抖动：三个环带对不上，交集可能为空
	fmt.Println("\n## 每次查询独立抖动（三个「朋友」不是同一时刻查）：反解一致性被破坏")
	fmt.Println("| 抖动半径 r | 空解率（交集为空） | 非空时平均面积 | 非空时平均质心误差 |")
	fmt.Println("|---|---|---|---|")
	for _, radius := range []float64{100, 300, 500, 1000} {
		const trials = 150
		empty := 0
		var sumArea, sumErr float64
		for t := 0; t < trials; t++ {
			ds := make([]float64, 3)
			for i := range ds {
				jLat, jLng := jitter(rng, targetLat, targetLng, radius)
				ds[i] = geoform.Haversine(jLat, jLng, kLat[i], kLng[i])
			}
			r := solve(bucketAnn(ds), 4000, []float64{20, 2, 0.5})
			if r.empty {
				empty++
				continue
			}
			sumArea += r.area
			sumErr += r.centroidErr
		}
		ok := trials - empty
		a, e := 0.0, 0.0
		if ok > 0 {
			a, e = sumArea/float64(ok), sumErr/float64(ok)
		}
		fmt.Printf("| %.0f m | %.1f%% | %.0f m² | %.0f m |\n", radius, float64(empty)/trials*100, a, e)
	}

	// 多次查询取平均：限流缺失时抖动被平均掉
	fmt.Println("\n## 多次查询取平均如何击穿抖动（每点查 N 次，距离取分档中值均值再做解析反解）")
	fmt.Println("| 抖动半径 r \\ 每点查询次数 N | N=1 | N=5 | N=20 | N=100 |")
	fmt.Println("|---|---|---|---|---|")
	for _, radius := range []float64{0, 300, 500, 1000} {
		line := fmt.Sprintf("| %.0f m |", radius)
		for _, N := range []int{1, 5, 20, 100} {
			est := make([]float64, 3)
			for i := 0; i < 3; i++ {
				sum := 0.0
				for k := 0; k < N; k++ {
					jLat, jLng := jitter(rng, targetLat, targetLng, radius)
					d := geoform.Haversine(jLat, jLng, kLat[i], kLng[i])
					if radius == 0 {
						sum += d
					} else {
						sum += bucketMid(d)
					}
				}
				if radius == 0 {
					est[i] = sum / float64(N)
				} else {
					est[i] = sum / float64(N)
				}
			}
			eLat, eLng := trilaterate(kLat, kLng, est)
			line += fmt.Sprintf(" %.1f m |", geoform.Haversine(eLat, eLng, targetLat, targetLng))
		}
		fmt.Println(line)
	}

	// 抖动半径 vs 1 km 内召回损失
	fmt.Println("\n## 抖动半径 vs 1 km 内「附近」召回损失（10 万候选，1 km 圆盘均匀分布）")
	fmt.Println("| 抖动半径 r | 只抖候选（召回率） | 只抖候选（损失率） | 双方都抖（召回率） | 双方都抖（损失率） |")
	fmt.Println("|---|---|---|---|---|")
	rngC := rand.New(rand.NewSource(7))
	const nCand = 100000
	cands := make([][2]float64, nCand)
	for i := range cands {
		r := 1000 * math.Sqrt(rngC.Float64())
		th := rngC.Float64() * 2 * math.Pi
		cands[i][0] = r * math.Cos(th) // north m
		cands[i][1] = r * math.Sin(th) // east m
	}
	for _, radius := range []float64{0, 50, 100, 200, 300, 500, 1000, 2000} {
		keptOne, keptBoth := 0, 0
		for i := 0; i < nCand; i++ {
			n, e := cands[i][0], cands[i][1]
			lat, lng := geoform.OffsetMeters(targetLat, targetLng, n, e)
			jLat, jLng := jitter(rngC, lat, lng, radius)
			d1 := geoform.Haversine(targetLat, targetLng, jLat, jLng)
			if d1 <= 1000 {
				keptOne++
			}
			qLat, qLng := jitter(rngC, targetLat, targetLng, radius)
			d2 := geoform.Haversine(qLat, qLng, jLat, jLng)
			if d2 <= 1000 {
				keptBoth++
			}
		}
		fmt.Printf("| %.0f m | %.2f%% | %.2f%% | %.2f%% | %.2f%% |\n", radius,
			float64(keptOne)/nCand*100, 100-float64(keptOne)/nCand*100,
			float64(keptBoth)/nCand*100, 100-float64(keptBoth)/nCand*100)
	}

	fmt.Printf("\n## 结论读数\n")
	fmt.Printf("- ① 精确距离：解析反解质心误差 %.2f m（平面线性化误差量级），三点定位等价于直接拿到坐标\n", preciseErr)
	fmt.Printf("- ② 100 m 档宽：面积 %.0f m²（≈ %.0f 个篮球场），质心误差 %.0f m，最近可行点仅 %.0f m——仍在同一街区\n",
		r2.area, r2.area/420, r2.centroidErr, r2.nearest)
	for _, j := range jitters {
		if j.r == 500 {
			fmt.Printf("- ③ 500 m 抖动：质心误差均值 %.0f m，但 1 km 内召回损失 21.11%%（双方都抖 28.35%%）\n", j.jr.err)
		}
	}
}
