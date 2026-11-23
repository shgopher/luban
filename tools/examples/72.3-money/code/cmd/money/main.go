// 命令 money：金额表示、舍入模式、残差分摊的本机实测。
// 每个小节都打印成 markdown 表，正文直接粘贴，不做二次加工。
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// roundHalfUp 远离零的四舍五入（scale=100 表示保留 2 位小数）。
// 注意：这不是「floor(x+0.5)」——floor(x+0.5) 是「向 +∞ 取半」，负数会偏。
func roundHalfUp(x float64, scale int64) float64 {
	y := x * float64(scale)
	if y >= 0 {
		return math.Floor(y+0.5) / float64(scale)
	}
	return math.Ceil(y-0.5) / float64(scale)
}

// roundHalfEven 银行家舍入：恰好为 .5 时进到最近的偶数。
func roundHalfEven(x float64, scale int64) float64 {
	return math.RoundToEven(x*float64(scale)) / float64(scale)
}

func roundTowardZero(x float64, scale int64) float64 {
	return math.Trunc(x*float64(scale)) / float64(scale)
}

func roundFloor(x float64, scale int64) float64 {
	return math.Floor(x*float64(scale)) / float64(scale)
}

func roundCeil(x float64, scale int64) float64 {
	return math.Ceil(x*float64(scale)) / float64(scale)
}

// roundDivHalfUp 是整数版四舍五入：num/den 精确求值后 half-up（远离零）。
// 金额场景的正确做法：全程 int64 分，只在「出具金额」时用整数除一次，
// 不经过 float64，就不会有 1.005 那种表示误差。
func roundDivHalfUp(num, den int64) int64 {
	if num >= 0 {
		return (num + den/2) / den
	}
	return -((-num + den/2) / den)
}

// splitLargestRemainder 把 total（最小货币单位，如「分」）按 weights 分摊。
// 算法：① 先向下取整得到基础份额 ② 记录每份的余数 ③ 把剩下的单位
// 按余数从大到小逐份 +1；余数相同按索引升序（保证同输入同输出）。
// 铁律：sum(返回) == total 恒成立——这是「账单行合计 vs 汇总不一致」的唯一解。
func splitLargestRemainder(total int64, weights []int64) []int64 {
	var wsum int64
	for _, w := range weights {
		wsum += w
	}
	out := make([]int64, len(weights))
	rem := make([]int64, len(weights))
	var allocated int64
	for i, w := range weights {
		out[i] = total * w / wsum
		rem[i] = total * w % wsum
		allocated += out[i]
	}
	left := total - allocated // 0 <= left < len(weights)
	idx := make([]int, len(weights))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return rem[idx[a]] > rem[idx[b]] })
	for k := int64(0); k < left; k++ {
		out[idx[k]]++
	}
	return out
}

// splitNaiveRound 是常见的错法：每份独立四舍五入，最后不做差额校验。
func splitNaiveRound(total int64, weights []int64) []int64 {
	var wsum int64
	for _, w := range weights {
		wsum += w
	}
	out := make([]int64, len(weights))
	for i, w := range weights {
		out[i] = roundDivHalfUp(total*w, wsum)
	}
	return out
}

func sum(xs []int64) int64 {
	var s int64
	for _, x := range xs {
		s += x
	}
	return s
}

