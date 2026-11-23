package main

import (
	"fmt"
	"math"
)

func lcg(seed uint64) func() float64 {
	x := seed
	return func() float64 {
		x = x*6364136223846793005 + 1442695040888963407
		return float64(x>>11) / float64(1<<53)
	}
}

func main() {
	const users = 1_000_000
	const days = 7
	const cpm = 10.0
	const budget = 20000.0
	const targetProb = 0.30
	const baseConv = 0.005 // 自然转化率（对照组基线）
	ctr := []float64{0.030, 0.015, 0.008, 0.004, 0.002, 0.001, 0.0005}
	const adCVR = 0.08

	type result struct {
		name                string
		imp, reach          int
		clicks, conv        int
		spend               float64
		ctrPct, cpa         float64
		incConv, incPer100k float64
	}
	fmt.Println("=== 74.5 频控 1/2/3/不限：7 天、100 万用户、预算 2 万元、CPM 10 元 ===")
	fmt.Printf("%-6s %10s %10s %8s %8s %10s %8s\n", "上限", "曝光", "独立触达", "点击", "转化", "花费", "CPA")
	var res []result
	for _, capK := range []int{1, 2, 3, 0} {
		r := lcg(7405)
		expo := make([]int, users)
		ctrl := make([]bool, users)
		for u := 0; u < users; u++ {
			ctrl[u] = u%10 == 0 // 10% 用户作为对照，不投广告
		}
		var imp, reach, clicks, conv int
		spend := 0.0
		for d := 0; d < days; d++ {
			for u := 0; u < users; u++ {
				if ctrl[u] || r() >= targetProb {
					continue
				}
				limit := capK
				if capK == 0 {
					limit = days
				}
				if expo[u] >= limit {
					continue
				}
				if spend+0.01 > budget+1e-9 {
					continue // 预算耗尽，停止投放
				}
				expo[u]++
				imp++
				if expo[u] == 1 {
					reach++
				}
				spend += cpm / 1000
				j := expo[u] - 1
				if j >= len(ctr) {
					j = len(ctr) - 1
				}
				if r() < ctr[j] {
					clicks++
					if r() < adCVR {
						conv++
					}
				}
			}
		}

		// 自然转化：对照组与实验组同分布，差值即广告带来的增量
		r2 := lcg(99)
		ctrlN, ctrlConv := 0, 0.0
		treatN, treatOrganic := 0, 0.0
		for u := 0; u < users; u++ {
			if ctrl[u] {
				ctrlN++
				if r2() < baseConv {
					ctrlConv++
				}
			} else {
				treatN++
				if r2() < baseConv {
					treatOrganic++
				}
			}
		}
		treatConv := treatOrganic + float64(conv)
		incConv := treatConv/float64(treatN) - ctrlConv/float64(ctrlN)
		name := fmt.Sprintf("%d", capK)
		if capK == 0 {
			name = "不限"
		}
		cpa := spend / float64(conv)
		res = append(res, result{name, imp, reach, clicks, conv, spend,
			float64(clicks) / float64(imp) * 100, cpa, incConv, incConv * 100000})
		fmt.Printf("%-6s %10d %10d %8d %8d %10.0f %8.2f\n", name, imp, reach, clicks, conv, spend, cpa)
	}

	fmt.Println("\n=== 增量实验：10% 对照，测出广告带来的真实转化提升 ===")
	fmt.Printf("%-6s %10s %10s %10s %14s\n", "上限", "提升(pp)", "每 10 万人增量", "95% 区间下限", "结论")
	for i, rr := range res {
		capK := []int{1, 2, 3, 0}[i]
		_ = capK
		// 两组比例差的标准误：p1=实验组，p2=对照组
		p1 := (rr.incConv + 0.005)
		p2 := 0.005
		se := math.Sqrt(p1*(1-p1)/900000 + p2*(1-p2)/100000)
		lo := (rr.incConv - 1.96*se) * 100000
		verdict := "显著为正"
		if lo <= 0 {
			verdict = "不显著"
		}
		fmt.Printf("%-6s %9.3f %12.0f %14.0f %10s\n", rr.name, rr.incConv*100, rr.incPer100k, lo, verdict)
	}

	fmt.Println("\n=== 预算平滑：2 万元预算、CPM 10 元（当天总供给 300 万次曝光）===")
	weights := []float64{0.2, 0.2, 0.6, 1.0, 1.4, 1.6, 1.5, 1.4, 1.3, 1.2, 1.2, 1.3, 1.4, 1.3, 1.2, 1.2, 1.3, 1.5, 1.8, 2.0, 2.2, 2.0, 1.5, 1.0}
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	for _, mode := range []string{"尽快", "按流量比例"} {
		remaining, cum := budget, make([]float64, len(weights))
		exhaust := 0
		for h, w := range weights {
			supplyCost := w / sum * 3_000_000 / 1000 * cpm
			allow := remaining
			if mode == "按流量比例" {
				allow = budget * (w / sum) // 按小时流量占比分配预算
			}
			spend := math.Min(supplyCost, math.Min(allow, remaining))
			remaining -= spend
			cum[h] = budget - remaining
			if remaining <= 0.01 && exhaust == 0 {
				exhaust = h + 1
			}
		}
		fmt.Printf("%s：第 %2d 小时花完；6h 花 %5.0f 元，12h 花 %5.0f 元，24h 花 %5.0f 元\n",
			mode, exhaust, cum[5], cum[11], cum[23])
	}
}
