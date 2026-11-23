package main

import "fmt"

func main() {
	const req = 100000
	const fill, ctr, cvr = 0.60, 0.020, 0.050
	imp := int(float64(req) * fill)
	clk := int(float64(imp) * ctr)
	conv := int(float64(clk) * cvr)

	fmt.Println("=== 74.1 四段数据流：10 万次广告请求 ===")
	fmt.Printf("请求 %d → 曝光 %d（填充率 %.0f%%）→ 点击 %d（CTR %.1f%%）→ 转化 %d（CVR %.1f%%）\n",
		req, imp, fill*100, clk, ctr*100, conv, cvr*100)
	fmt.Printf("事件量比 曝光:点击:转化 = 1 : %.3f : %.4f\n",
		float64(clk)/float64(imp), float64(conv)/float64(imp))

	const cpm, cpc, cpa = 12.0, 0.60, 30.0
	revCPM := float64(imp) / 1000 * cpm
	revCPC := float64(clk) * cpc
	revCPA := float64(conv) * cpa
	fmt.Println("\n=== 同一批流量的收入（三种计费口径）===")
	fmt.Printf("CPM %5.2f 元/千次曝光 → 收入 %8.0f 元，折合 eCPM %6.2f 元\n", cpm, revCPM, revCPM/float64(imp)*1000)
	fmt.Printf("CPC %5.2f 元/次点击   → 收入 %8.0f 元，折合 eCPM %6.2f 元\n", cpc, revCPC, revCPC/float64(imp)*1000)
	fmt.Printf("CPA %5.2f 元/次转化   → 收入 %8.0f 元，折合 eCPM %6.2f 元\n", cpa, revCPA, revCPA/float64(imp)*1000)

	fmt.Println("\n=== 计费风险随口径后移而放大 ===")
	fmt.Printf("转化率从 %.1f%% 跌到 %.1f%%：CPM 收入不变，CPC 收入不变，CPA 收入 %.0f → %.0f 元\n",
		cvr*100, cvr*50, revCPA, revCPA*0.5)

	type stage struct {
		name string
		ms   float64
	}
	stages := []stage{{"候选/召回", 20}, {"粗排", 10}, {"精排", 35}, {"事件落库", 2}, {"网络与渲染", 20}}
	fmt.Println("\n=== 端到端延迟预算（P99 100ms）===")
	total := 0.0
	for _, s := range stages {
		total += s.ms
		fmt.Printf("%-12s %5.1f ms\n", s.name, s.ms)
	}
	fmt.Printf("合计 %.1f ms，余量 %.1f ms\n", total, 100-total)
}
