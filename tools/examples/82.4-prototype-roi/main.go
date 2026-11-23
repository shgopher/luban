// 原型的期望收益模型：本机计算，不是线上实测。
// 一个时间盒只有两种结局：要么在盒内拿到「继续/放弃」的结论，要么白烧人天。
// 因此把原型当成一次赌注：
//
//	成本   C = 时间盒天数 × 投入人数（人天）
//	延期   Delay = 时间盒天数 × V（占用关键路径 1 天的机会成本；并行则为 0）
//	收益   = q × p × R
//	       q：不做原型时问题真实存在的概率
//	       p：时间盒内能把问题验出来的概率（这是原型团队唯一能提升的变量）
//	       R：问题留到后期（联调/上线）才发现的返工成本（人天）
//	回本点 p* = (C + Delay) / (q × R)：p 达到多少，原型才不亏。
//
// p* 越低越该做；p* > 1 表示即使 100% 验出问题也不回本，直接上。
package main

import "fmt"

type proto struct {
	name   string  // 原型类型
	box    float64 // 时间盒（天）
	people float64 // 投入人数
	V      float64 // 占用关键路径 1 天的机会成本（人天/天），并行 = 0
	q      float64 // 问题真实存在的概率
	R      float64 // 后期返工的代价（人天）
	note   string
}

func (p proto) cost() float64  { return p.box * p.people }
func (p proto) delay() float64 { return p.box * p.V }
func (p proto) total() float64 { return p.cost() + p.delay() }
func (p proto) breakEvenP() float64 {
	if p.q*p.R == 0 {
		return -1
	}
	return p.total() / (p.q * p.R)
}
func (p proto) breakEvenR(targetP float64) float64 { // 若只能做到 p=targetP，返工成本要多大才回本
	return p.total() / (p.q * targetP)
}
func (p proto) maxBox(targetP float64) float64 { // 若目标是 p=targetP，时间盒上限（天）
	return p.q * targetP * p.R / (p.people + p.V)
}
func (p proto) verdict() string {
	pv := p.breakEvenP()
	switch {
	case pv <= 0.4:
		return "值得做（回本点 ≤0.4，收益 ≥2.5 倍成本）"
	case pv <= 0.8:
		return "有条件做（回本点 0.4–0.8，必须先写清放弃判据）"
	case pv <= 1.0:
		return "边界：回本点 0.8–1.0，只有判据极硬才做"
	default:
		return "直接上（回本点 >1，验出问题也不回本）"
	}
}

func main() {
	cases := []proto{
		{"技术验证（兼容/可行性）", 3, 2, 0, 0.5, 40, "并行验证，不占关键路径"},
		{"性能验证（压测）", 2, 2, 1, 0.6, 25, "占关键路径：压测环境要人等"},
		{"交互验证（可用性测试）", 5, 1, 0, 0.7, 15, "与开发并行，不占关键路径"},
		{"反例：可逆小改动", 3, 1, 2, 0.3, 8, "回滚成本低于原型成本"},
		{"反例：需求模糊的交互原型", 5, 1, 0, 0.8, 5, "问题不在方案而在需求"},
	}

	fmt.Println("== 回本点：p* = (时间盒×人数 + 时间盒×V) / (q×R) ==")
	fmt.Printf("%-24s %6s %6s %6s %6s %6s %8s  %s\n", "原型类型", "盒子", "人天", "延期", "q", "R", "回本点p*", "判定")
	for _, c := range cases {
		fmt.Printf("%-24s %6.0f %6.1f %6.1f %6.2f %6.0f %8.2f  %s\n",
			c.name, c.box, c.cost(), c.delay(), c.q, c.R, c.breakEvenP(), c.verdict())
	}

	fmt.Println("\n== 另一种问法：只能做到 p=0.6（六成概率验出问题），后期返工要多大才回本 ==")
	for _, c := range cases[:3] {
		fmt.Printf("%-24s R* = %.1f 人天（低于此值就不该做原型）\n", c.name, c.breakEvenR(0.6))
	}

	fmt.Println("\n== 时间盒上限：目标 p=0.5 时，盒子最多几天 ==")
	for _, c := range cases[:3] {
		fmt.Printf("%-24s box* = %.1f 天（超过即回不了本）\n", c.name, c.maxBox(0.5))
	}

	fmt.Println("\n== 敏感性：时间盒 ×0.5 / ×1 / ×2，回本点怎么变 ==")
	for _, c := range cases[:3] {
		fmt.Printf("%-24s", c.name)
		for _, k := range []float64{0.5, 1, 2} {
			x := c
			x.box = c.box * k
			fmt.Printf("  box=%.1f天→p*=%.2f", x.box, x.breakEvenP())
		}
		fmt.Println()
	}

	fmt.Println("\n== 判据推导：p*=0.5、q=0.5 时，R 至少是原型成本的几倍 ==")
	fmt.Printf("R ≥ 2(C+Delay)/(q×p) = 2C/(0.5×0.5) = 4C（q=0.5 时返工成本需 ≥4 倍原型成本）\n")
	fmt.Println("\n环境：go1.27.1 darwin/arm64（Apple M1）；本机计算，非线上实测")
}
