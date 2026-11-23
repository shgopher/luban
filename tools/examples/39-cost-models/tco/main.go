package main

import (
	"fmt"
	"math"
)

// 单实例折算：自建消息队列 vs 托管服务 vs 按量托管。示例参数，用于演示口径。
type option struct {
	name     string
	oneTime  float64 // 一次性投入（建设、迁移、培训）
	monthly  float64 // 月度运营成本（资源 + 人力 + 工具）
	riskPerM float64 // 每月期望风险成本（故障损失 × 概率）
}

func pvMonthly(monthly float64, months int, annualRate float64) float64 {
	r := annualRate / 12
	af := (1 - math.Pow(1+r, -float64(months))) / r // 年金现值系数
	return monthly * af
}

func main() {
	const (
		months     = 36
		annualRate = 0.08
	)

	// 自建：3 台 8C32G（2400 元/台·月）+ 3TB SSD（500 元/TB·月）+ 0.5 FTE + 监控
	selfMonthly := 3*2400 + 3*500 + 0.5*35000 + 800
	// 托管：资源按 1.8 倍溢价，人力 0.1 FTE
	managedMonthly := 1.8*(3*2400+3*500) + 0.1*35000
	// 按量托管：忙时用量计费，人力 0.05 FTE
	serverlessMonthly := 22000 + 0.05*35000

	opts := []option{
		{"自建（IaaS 上自己运维）", 30500, selfMonthly, 4167},  // 30,500 = 15 人日建设 + 培训
		{"全托管服务", 40000, managedMonthly, 800},             // 40,000 = 迁移 + 双跑一个月
		{"按量托管（Serverless）", 5000, serverlessMonthly, 1200},
	}

	fmt.Println("=== 一、三年 TCO：一次性 + 月成本折现 + 风险成本 ===")
	fmt.Printf("折现率 %.0f%%/年，36 个月年金现值系数 %.2f\n\n", annualRate*100, pvMonthly(1, months, annualRate))
	fmt.Printf("%-26s %10s %10s %12s %10s %s\n", "方案", "一次性", "月运营", "风险/月", "三年TCO", "相对最省")
	var best, bestCost float64 = 0, math.Inf(1)
	for i, o := range opts {
		tco := o.oneTime + pvMonthly(o.monthly+o.riskPerM, months, annualRate)
		fmt.Printf("%-26s %10.0f %10.0f %12.0f %10.0f\n", o.name, o.oneTime, o.monthly, o.riskPerM, tco)
		if tco < bestCost { bestCost, best = tco, float64(i) }
	}
	for _, o := range opts {
		tco := o.oneTime + pvMonthly(o.monthly+o.riskPerM, months, annualRate)
		fmt.Printf("  %-26s 比最省方案多 %+.0f 元（%+.0f%%）\n", o.name, tco-bestCost, (tco/bestCost-1)*100)
	}
	fmt.Printf("→ 最省：%s\n", opts[int(best)].name)

	fmt.Println("\n=== 二、结论对哪个假设最敏感 ===")
	fmt.Printf("%-18s %-14s %-14s %-14s %s\n", "假设变化", "自建TCO", "托管TCO", "差额", "仍选托管?")
	type sens struct{ name string; selfM, managedM float64 }
	for _, s := range []sens{
		{"基准", selfMonthly, managedMonthly},
		{"人力单价 -50%", 3*2400 + 3*500 + 0.5*17500 + 800, 1.8*(3*2400+3*500) + 0.1*17500},
		{"人力单价 +50%", 3*2400 + 3*500 + 0.5*52500 + 800, 1.8*(3*2400+3*500) + 0.1*52500},
		{"托管溢价 1.5 倍", selfMonthly, 1.5*(3*2400+3*500) + 0.1*35000},
		{"托管溢价 2.5 倍", selfMonthly, 2.5*(3*2400+3*500) + 0.1*35000},
		{"运维人力降到 0.2 FTE", 3*2400 + 3*500 + 0.2*35000 + 800, managedMonthly},
	} {
		selfTCO := 30500 + pvMonthly(s.selfM+4167, months, annualRate)
		manTCO := 40000 + pvMonthly(s.managedM+800, months, annualRate)
		pick := "是"
		if selfTCO < manTCO { pick = "否，自建更省" }
		fmt.Printf("%-18s %-14.0f %-14.0f %+14.0f %s\n", s.name, selfTCO, manTCO, selfTCO-manTCO, pick)
	}

	fmt.Println("\n=== 三、多拆一个微服务的固定成本 ===")
	perService := 1200.0 // 网关/注册/监控/值班/CI 的月分摊（示例）
	fmt.Printf("每个服务每月固定分摊 %.0f 元 → 每年 %.0f 元；拆 20 个服务每年多 %.0f 元\n",
		perService, perService*12, perService*12*20)
	fmt.Printf("若为此多养 0.5 FTE 平台团队（%.0f 元/月），每年再加 %.0f 元\n", 0.5*35000, 0.5*35000*12)
	fmt.Printf("对照：单体的发布效率收益通常要“每周多发布 N 次 × 每次事故损失减少”才摊得平\n\n")
	fmt.Println("=== 四、规模门槛：多少台等效资源时自建才开始更省 ===")
	selfFixed := 0.5*35000 + 800 + 4167 // 人力 + 工具 + 风险
	manFixed := 0.1*35000 + 800
	selfPer, manPer := 2400.0+500.0, 1.8*(2400.0+500.0)
	selfOne, manOne := 30500.0, 40000.0
	// 把一次性差额摊进 36 个月
	amort := (selfOne - manOne) / pvMonthly(1, months, annualRate)
	be := (manFixed - selfFixed + amort) / (selfPer - manPer)
	fmt.Printf("自建：固定 %.0f 元/月 + %.0f 元/台·月；托管：固定 %.0f 元/月 + %.0f 元/台·月\n", selfFixed, selfPer, manFixed, manPer)
	fmt.Printf("规模门槛：约 %.1f 台等效资源。低于它自建更贵，高于它自建才开始摊薄人力\n", be)
	fmt.Printf("%-8s %-14s %-14s %s\n", "规模", "自建/月", "托管/月", "更省")
	for _, n := range []float64{3, 5, 8, 20, 50, 100} {
		selfM := selfFixed + selfPer*n
		manM := manFixed + manPer*n
		pick := "托管"
		if selfM < manM { pick = "自建" }
		fmt.Printf("%-8.0f %-14.0f %-14.0f %s\n", n, selfM, manM, pick)
	}
}