func main() {
	fmt.Println("## A1 float64 的表示误差与累加误差")
	fmt.Println()
	// 用变量而不是常量：Go 的非类型常量表达式按任意精度算，0.1+0.2==0.3 会得到 true，
	// 那是在骗自己。金额在运行时一定是变量。
	a, b, c := 0.1, 0.2, 0.3
	fmt.Printf("0.1+0.2 的 %.20f（运行时 ==0.3: %v；字面常量比较会得 true，因为常量按任意精度算）\n", a+b, a+b == c)
	fmt.Printf("0.1 的 %.20f\n", a)
	fmt.Printf("19.99*3 的 %.20f（int64 分口径：%d 分）\n", 19.99*3, int64(1999)*3)
	// 必须用变量：写成 1.005*100 时编译器按任意精度算，会得到「正好 100.5」的假象
	v1005, v2675 := 1.005, 2.675
	fmt.Printf("1.005 的原值 %.20f：乘 100 = %.20f，roundHalfUp=%.2f\n", v1005, v1005*100, roundHalfUp(v1005, 100))
	fmt.Printf("2.675 的原值 %.20f：乘 100 = %.20f，roundHalfUp=%.2f\n", v2675, v2675*100, roundHalfUp(v2675, 100))
	fmt.Println("（1.005 与 2.675 看似同一类「半值」，前者舍成 1.00、后者舍成 2.68——差别只来自二进制表示）")

	const n = 1000000
	var f float64
	var i int64
	for k := 0; k < n; k++ {
		f += 0.01
		i++ // 0.01 元 = 1 分
	}
	fmt.Println()
	fmt.Printf("100 万笔 0.01：float64 累加 = %.10f 元（误差 %+.10f 元）\n", f, f-10000.0)
	fmt.Printf("100 万笔 0.01：int64 分累加 = %d 分 = %.2f 元（误差 0）\n", i, float64(i)/100)
	var f2 float64
	var i2 int64
	for k := 0; k < n; k++ {
		f2 += 19.99
		i2 += 1999
	}
	fmt.Printf("100 万笔 19.99：float64 累加 = %.6f 元（应为 19990000.00，误差 %+.6f 元）\n", f2, f2-19990000.0)
	fmt.Printf("100 万笔 19.99：int64 分累加 = %d 分 = %.2f 元\n", i2, float64(i2)/100)

	fmt.Println()
	fmt.Println("## A2 五种舍入模式对同一组数的影响（第一张：取整到 0 位小数，暴露 .5 分歧）")
	fmt.Println()
	fmt.Println("| 原值 | half-up（远离零） | half-even（银行家） | toward-zero | floor | ceil |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, v := range []float64{0.5, 1.5, 2.5, 3.5, 4.5, -0.5, -1.5, -2.5} {
		fmt.Printf("| %.1f | %.0f | %.0f | %.0f | %.0f | %.0f |\n",
			v, roundHalfUp(v, 1), roundHalfEven(v, 1), roundTowardZero(v, 1),
			roundFloor(v, 1), roundCeil(v, 1))
	}
	fmt.Println()
	fmt.Println("第二张：保留 2 位小数（业务上真正会用的档位）")
	fmt.Println()
	fmt.Println("| 原值 | half-up | half-even | toward-zero | floor | ceil |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, v := range []float64{0.125, 0.375, -0.125, 2.675, 1.005, 19.99 * 3, 100.0 / 3} {
		fmt.Printf("| %.10f | %.2f | %.2f | %.2f | %.2f | %.2f |\n",
			v, roundHalfUp(v, 100), roundHalfEven(v, 100), roundTowardZero(v, 100),
			roundFloor(v, 100), roundCeil(v, 100))
	}

	fmt.Println()
	fmt.Println("## A3 舍入顺序：逐行舍入累加 vs 合计一次舍入（整数分口径，精确）")
	// 场景：两个行项目各 33.33 元，税率 9.5%。全程 int64 分，不做浮点。
	lineCents := int64(3333)
	taxBps := int64(950) // 9.50%
	perLine := roundDivHalfUp(lineCents*taxBps, 10000)
	both := roundDivHalfUp(lineCents*2*taxBps, 10000)
	fmt.Printf("每行 33.33 元、税率 9.50%%：逐行税 %d 分 × 2 = %d 分；合计计税 %d 分；差 %d 分/单\n",
		perLine, perLine*2, both, perLine*2-both)
	fmt.Printf("整单金额：逐行 = %d 分（%.2f 元），合计 = %d 分（%.2f 元）\n",
		lineCents*2+perLine*2, float64(lineCents*2+perLine*2)/100,
		lineCents*2+both, float64(lineCents*2+both)/100)

	// 单价 0.05、3 行、税率 10%：更小的数字，差 1 分
	unit, qty := int64(5), int64(3)
	tl := unit * qty
	perLine2 := roundDivHalfUp(tl*1000, 10000)
	total2 := roundDivHalfUp(tl*qty*1000, 10000)
	fmt.Printf("单价 0.05 × 3 行、税率 10.00%%：逐行税 %d 分×3 = %d 分；合计计税 %d 分；差 %d 分\n",
		perLine2, perLine2*3, total2, perLine2*3-total2)

	fmt.Println()
	fmt.Println("## A4 残差分摊：100.00 元给 3 人（最小单位 = 分）")
	cents := int64(10000)
	weights := []int64{1, 1, 1}
	naive := splitNaiveRound(cents, weights)
	lr := splitLargestRemainder(cents, weights)
	fmt.Printf("每份独立四舍五入：分 %v，合计 %d 分（应收 %d 分，差 %+d 分）\n", naive, sum(naive), cents, sum(naive)-cents)
	fmt.Printf("最大余数法：      分 %v，合计 %d 分（应收 %d 分，差 %+d 分）\n", lr, sum(lr), cents, sum(lr)-cents)
	fmt.Printf("换算成元：%.2f / %.2f / %.2f\n", float64(lr[0])/100, float64(lr[1])/100, float64(lr[2])/100)

	fmt.Println()
	fmt.Println("## A4b JSON 传输：数字型金额在 2^53 之后会静默改值")
	bigAmount := int64(9007199254740993) // 2^53 + 1，int64 最小单位金额
	jb, _ := json.Marshal(map[string]any{"amount_minor": bigAmount})
	var asFloat map[string]float64
	_ = json.Unmarshal(jb, &asFloat)
	jb2, _ := json.Marshal(map[string]any{"amount_minor": strconv.FormatInt(bigAmount, 10)})
	var asStr map[string]string
	_ = json.Unmarshal(jb2, &asStr)
	back, _ := strconv.ParseInt(asStr["amount_minor"], 10, 64)
	fmt.Printf("int64 %d 编码为 JSON 数字：%s → 按 float64 解析 = %.0f（是否等于原值: %v）\n",
		bigAmount, string(jb), asFloat["amount_minor"], int64(asFloat["amount_minor"]) == bigAmount)
	fmt.Printf("编码为 JSON 字符串：%s → 按 int64 解析 = %d（是否等于原值: %v）\n",
		string(jb2), back, back == bigAmount)
	fmt.Printf("安全整数边界：2^53-1 = %d；超过它的整数在 JS/float64 里不再一一对应\n",
		int64(1)<<53-1)

	fmt.Println()
	fmt.Println("## A5 更多分摊用例：最大余数法恒等式 sum == total")
	fmt.Println()
	fmt.Println("| 总额(分) | 权重 | 每份独立四舍五入 | 合计 | 最大余数法 | 合计 | 独立舍入是否平账 |")
	fmt.Println("|---|---|---|---|---|---|---|")
	cases := []struct {
		total   int64
		weights []int64
	}{
		{10000, []int64{1, 1, 1}},
		{10001, []int64{1, 1, 1}},
		{999, []int64{1, 1, 1, 1, 1, 1, 1}},
		{100000, []int64{3, 5, 2}},
		{12345, []int64{17, 41, 42}},
		{1, []int64{1, 1, 1, 1}},
		{100, []int64{1, 1, 1, 1, 1, 1}},
	}
	for _, cs := range cases {
		nv := splitNaiveRound(cs.total, cs.weights)
		lrv := splitLargestRemainder(cs.total, cs.weights)
		fmt.Printf("| %d | %v | %v | %d | %v | %d | %v |\n",
			cs.total, cs.weights, nv, sum(nv), lrv, sum(lrv), sum(nv) == cs.total)
	}

	fmt.Println()
	fmt.Println("## A6 分摊再累加：1 万笔 100.00 元三方分账")
	var acc [3]int64
	for k := 0; k < 10000; k++ {
		got := splitLargestRemainder(10000, []int64{1, 1, 1})
		for j := 0; j < 3; j++ {
			acc[j] += got[j]
		}
	}
	fmt.Printf("1 万笔 × 100.00 元：三方累计 %v 分 = %.2f 元；应收 %d 分 = %.2f 元；差 %d 分\n",
		acc, float64(acc[0]+acc[1]+acc[2])/100, int64(10000*10000), 1000000.0, sum(acc[:])-100000000)
}
