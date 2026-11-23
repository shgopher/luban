package main

import (
	"fmt"
	"math/rand"
)

const (
	orderN   = 10000 // 订单数
	returnR  = 0.08  // 退货率
	goodsVal = 100.0 // 单件货值/售价（元）
)

// 退货原因分布（合计 100%）
type reason struct {
	name   string
	share  float64
	cost   float64 // 逆向物流 + 质检 + 上架成本（元/件）
	recov  float64 // 回收价值（元/件）：再售、供应商承担、承运商赔付
	days   float64 // 处理时长（天）
	resale float64 // 可再次销售的比例
}

var reasons = []reason{
	{"无理由 / 不喜欢", 0.45, 8, 57, 2, 0.95}, // 区域退货仓直检直发
	{"尺码 / 参数不符", 0.20, 8, 57, 2, 0.95}, // 同上
	{"质量问题", 0.20, 4, 50, 7, 0.00},      // 退回供应商，供应商承担
	{"运输破损", 0.10, 2, 30, 1, 0.00},      // 就地报废，承运商赔付
	{"其他", 0.05, 17, 40, 5, 0.40},       // 回中心仓质检
}

func main() {
	returns := int(orderN * returnR)

	// 策略 A：全部退回中心仓，统一质检、上架、再处置
	costA := 17.0 * float64(returns) // 12 运费 + 3 质检 + 2 上架
	recovA := 0.0                    // 55% 按 60 元再售、25% 降级按 30 元
	recovA = 0.55*60*float64(returns) + 0.25*30*float64(returns)
	lossA := goodsVal*float64(returns) - recovA + costA

	// 策略 B：按原因分级处置，只有“其他”回中心仓
	r := rand.New(rand.NewSource(2026))
	costB, recovB, daysB := 0.0, 0.0, 0.0
	// 先做一次随机分配，得出各原因的件数（分布稳定，随机只影响 ±1 件）
	counts := make([]int, len(reasons))
	for i := 0; i < returns; i++ {
		x := r.Float64()
		sum := 0.0
		for k := range reasons {
			sum += reasons[k].share
			if x <= sum {
				counts[k]++
				break
			}
		}
	}
	for k, rs := range reasons {
		n := float64(counts[k])
		costB += rs.cost * n
		recovB += rs.recov * n
		daysB += rs.days * n
	}
	lossB := goodsVal*float64(returns) - recovB + costB

	fmt.Printf("订单 %d 单、退货率 %.0f%% → %d 单退货；货值 %.0f 元/件\n\n", orderN, returnR*100, returns, goodsVal)
	fmt.Printf("退货原因：")
	for k := range reasons {
		fmt.Printf("%s %.1f%%(%d 件)  ", reasons[k].name, reasons[k].share*100, counts[k])
	}
	fmt.Println()

	fmt.Printf("\n%-26s %-11s %-11s %-12s %-13s %s\n",
		"策略", "逆向成本", "回收价值", "货值回收率", "净损失", "平均处理天数")
	fmt.Printf("%-26s %8.0f 元  %8.0f 元   %5.1f%%     %10.0f 元   %4.1f 天\n",
		"A 全部回中心仓质检", costA, recovA, 100*recovA/(goodsVal*float64(returns)), lossA, 5.0)
	fmt.Printf("%-26s %8.0f 元  %8.0f 元   %5.1f%%     %10.0f 元   %4.1f 天\n",
		"B 按原因分级处置", costB, recovB, 100*recovB/(goodsVal*float64(returns)), lossB, daysB/float64(returns))
	fmt.Printf("\nB 比 A 少损失 %.0f 元（%.0f%%），处理时长缩短 %.1f 天；关键在“无理由/尺码”直检直发，不回中心仓\n",
		lossA-lossB, 100*(lossA-lossB)/lossA, 5.0-daysB/float64(returns))

	// 异常件赔付：50 单异常（丢 10 / 破损 20 / 超时 20）
	type claim struct {
		name  string
		n     int
		each  float64
		basis string
	}
	claims := []claim{
		{"丢件", 10, 300, "按保价 300 元赔"},
		{"破损", 20, goodsVal * 0.8, "按货值 80% 赔"},
		{"超时", 20, 5, "5 元/单（延迟 1~3 天）"},
	}
	fmt.Println("\n异常件赔付（50 单异常：丢 10 / 破损 20 / 超时 20）：")
	fmt.Printf("%-8s %-6s %-10s %-12s %s\n", "类型", "件数", "单件赔付", "赔付合计", "依据")
	total := 0.0
	for _, c := range claims {
		total += float64(c.n) * c.each
		fmt.Printf("%-8s %-6d %7.1f 元  %8.1f 元   %s\n", c.name, c.n, c.each, float64(c.n)*c.each, c.basis)
	}
	fmt.Printf("合计 50 单赔付 %.0f 元；同期正向运费按 8 元/单计为 %.0f 元，赔付占运费 %.1f%%\n",
		total, 8*float64(orderN), 100*total/(8*float64(orderN)))
}
