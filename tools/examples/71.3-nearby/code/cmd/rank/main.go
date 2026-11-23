// 实测 D：位置排序抖动（30 s 刷新间隔下 top20 的重合与翻页重复/丢失）
// 场景：观察者在固定点，附近 1 000 个用户随机移动 30 s（0-10 m/s），比较
//   ① 按精确距离排序 ② 按距离分档 + uid tie-break（100 m / 500 m / 1000 m 档）
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"s703/internal/geoform"
)

const (
	viewerLat = 39.996794
	viewerLng = 116.48105
	users     = 1000
	topN      = 20
	trials    = 500
)

type item struct {
	uid  int
	lat  float64
	lng  float64
	dist float64
}

func dist(u *item) float64 { return geoform.Haversine(viewerLat, viewerLng, u.lat, u.lng) }

func rank(items []item, scheme string) []int {
	cp := make([]item, len(items))
	copy(cp, items)
	sort.SliceStable(cp, func(i, j int) bool {
		switch scheme {
		case "exact":
			if cp[i].dist != cp[j].dist {
				return cp[i].dist < cp[j].dist
			}
			return cp[i].uid < cp[j].uid
		case "bucket100":
			bi, bj := math.Floor(cp[i].dist/100), math.Floor(cp[j].dist/100)
			if bi != bj {
				return bi < bj
			}
			return cp[i].uid < cp[j].uid
		case "bucket500":
			bi, bj := math.Floor(cp[i].dist/500), math.Floor(cp[j].dist/500)
			if bi != bj {
				return bi < bj
			}
			return cp[i].uid < cp[j].uid
		default: // bucket1000
			bi, bj := math.Floor(cp[i].dist/1000), math.Floor(cp[j].dist/1000)
			if bi != bj {
				return bi < bj
			}
			return cp[i].uid < cp[j].uid
		}
	})
	out := make([]int, len(cp))
	for i := range cp {
		out[i] = cp[i].uid
	}
	return out
}

