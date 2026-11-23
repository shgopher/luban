package main

// 77.4 沟通路径与团队规模：路径总数只是第二列，真正决定"该不该加人/拆分"的是
// 边际沟通成本与团队有效深度产能。模型参数（本机计算，假设值）：
// 名义每周 40h，其中可深度工作 28h；每条沟通路径每周维护 3h。

import "fmt"

const (
	workHours = 40.0 // 名义工时
	deepHours = 28.0 // 每人每周可用于深度工作的时间
	pathHours = 3.0  // 每条路径每周的维护工时（同步、对齐、评审摊薄）
)

func paths(n int) int { return n * (n - 1) / 2 }

// coordPerPerson 总协调工时 pathHours·n(n−1)/2 摊到每人头上
func coordPerPerson(n int) float64 { return pathHours * float64(n-1) / 2 }

// effective 团队每周有效深度产能（被协调压穿时按 0 计）
func effective(n int) float64 {
	left := deepHours - coordPerPerson(n)
	if left < 0 {
		left = 0 // 协调已经吃满全部深度工时，模型饱和
	}
	return float64(n) * left
}

// marginal 加第 n 个人带来的净增有效深度产能
func marginal(n int) float64 { return deepHours - pathHours*float64(n-1) }

func main() {
	fmt.Printf("参数：名义 %.0fh/周，可深度工作 %.0fh/周，每条沟通路径 %.0fh/周\n\n",
		workHours, deepHours, pathHours)
	fmt.Printf("%-6s %-8s %-10s %-12s %-8s %-14s %s\n",
		"人数", "路径数", "人均路径", "人均协调h", "深度h", "团队有效深度h", "建议结构")
	fmt.Println("--------------------------------------------------------------------------------")
	type row struct {
		n    int
		note string
	}
	rows := []row{
		{5, "单团队，喊一声就同步，不需要流程"},
		{9, "单团队上限：站会 + 看板，还能共享上下文"},
		{10, "产能峰值：再加人净增趋零"},
		{15, "拆 2 个 7–8 人团队，跨团队只留接口人"},
		{25, "3 个 8 人团队 + 平台/接口人，25 人已不是一个团队"},
	}
	for _, r := range rows {
		deepLeft := deepHours - coordPerPerson(r.n)
		if deepLeft < 0 {
			deepLeft = 0
		}
		fmt.Printf("%-6d %-8d %-10d %-12.1f %-8.1f %-14.1f %s\n",
			r.n, paths(r.n), r.n-1, coordPerPerson(r.n), deepLeft, effective(r.n), r.note)
	}

	fmt.Println("\n加第 n 个人的边际账：")
	fmt.Printf("%-6s %-10s %-12s %s\n", "n", "新增路径", "新增协调h", "净增有效深度h")
	for _, n := range []int{5, 9, 10, 11, 16} {
		fmt.Printf("%-6d %-10d %-12.0f %+.1f\n", n, n-1, pathHours*float64(n-1), marginal(n))
	}

	// 峰值：扫 2..40 人，找有效深度产能最高的人数
	bestN, bestEff := 0, 0.0
	for n := 2; n <= 40; n++ {
		if e := effective(n); e > bestEff {
			bestN, bestEff = n, e
		}
	}
	fmt.Printf("\n有效产能峰值：%d 人、%.1f h/周；第 %d 人起边际净增为负（%.1f h）\n",
		bestN, bestEff, bestN+1, marginal(bestN+1))
	fmt.Printf("对照：10 人 %.1f h，11 人 %.1f h，15 人 %.1f h，25 人 %.1f h（饱和）\n",
		effective(10), effective(11), effective(15), effective(25))

	// 拆分前后对比：sizes 是拆分后的团队人数；团队间只走接口人，
	// 所以团队间路径数 = k(k−1)/2，而不是把两个团队的人两两连线。
	split := func(sizes []int) (int, float64) {
		p, e := 0, 0.0
		for _, n := range sizes {
			p += paths(n)
			e += effective(n)
		}
		inter := len(sizes) * (len(sizes) - 1) / 2
		p += inter
		e -= pathHours * float64(inter)
		return p, e
	}
	p25, e25 := split([]int{8, 8, 9})
	p15, e15 := split([]int{7, 8})
	fmt.Printf("\n拆分账（团队间只走接口人，团队间路径 = k(k−1)/2）：\n")
	fmt.Printf("25 人单团队：路径 %d，有效深度 %.1f h/周 → 拆 8+8+9：路径 %d（含 3 条团队间），有效深度 %.1f h/周（路径 −%.0f%%）\n",
		paths(25), effective(25), p25, e25, 100*(1-float64(p25)/float64(paths(25))))
	fmt.Printf("15 人单团队：路径 %d，有效深度 %.1f h/周 → 拆 7+8：路径 %d（含 1 条团队间），有效深度 %.1f h/周（路径 −%.0f%%）\n",
		paths(15), effective(15), p15, e15, 100*(1-float64(p15)/float64(paths(15))))

	// 新人 ramp 账：把第 10 个人加进 9 人团队。
	// 前 8 周新人只产出 30%；同时老成员各多 1 条路径，每人少 1.5h 深度工时。
	newbieFactor := 0.30
	rampWeeks, lossPerWeek := 8, 9*1.5-14.5*newbieFactor
	fmt.Printf("\n加第 10 人的回本账：前 %d 周团队每周净少 %.1f h，之后每周净增 %.1f h\n",
		rampWeeks, lossPerWeek, marginal(10))
	fmt.Printf("回本周期 ≈ %.0f 周（%.1f 年），前提是新人 ramp 期产出 %.0f%%\n",
		float64(rampWeeks)*lossPerWeek/marginal(10),
		float64(rampWeeks)*lossPerWeek/marginal(10)/52, newbieFactor*100)

	// 拆分信号：边际净增 < 2h/周 的第一个人数
	for n := 2; n <= 40; n++ {
		if marginal(n) < 2 {
			fmt.Printf("\n拆分信号：加第 %d 人时边际净增只剩 %.1f h/周（< 2h），团队规模已经到顶\n",
				n, marginal(n))
			break
		}
	}
}
