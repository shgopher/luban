package main

import (
	"fmt"
	"sort"
)

type ad struct {
	name string
	bid  float64 // 出价：CPM 为元/千次曝光，CPC/CPA 为元/次
	pctr float64
	pcvr float64
	mode string
}

// eCPM：把三种计费方式的出价折算成同一个可比较的千次曝光价格
func (a ad) eCPM() float64 {
	switch a.mode {
	case "CPM":
		return a.bid
	case "CPC":
		return a.bid * a.pctr * 1000
	default: // CPA / oCPX
		return a.bid * a.pctr * a.pcvr * 1000
	}
}

func main() {
	ads := []ad{
		{"A 品牌 CPM", 18, 0.018, 0, "CPM"},
		{"B 效果 CPC", 1.20, 0.025, 0, "CPC"},
		{"C 转化 CPA", 28, 0.030, 0.035, "CPA"},
		{"D oCPX", 45, 0.028, 0.040, "oCPX"},
	}

	fmt.Println("=== 74.3 竞价：四种计费方式折算成 eCPM ===")
	fmt.Printf("%-14s %-5s %8s %7s %7s %10s\n", "广告", "计费", "出价", "pCTR", "pCVR", "eCPM")
	for _, a := range ads {
		fmt.Printf("%-14s %-5s %8.2f %7.3f %7.3f %10.2f\n", a.name, a.mode, a.bid, a.pctr, a.pcvr, a.eCPM())
	}

	sort.Slice(ads, func(i, j int) bool { return ads[i].eCPM() > ads[j].eCPM() })
	fmt.Println("\n=== 按 eCPM 排序（价高者得这次曝光）===")
	for i, a := range ads {
		fmt.Printf("第 %d 名 %-14s eCPM %6.2f 元\n", i+1, a.name, a.eCPM())
	}

	fmt.Println("\n=== 一价 vs 二价：赢家实际付多少 ===")
	win := ads[0]
	for _, second := range []bool{false, true} {
		charge := win.eCPM()
		if second && len(ads) > 1 {
			charge = ads[1].eCPM() + 0.01 // 二价：第二名的 eCPM 加最小加价
		}
		switch win.mode {
		case "CPM":
			fmt.Printf("二价=%v → 赢家 %s 付 %.2f 元/千次曝光（一价应 %.2f）\n",
				second, win.name, charge, win.eCPM())
		case "CPC":
			fmt.Printf("二价=%v → 赢家 %s 付 %.2f 元/次点击（一价应 %.2f）\n",
				second, win.name, charge/1000/win.pctr, win.bid)
		default:
			fmt.Printf("二价=%v → 赢家 %s 付 %.2f 元/次转化（一价应 %.2f）\n",
				second, win.name, charge/1000/win.pctr/win.pcvr, win.bid)
		}
	}

	fmt.Println("\n=== 预算平滑：日预算 10000 元、CPM 10 元（小时流量权重见代码）===")
	const budget, cpm = 10000.0, 10.0
	weights := []float64{0.2, 0.2, 0.6, 1.0, 1.4, 1.6, 1.5, 1.4, 1.3, 1.2, 1.2, 1.3, 1.4, 1.3, 1.2, 1.2, 1.3, 1.5, 1.8, 2.0, 2.2, 2.0, 1.5, 1.0}
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	for _, mode := range []string{"尽快", "按流量比例"} {
		remaining := budget
		cum := make([]float64, len(weights))
		exhaust := len(weights)
		for h, w := range weights {
			supplyCost := w / sum * 2_000_000 / 1000 * cpm // 当天总供给 200 万次曝光
			allow := remaining
			if mode == "按流量比例" {
				allow = budget * (w / sum) // 按小时流量占比分配预算
			}
			spend := supplyCost
			if spend > allow {
				spend = allow
			}
			if spend > remaining {
				spend = remaining
			}
			remaining -= spend
			cum[h] = budget - remaining
			if remaining <= 0.01 && exhaust == len(weights) {
				exhaust = h + 1
			}
		}
		fmt.Printf("%s：第 %2d 小时预算耗尽；6h 花 %5.0f 元，12h 花 %5.0f 元，24h 花 %5.0f 元\n",
			mode, exhaust, cum[5], cum[11], cum[23])
	}

	fmt.Println("\n=== 无效点击过滤对 CPC 收入的影响（6000 次点击、0.60 元/次）===")
	const clicks, cpc = 6000, 0.60
	for _, inv := range []float64{0.05, 0.10, 0.20} {
		valid := int(float64(clicks) * (1 - inv))
		fmt.Printf("无效点击 %.0f%% → 计费点击 %4d，收入 %6.0f 元（未过滤 %6.0f 元）\n",
			inv*100, valid, float64(valid)*cpc, float64(clicks)*cpc)
	}
}