func main() {
	fmt.Println("# 实测 D：位置排序抖动（30 s 刷新）")
	fmt.Println("# 环境：macOS arm64 / Apple M1 / Go 1.27.1 / 2026-11-21（纯计算）")
	fmt.Println("# 观察者固定在北京 (39.996794, 116.48105)；1 000 个用户初始均匀分布在半径 10 km 圆盘内")
	fmt.Println("# 30 s 内每人以 0-10 m/s 随机方向直线移动（位移 0-300 m）；500 轮独立试验")
	fmt.Println()

	rng := rand.New(rand.NewSource(20261121))
	schemes := []struct {
		key, name string
	}{
		{"exact", "① 精确距离排序"},
		{"bucket100", "② 距离分档 100 m + uid"},
		{"bucket500", "③ 距离分档 500 m + uid"},
		{"bucket1000", "④ 距离分档 1000 m + uid"},
	}

	type agg struct {
		overlap    float64 // top20 集合重合条数（0-20）
		rankMoved  float64 // top20 内共同成员的平均名次位移
		identical  float64 // top20 顺序完全一致的比例
		dup        float64 // 第 2 屏（t30 的第 21-40 名）中上一屏出现过的条数
		lost       float64 // t0 第 21-40 名中，在 t30 前 40 名里彻底消失的条数
		pageShift  float64 // 第 2 屏与 t0 第 21-40 名的集合重合条数
		trueTop20  float64 // 该方案 top20 与「真实最近 20 人」的重合条数
		dist20     float64 // 该方案第 20 名的距离（m）
		trueD20    float64 // 真实第 20 近的距离（m）
	}
	stats := make(map[string]*agg)

	for _, sc := range schemes {
		stats[sc.key] = &agg{}
	}

	for t := 0; t < trials; t++ {
		users0 := make([]item, users)
		for i := range users0 {
			r := 10000 * math.Sqrt(rng.Float64())
			th := rng.Float64() * 2 * math.Pi
			lat, lng := geoform.OffsetMeters(viewerLat, viewerLng, r*math.Sin(th), r*math.Cos(th))
			users0[i] = item{uid: i, lat: lat, lng: lng}
			users0[i].dist = dist(&users0[i])
		}
		users30 := make([]item, users)
		for i := range users30 {
			users30[i] = users0[i]
			speed := rng.Float64() * 10 // m/s
			heading := rng.Float64() * 360
			moved := speed * 30
			rad := heading * math.Pi / 180
			users30[i].lat, users30[i].lng = geoform.OffsetMeters(users30[i].lat, users30[i].lng, moved*math.Cos(rad), moved*math.Sin(rad))
			users30[i].dist = dist(&users30[i])
		}
		for _, sc := range schemes {
			r0 := rank(users0, sc.key)
			r30 := rank(users30, sc.key)
			a := stats[sc.key]

			// top20 集合重合
			set := make(map[int]bool, topN)
			for _, uid := range r0[:topN] {
				set[uid] = true
			}
			inter := 0
			for _, uid := range r30[:topN] {
				if set[uid] {
					inter++
				}
			}
			a.overlap += float64(inter)

			// 共同成员的名次位移
			pos30 := make(map[int]int, topN)
			for pos, uid := range r30[:topN] {
				pos30[uid] = pos
			}
			moved, common := 0, 0
			for pos, uid := range r0[:topN] {
				if p, ok := pos30[uid]; ok {
					common++
					moved += abs(pos - p)
				}
			}
			if common > 0 {
				a.rankMoved += float64(moved) / float64(common)
			}
			identical := 1.0
			for i := 0; i < topN; i++ {
				if r0[i] != r30[i] {
					identical = 0
					break
				}
			}
			a.identical += identical

			// 翻页：第 1 屏 t0 的前 20；第 2 屏 t30 的第 21-40
			page1 := make(map[int]bool, topN)
			for _, uid := range r0[:topN] {
				page1[uid] = true
			}
			dup := 0
			for _, uid := range r30[topN : 2*topN] {
				if page1[uid] {
					dup++
				}
			}
			a.dup += float64(dup)

			// 丢失：t0 的第 21-40 名里，在 t30 的前 40 名中找不到的条数
			t30top40 := make(map[int]bool, 2*topN)
			for _, uid := range r30[:2*topN] {
				t30top40[uid] = true
			}
			lost := 0
			for _, uid := range r0[topN : 2*topN] {
				if !t30top40[uid] {
					lost++
				}
			}
			a.lost += float64(lost)

			// t0 第 21-40 名与 t30 第 21-40 名的集合重合
			set2 := make(map[int]bool, topN)
			for _, uid := range r30[topN : 2*topN] {
				set2[uid] = true
			}
			shift := 0
			for _, uid := range r0[topN : 2*topN] {
				if set2[uid] {
					shift++
				}
			}
			a.pageShift += float64(shift)

			// 排序准确性：该方案 top20 与「真实最近 20 人」的重合
			truth := make([]item, len(users0))
			copy(truth, users0)
			sort.SliceStable(truth, func(i, j int) bool { return truth[i].dist < truth[j].dist })
			trueSet := make(map[int]bool, topN)
			for _, it := range truth[:topN] {
				trueSet[it.uid] = true
			}
			hit := 0
			chosen := make(map[int]bool, topN)
			for _, uid := range r30[:topN] {
				chosen[uid] = true
				if trueSet[uid] {
					hit++
				}
			}
			a.trueTop20 += float64(hit)
			a.trueD20 += truth[topN-1].dist
			maxD := 0.0
			for _, it := range users30 {
				if chosen[it.uid] && it.dist > maxD {
					maxD = it.dist
				}
			}
			a.dist20 += maxD
		}
	}

	fmt.Println("## 连续两次刷新（t0 → t30）top20 的稳定性")
	fmt.Println("| 排序方案 | top20 重合条数（/20） | 重合率 | 共同成员平均名次位移 | top20 顺序完全不变的比例 |")
	fmt.Println("|---|---|---|---|---|")
	for _, sc := range schemes {
		a := stats[sc.key]
		fmt.Printf("| %s | %.2f | %.1f%% | %.2f 位 | %.1f%% |\n",
			sc.name, a.overlap/trials, a.overlap/trials/topN*100, a.rankMoved/trials, a.identical/trials*100)
	}

	fmt.Println("\n## 翻页（第 1 屏在 t0，第 2 屏在 t30 取第 21-40 名）")
	fmt.Println("| 排序方案 | 第 2 屏中「上一屏已出现」条数 | 重复率 | t0 第 21-40 名在 t30 前 40 名中消失条数 | 丢失率 | t0/t30 第 2 屏集合重合 |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, sc := range schemes {
		a := stats[sc.key]
		fmt.Printf("| %s | %.1f 条 | %.1f%% | %.1f 条 | %.1f%% | %.1f 条 |\n",
			sc.name, a.dup/trials, a.dup/trials/topN*100, a.lost/trials, a.lost/trials/topN*100, a.pageShift/trials)
	}

	fmt.Println("\n## 排序准确性（用「真实最近 20 人」当对照；距离分档越大越不准）")
	fmt.Println("| 排序方案 | top20 与真实最近 20 人重合（/20） | 返回列表第 20 名的距离 | 真实 20 人的最远距离 |")
	fmt.Println("|---|---|---|---|")
	for _, sc := range schemes {
		a := stats[sc.key]
		fmt.Printf("| %s | %.2f | %.0f m | %.0f m |\n", sc.name, a.trueTop20/trials, a.dist20/trials, a.trueD20/trials)
	}

	// 本地 CPU 成本：1 000 条候选的三种排序（不含 Redis/网络）
	{
		bench := make([]item, users)
		for i := range bench {
			r := 10000 * math.Sqrt(rng.Float64())
			th := rng.Float64() * 2 * math.Pi
			lat, lng := geoform.OffsetMeters(viewerLat, viewerLng, r*math.Sin(th), r*math.Cos(th))
			bench[i] = item{uid: i, lat: lat, lng: lng}
			bench[i].dist = dist(&bench[i])
		}
		fmt.Println("\n## 1 000 条候选的本地排序成本（同进程，不含网络；10 000 次平均）")
		fmt.Println("| 方案 | 单次耗时 |")
		fmt.Println("|---|---|")
		for _, key := range []string{"exact", "bucket100", "bucket500", "bucket1000"} {
			start := time.Now()
			for i := 0; i < 10000; i++ {
				rank(bench, key)
			}
			fmt.Printf("| %s | %.2f µs |\n", key, float64(time.Since(start).Microseconds())/10000)
		}
		// 生产写法：不稳定排序 sort.Slice（tie-break 全序，稳定性不靠排序算法）
		for _, key := range []string{"exact", "bucket1000"} {
			start := time.Now()
			for i := 0; i < 10000; i++ {
				cp := make([]item, len(bench))
				copy(cp, bench)
				switch key {
				case "exact":
					sort.Slice(cp, func(i, j int) bool {
						if cp[i].dist != cp[j].dist {
							return cp[i].dist < cp[j].dist
						}
						return cp[i].uid < cp[j].uid
					})
				default:
					sort.Slice(cp, func(i, j int) bool {
						bi, bj := math.Floor(cp[i].dist/1000), math.Floor(cp[j].dist/1000)
						if bi != bj {
							return bi < bj
						}
						return cp[i].uid < cp[j].uid
					})
				}
			}
			fmt.Printf("| %s（sort.Slice 不稳定排序） | %.2f µs |\n", key, float64(time.Since(start).Microseconds())/10000)
		}
		// haversine 成本
		start := time.Now()
		sum := 0.0
		for i := 0; i < 1000000; i++ {
			sum += geoform.Haversine(viewerLat, viewerLng, bench[i%users].lat, bench[i%users].lng)
		}
		fmt.Printf("\n- haversine 单次：%.3f ns（100 万次累计 %.3f s，sum=%.1f）\n", float64(time.Since(start).Nanoseconds())/1e6, time.Since(start).Seconds(), sum)
	}

	fmt.Println("\n## 读数")
	for _, sc := range schemes {
		a := stats[sc.key]
		fmt.Printf("- %s：top20 重合 %.2f/20（%.1f%%）、顺序完全不变 %.1f%%、第 2 屏重复 %.1f 条、丢失 %.1f 条\n",
			sc.name, a.overlap/trials, a.overlap/trials/topN*100, a.identical/trials*100, a.dup/trials, a.lost/trials)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
