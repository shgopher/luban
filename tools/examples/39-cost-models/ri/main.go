package main

import (
	"fmt"
	"math"
)

// 价格是「示例量级」，用于演示口径，不是任何厂商的报价。
const (
	onDemandHour = 1.20 // 按需：元/实例·小时
	ri1yHour     = 0.78 // 1 年预留、无预付（省 35%）
	ri3yHour     = 0.54 // 3 年预留（无预付省 55%；全预付的资金成本另算）
	spotHour     = 0.36 // Spot（省 70%，可中断）
	monthHours   = 730.0
	years3Hours  = 3 * 8760.0
)

func main() {
	fmt.Println("=== 一、承诺折扣的盈亏平衡：一年至少用多少小时才不亏 ===")
	type plan struct {
		name   string
		price  float64
		upfront bool
		note   string
	}
	for _, p := range []plan{
		{"按需", onDemandHour, false, "基准"},
		{"1 年 RI / SP（无预付）", ri1yHour, false, "用不满这个比例就比按需还贵"},
		{"3 年 RI（无预付）", ri3yHour, false, "承诺越长，要求的确定性越高"},
	} {
		be := p.price / onDemandHour
		fmt.Printf("%-24s 折后 %.2f 元/h  省 %2.0f%%  盈亏平衡使用率 %3.0f%%  %s\n",
			p.name, p.price, (1-p.price/onDemandHour)*100, be*100, p.note)
	}

	// 3 年全预付的资金成本：预付一笔钱，换每小时折扣
	prepay := ri3yHour * years3Hours // 单实例 3 年全预付总额
	const annualRate = 0.08
	monthlyRate := annualRate / 12
	// 36 个月年金现值系数
	af := (1 - math.Pow(1+monthlyRate, -36)) / monthlyRate
	monthlyOnDemand := onDemandHour * monthHours // 全时按需的月成本
	beWithCash := prepay / (monthlyOnDemand * af)
	fmt.Printf("\n3 年全预付：单实例预付 %.0f 元；按 %.0f%% 年化资金成本折现，盈亏平衡使用率从 45%% 抬到 %.0f%%\n",
		prepay, annualRate*100, beWithCash*100)
	fmt.Println("→ 承诺期越长，越要先扣掉“这笔钱本可以干别的”")

	fmt.Println("\n=== 二、三层负载：哪些该买承诺，哪些该留弹性 ===")
	const (
		baseUnits  = 100.0 // 基线：恒常 100 台
		waveUnits  = 20.0  // 波动：20 台，30% 的时间需要
		burstUnits = 15.0  // 突发：15 台，5% 的时间需要
	)
	baseH, waveH, burstH := monthHours, 0.30*monthHours, 0.05*monthHours

	plans := []struct {
		name string
		cost float64
		note string
	}{
		{"A 全部按需", baseUnits*baseH*onDemandHour + waveUnits*waveH*onDemandHour + burstUnits*burstH*onDemandHour, "零承诺，成本最高"},
		{"B 按峰值全买 1 年 RI", (baseUnits+waveUnits+burstUnits)*baseH*ri1yHour, "峰值也承诺，空转严重"},
		{"C 基线 RI + 波动按需 + 突发 Spot", baseUnits*baseH*ri1yHour + waveUnits*waveH*onDemandHour + burstUnits*burstH*spotHour, "按“确定性”分层的标准组合"},
		{"D 基线 3 年 RI + 波动按需 + 突发 Spot", baseUnits*baseH*ri3yHour + waveUnits*waveH*onDemandHour + burstUnits*burstH*spotHour, "最省，但锁 3 年"},
	}
	fmt.Printf("%-38s %10s %12s %s\n", "方案", "月成本", "比按需省", "代价")
	base := plans[0].cost
	for _, p := range plans {
		fmt.Printf("%-38s %10.0f %11.0f%% %s\n", p.name, p.cost, (1-p.cost/base)*100, p.note)
	}

	fmt.Println("\n=== 三、承诺覆盖率：买到基线的百分之多少最省 ===")
	for _, ratio := range []float64{0.5, 0.7, 0.9, 1.0, 1.1, 1.2} {
		commit := baseUnits * ratio
		cost := commit*baseH*ri1yHour + waveUnits*waveH*onDemandHour + burstUnits*burstH*spotHour
		if commit < baseUnits { // 没承诺的基线用按需
			cost += (baseUnits - commit) * baseH * onDemandHour
		}
		waste := 0.0
		if commit > baseUnits {
			waste = (commit - baseUnits) * baseH * ri1yHour
		}
		note := "最优区间"
		if ratio < 0.9 {
			note = "承诺不足，基线部分按需付费"
		}
		if ratio > 1.0 {
			note = "超出基线的承诺必然空转"
		}
		fmt.Printf("承诺 %3.0f%%（%3.0f 台）  月成本 %6.0f 元  空转 %5.0f 元  %s\n", ratio*100, commit, cost, waste, note)
	}

	fmt.Println("\n=== 四、业务下坡时的承诺风险 ===")
	for _, drop := range []float64{0.0, 0.2, 0.4} {
		real := baseUnits * (1 - drop)
		committed := baseUnits // 按原基线买了 1 年
		waste := (committed - real) * baseH * ri1yHour
		if waste < 0 { waste = 0 }
		fmt.Printf("基线下降 %2.0f%%：仍需为 %v 台的承诺付费，空转成本 %.0f 元/月，占承诺支出 %.0f%%\n",
			drop*100, int(committed), waste, waste/(committed*baseH*ri1yHour)*100)
	}
}

