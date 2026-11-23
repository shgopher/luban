package main

import (
	"fmt"
	"math"
)

// 负载曲线：24 小时，单位 QPS（示例数据，用于演示口径，不是任何真实业务）
var curves = map[string][]float64{
	"企业 ERP":  {900, 850, 820, 800, 810, 880, 1000, 1100, 1200, 1250, 1200, 1000, 950, 1050, 1200, 1250, 1200, 1000, 900, 860, 840, 830, 820, 900},
	"电商":     {300, 260, 240, 230, 250, 350, 600, 900, 1200, 1400, 1500, 1600, 1500, 1450, 1400, 1500, 1700, 2000, 2300, 2600, 2800, 2200, 1500, 800},
	"报表/批处理": {150, 150, 150, 150, 150, 150, 150, 150, 400, 800, 1200, 1600, 1600, 1400, 1000, 600, 250, 200, 150, 150, 150, 150, 150, 150},
}

const (
	nodeCapQPS   = 100.0 // 单节点安全容量（已按 70% 水位折算）
	fixedPrice   = 800.0 // 固定容量：元/节点·月
	elasticMul   = 1.6   // 弹性单价 = 固定折算到小时的 1.6 倍
	daysPerMonth = 30.0
	monthHours   = 730.0
)

func mean(xs []float64) float64 { s := 0.0; for _, x := range xs { s += x }; return s / float64(len(xs)) }

func main() {
	elasticHour := fixedPrice / monthHours * elasticMul // 元/节点·小时

	fmt.Println("=== 一、同一负载的三种买法 ===")
	for _, name := range []string{"企业 ERP", "电商", "报表/批处理"} {
		load := curves[name]
		avg, peak := mean(load), 0.0
		for _, q := range load { if q > peak { peak = q } }
		fmt.Printf("\n%s：均值 %.0f QPS，峰值 %.0f QPS，峰均比 %.2f\n", name, avg, peak, peak/avg)
		fmt.Printf("%-24s %8s %10s %10s %10s\n", "买法", "月成本", "平均利用率", "每峰值QPS", "比按峰值省")

		aNodes := math.Ceil(peak / nodeCapQPS)
		aCost := aNodes * fixedPrice

		bestCost := math.Inf(1)
		for n := 0.0; n <= aNodes; n++ {
			cap := n * nodeCapQPS
			el := 0.0
			for _, q := range load { if q > cap { el += (q - cap) / nodeCapQPS } }
			if c := n*fixedPrice + el*elasticHour*daysPerMonth; c < bestCost { bestCost = c }
		}
		cCost := 0.0
		for _, q := range load { cCost += q / nodeCapQPS * elasticHour * daysPerMonth }

		fmt.Printf("%-24s %8.0f %9.1f%% %10.1f %9s\n", "A 按峰值固定买", aCost, avg/(aNodes*nodeCapQPS)*100, aCost/peak, "-")
		fmt.Printf("%-24s %8.0f %9.1f%% %10.1f %8.0f%%\n", "B 基线固定+弹性补峰", bestCost, avg/(bestCost/fixedPrice*nodeCapQPS)*100, bestCost/peak, (1-bestCost/aCost)*100)
		fmt.Printf("%-24s %8.0f %9.1f%% %10.1f %8.0f%%\n", "C 全部按小时弹性", cCost, avg/(cCost/fixedPrice*nodeCapQPS)*100, cCost/peak, (1-cCost/aCost)*100)
	}

	fmt.Println("\n=== 二、固定买还是弹性买：每天用几小时是分界线 ===")
	breakEvenHours := fixedPrice / (elasticHour * daysPerMonth)
	fmt.Printf("固定节点 %.0f 元/月；弹性节点 %.2f 元/小时 → 盈亏平衡：每天 %.1f 小时\n\n", fixedPrice, elasticHour, breakEvenHours)
	fmt.Printf("%-10s %-14s %-14s %s\n", "每天用量", "固定月成本", "弹性月成本", "更省")
	for _, h := range []float64{2, 4, 8, 12, 15, 16, 24} {
		fx, el := fixedPrice, h*daysPerMonth*elasticHour
		pick := "弹性"
		if fx < el { pick = "固定" }
		fmt.Printf("%-10.0f %-14.0f %-14.0f %s\n", h, fx, el, pick)
	}

	fmt.Println("\n=== 三、欠配的盈亏平衡：水位推到 90% 得省多少才不亏 ===")
	load := curves["电商"]
	peak := 0.0
	for _, q := range load { if q > peak { peak = q } }
	aCost := math.Ceil(peak/nodeCapQPS) * fixedPrice
	// 把容量配到峰值的 90%：允许峰值时段超载 10%
	n90 := math.Ceil(peak * 0.9 / nodeCapQPS)
	saving := aCost - n90*fixedPrice
	overflow := 0.0
	for _, q := range load { if q > n90*nodeCapQPS { overflow += q - n90*nodeCapQPS } }
	breakEvenLoss := saving / (overflow * daysPerMonth)
	fmt.Printf("按峰值配：%v 节点 %.0f 元/月；只配到峰值的 90%%：%v 节点 %.0f 元/月\n", int(math.Ceil(peak/nodeCapQPS)), aCost, int(n90), n90*fixedPrice)
	fmt.Printf("省下 %.0f 元/月，代价是每天 %.0f QPS·h 超载 → 盈亏平衡损失 = %.3f 元/QPS·h\n", saving, overflow, breakEvenLoss)
	fmt.Printf("%-16s %-16s %-16s %s\n", "超载损失/元/QPS·h", "损失/月", "净收益", "该不该省")
	for _, loss := range []float64{0.01, breakEvenLoss, 0.5, 3} {
		l := overflow * loss * daysPerMonth
		pick := "该省"
		if l > saving { pick = "不该省" }
		if math.Abs(l-saving) < 1 { pick = "持平" }
		fmt.Printf("%-16.3f %-16.0f %+-16.0f %s\n", loss, l, saving-l, pick)
	}

	fmt.Println("\n=== 四、把容量成本换算成可比的单位 ===")
	perQPSMonth := aCost / peak
	perMillion := perQPSMonth / (monthHours * 3600 / 1e6)
	fmt.Printf("固定：%.1f 元/峰值 QPS·月；全部弹性折算：%.1f 元/峰值 QPS·月\n", perQPSMonth, elasticHour*monthHours/nodeCapQPS)
	fmt.Printf("换算成请求：%.2f 元 / 百万请求（%.4f 元/千次）\n", perMillion, perMillion/1000)
	fmt.Printf("一个月 730 小时，若按峰值买而平均只用到 45.5%%，每承载 1 QPS 实际付 %.1f 元\n", perQPSMonth/0.455)
}
