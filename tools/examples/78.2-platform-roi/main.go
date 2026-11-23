package main

// 78.2 平台团队的自助率与 ROI：把"平台值不值得建"落成一道算术题。
// 模型参数（本机计算，假设值）：
//   月操作 2000 次（接入 / 配置变更 / 发布 / 配额申请）
//   每次操作人工 1.5h（业务方手工，含排队与沟通）
//   平台处理一张工单 1.0h，业务方等待 0.5h（两者合计仍是 1.5h）
//   平台建设固定投入 6 人 × 160h/月 = 960h/月
// 结论形式：自助率要到多少，平台才回本。

import "fmt"

const (
	opsPerMonth  = 2000.0 // 每月操作次数
	manualHours  = 1.5    // 每次操作的人工工时（业务方手工）
	serviceHours = 1.0    // 平台处理一张工单的工时
	waitHours    = 0.5    // 业务方等工单/沟通的工时
	buildFTE     = 6.0    // 平台建设固定人力
	hoursPerFTE  = 160.0  // 每人每月工时
)

var buildHours = buildFTE * hoursPerFTE // 960h/月

// platformHours 平台侧工时 = 建设固定投入 + 非自助工单的服务工时
func platformHours(selfRate float64) float64 {
	return buildHours + opsPerMonth*(1-selfRate)*serviceHours
}

// orgHours 建平台后组织总工时 = 平台侧 + 业务方等待
func orgHours(selfRate float64) float64 {
	return platformHours(selfRate) + opsPerMonth*(1-selfRate)*waitHours
}

// saved 净节省工时 = 不建平台（全手工）− 建平台后的总工时
func saved(selfRate float64) float64 {
	return opsPerMonth*manualHours - orgHours(selfRate)
}

func main() {
	fmt.Printf("参数：月操作 %.0f 次，单次人工 %.1fh，平台处理每单 %.1fh，等待 %.1fh，建设 %.0f 人\n\n",
		opsPerMonth, manualHours, serviceHours, waitHours, buildFTE)
	fmt.Printf("不建平台（全手工）月工时 = %.0f × %.1f = %.0f h\n\n",
		opsPerMonth, manualHours, opsPerMonth*manualHours)
	fmt.Printf("%-8s %-8s %-9s %-16s %-14s %s\n",
		"模式", "自助率", "月工单量", "平台人力(建设+服务)", "净节省h/月", "节省/投入")
	fmt.Println("----------------------------------------------------------------------------------")
	modes := []struct {
		name string
		rate float64
	}{
		{"工单制", 0.10},
		{"半自助", 0.60},
		{"全自助", 0.90},
	}
	for _, m := range modes {
		tickets := opsPerMonth * (1 - m.rate)
		staff := platformHours(m.rate) / hoursPerFTE
		sv := saved(m.rate)
		roi := sv / platformHours(m.rate)
		fmt.Printf("%-8s %-8s %-9.0f %-16.1f %-14.0f %+.2f\n",
			m.name, fmt.Sprintf("%.0f%%", m.rate*100), tickets, staff, sv, roi)
	}

	// 回本点：净节省 = 0 → selfRate = buildHours / (ops × manualHours)
	breakeven := buildHours / (opsPerMonth * manualHours)
	fmt.Printf("\n回本自助率 = 平台建设工时 ÷ 全手工总工时 = %.0f ÷ %.0f = %.1f%%\n",
		buildHours, opsPerMonth*manualHours, breakeven*100)
	fmt.Printf("自助率 32%% 时净节省 %.0f h/月（正好回本）；60%% 时 %.0f h/月；90%% 时 %.0f h/月\n",
		saved(0.32), saved(0.60), saved(0.90))

	// 敏感度一：操作量 × 单次人工耗时 决定的回本自助率
	fmt.Printf("\n回本所需自助率（行=月操作次数，列=单次人工工时）：\n")
	fmt.Printf("%-12s %-10s %-10s %s\n", "操作/月", "1.0h", "1.5h", "2.0h")
	for _, ops := range []float64{500, 1000, 2000, 4000} {
		fmt.Printf("%-12.0f", ops)
		for _, a := range []float64{1.0, 1.5, 2.0} {
			r := buildHours / (ops * a)
			if r > 1 {
				fmt.Printf(" %-9s", ">100%")
			} else {
				fmt.Printf(" %-9s", fmt.Sprintf("%.0f%%", r*100))
			}
		}
		fmt.Println()
	}

	// 敏感度二：建设投入变化
	fmt.Printf("\n建设投入(人) → 回本自助率：")
	for _, fte := range []float64{2, 4, 6, 9, 12} {
		r := fte * hoursPerFTE / (opsPerMonth * manualHours)
		fmt.Printf("  %.0f人=%.1f%%", fte, r*100)
	}
	fmt.Println()

	// 折算成钱：全成本按 200 元/h 计（月薪 3.2 万 ÷ 160h）
	const hourlyCost = 200.0
	fmt.Printf("\n按 %.0f 元/h 折算：全手工月成本 %.0f 万元；全自助净省 %.0f h ≈ %.1f 万元/月\n",
		hourlyCost, opsPerMonth*manualHours*hourlyCost/1e4,
		saved(0.90), saved(0.90)*hourlyCost/1e4)

	// 临界操作量：给定目标自助率，需要多少月操作才能回本
	fmt.Printf("\n若自助率能做到 70%%，回本所需月操作量 = %.0f 次（当前 %.0f 次）\n",
		buildHours/(0.70*manualHours), opsPerMonth)
}
