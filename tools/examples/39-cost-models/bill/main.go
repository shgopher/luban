package main

import (
	"fmt"
	"math"
	"sort"
)

// 账单行：服务、计量单位、用量、单价（元）。示例数据，用于演示口径。
type line struct {
	service string
	unit    string
	qty     float64
	price   float64
	tag     string // 成本归属标签；空串 = 无主
}

func cost(ls []line) float64 { s := 0.0; for _, l := range ls { s += l.qty * l.price }; return s }

func main() {
	// 上月（11 月）与本月（12 月）的账单明细
	last := []line{
		{"计算（K8s 节点）", "vCPU·h", 120000, 0.35, "订单"},
		{"计算（K8s 节点）", "vCPU·h", 40000, 0.35, "搜索"},
		{"对象存储", "GB·月", 60000, 0.15, "订单"},
		{"网络 / CDN 回源", "GB", 30000, 0.50, "订单"},
		{"数据库（RDS）", "实例·h", 20000, 1.00, "订单"},
		{"可观测性（日志摄取）", "GB", 20000, 1.00, ""},
	}
	now := []line{
		{"计算（K8s 节点）", "vCPU·h", 135000, 0.35, "订单"},
		{"计算（K8s 节点）", "vCPU·h", 44000, 0.35, "搜索"},
		{"对象存储", "GB·月", 78000, 0.15, "订单"},
		{"网络 / CDN 回源", "GB", 52000, 0.50, "订单"},
		{"数据库（RDS）", "实例·h", 20000, 1.05, "订单"},
		{"可观测性（日志摄取）", "GB", 26000, 1.00, ""},
		{"向量检索（新增）", "实例·h", 12000, 1.50, "推荐"},
	}

	totalLast, totalNow := cost(last), cost(now)
	fmt.Println("=== 一、两个月总额与单位成本 ===")
	const ordersLast, ordersNow = 500000.0, 545000.0
	fmt.Printf("上月 %.0f 元，%0.f 单 → 单位成本 %.2f 元/单\n", totalLast, ordersLast, totalLast/ordersLast)
	fmt.Printf("本月 %.0f 元，%.0f 单 → 单位成本 %.2f 元/单\n", totalNow, ordersNow, totalNow/ordersNow)
	fmt.Printf("总额 %+.0f 元（%+.1f%%）；单位成本 %+.1f%% → %s\n\n",
		totalNow-totalLast, (totalNow/totalLast-1)*100, (totalNow/ordersNow)/(totalLast/ordersLast)*100-100,
		pick((totalNow/ordersNow)/(totalLast/ordersLast) < 1.0, "效率在提升，涨幅主要来自业务量", "效率在退化，涨幅超过业务量"))

	// 按服务聚合：量 → 单价 → 交叉项
	type agg struct{ qty, price float64 }
	sum := func(ls []line) map[string]agg {
		m := map[string]agg{}
		for _, l := range ls {
			a := m[l.service]
			a.qty += l.qty
			a.price = l.price // 同服务同单价；混合单价时这里应改为加权
			m[l.service] = a
		}
		return m
	}
	l, n := sum(last), sum(now)
	keys := map[string]bool{}
	for k := range l { keys[k] = true }
	for k := range n { keys[k] = true }

	fmt.Println("=== 二、Δ成本拆成量效应 / 价效应 / 交叉项 ===")
	type row struct{ service string; volume, price, cross, delta float64 }
	rows := []row{}
	for k := range keys {
		a, b := l[k], n[k]
		volume := (b.qty - a.qty) * a.price
		price := a.qty * (b.price - a.price)
		cross := (b.qty - a.qty) * (b.price - a.price)
		if a.qty == 0 { // 新增服务：全额算新增，不摊进交叉项
			volume, price, cross = b.qty*b.price, 0, 0
		}
		rows = append(rows, row{k, volume, price, cross, volume + price + cross})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].delta > rows[j].delta })
	fmt.Printf("总量变化 %+.0f 元\n", totalNow-totalLast)
	var sv, sp, sc float64
	for _, r := range rows {
		sv, sp, sc = sv+r.volume, sp+r.price, sc+r.cross
		fmt.Printf("%-24s 量 %+7.0f  价 %+6.0f  交叉 %+5.0f  → %+7.0f\n", r.service, r.volume, r.price, r.cross, r.delta)
	}
	fmt.Printf("%-24s 量 %+7.0f  价 %+6.0f  交叉 %+5.0f  → %+7.0f\n", "合计", sv, sp, sc, sv+sp+sc)

	fmt.Println("\n=== 三、异常定位：按“量 / 价 / 新增”三分类 ===")
	var volDriven, priceDriven, newDriven float64
	for _, r := range rows {
		a := l[r.service]
		switch {
		case a.qty == 0: // 上月不存在 = 新增服务
			newDriven += r.delta
		case math.Abs(r.price) > math.Abs(r.volume): // 价效应盖过量效应
			priceDriven += r.delta
		default:
			volDriven += r.delta
		}
	}
	fmt.Printf("量驱动 %+.0f 元（用量涨，先问业务是否真涨了）\n", volDriven)
	fmt.Printf("价驱动 %+.0f 元（单价或规格变了，先查变更与计费口径）\n", priceDriven)
	fmt.Printf("新增服务 %+.0f 元（先问谁批准的、有没有预算）\n", newDriven)
	fmt.Printf("残留 %+.0f 元（交叉项与抵扣）\n", totalNow-totalLast-volDriven-priceDriven-newDriven)

	fmt.Println("\n=== 四、归因覆盖率：无主成本占多少 ===")
	var untagged float64
	for _, x := range now { if x.tag == "" { untagged += x.qty * x.price } }
	fmt.Printf("本月无标签成本 %.0f 元，占总额 %.1f%%（治理目标：< 5%%）\n", untagged, untagged/totalNow*100)

	fmt.Println("\n=== 五、下月预算：基线 × 增长 + 计划变更 ===")
	avg3 := (totalLast + totalNow) / 2 // 只有两个月时用两点均值
	growth := 0.06                     // 业务增长假设
	planned := 6000.0                  // 已批准的变更（新增向量检索扩容）
	forecast := avg3*(1+growth) + planned
	fmt.Printf("近两月均值 %.0f，增长假设 %.0f%%，计划变更 %+.0f → 下月预算 %.0f 元（±10%% 区间 %.0f–%.0f）\n",
		avg3, growth*100, planned, forecast, forecast*0.9, forecast*1.1)
}

func pick(cond bool, a, b string) string { if cond { return a }; return b }
