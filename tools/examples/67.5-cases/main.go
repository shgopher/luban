// 67.5 三级联动：限额（授权）→ 拦截（决策）→ 复核（处置）。
// 金额区间 × 风险等级 → 放行 / 二次验证 / 人工复核 / 拦截；超限额只升档、不直接拒；
// 只有硬上限与名单命中才无条件拦截。复核超时按金额分级兜底。
package main

import "fmt"

type Risk int

const (
	Low Risk = iota
	Mid
	High
	Critical
)

func (r Risk) String() string {
	return [...]string{"低", "中", "高", "极高"}[r]
}

type Action int

const (
	Pass Action = iota
	Verify
	Review
	Block
)

func (a Action) String() string {
	return [...]string{"放行", "二次验证", "人工复核", "拦截"}[a]
}

const (
	hardCeiling = 200000 // 硬上限：任何风险等级、任何限额都不能放过去
	reviewSLA   = 15     // 复核 SLA，分钟
)

// 金额 4 档 × 风险 4 档。金额档：<1 千 / 1 千–1 万 / 1 万–5 万 / ≥5 万。
var grid = [4][4]Action{
	{Pass, Pass, Verify, Block},    // < 1,000
	{Pass, Verify, Review, Block},  // 1,000 – 9,999
	{Verify, Review, Block, Block}, // 10,000 – 49,999
	{Review, Block, Block, Block},  // ≥ 50,000
}

func amountBand(amount int) int {
	switch {
	case amount < 1000:
		return 0
	case amount < 10000:
		return 1
	case amount < 50000:
		return 2
	default:
		return 3
	}
}

// 第一级：限额。超限不直接拒，只把处置升一档（复核已是人工档，不再升）。
func applyLimit(a Action, amount, limit int) (Action, bool) {
	if amount <= limit {
		return a, false
	}
	switch a {
	case Pass:
		return Verify, true
	case Verify:
		return Review, true
	default: // Review / Block 不动
		return a, true
	}
}

// 决策：名单 → 硬上限 → 矩阵 → 限额升档。返回动作与可解释原因。
func decide(amount int, risk Risk, limit int, blacklisted bool) (Action, string) {
	if blacklisted {
		return Block, "名单命中：确定性事实，直接拦截"
	}
	if amount > hardCeiling {
		return Block, fmt.Sprintf("金额 %d 超硬上限 %d，绕过限额审批也必须拒", amount, hardCeiling)
	}
	a := grid[amountBand(amount)][risk]
	reason := fmt.Sprintf("矩阵[金额档%d][风险%s]=%s", amountBand(amount), risk, a)
	if na, escalated := applyLimit(a, amount, limit); escalated {
		if na != a {
			reason += fmt.Sprintf("；超限额 %d → 升档为 %s", limit, na)
		} else {
			reason += fmt.Sprintf("；超限额 %d，但已在人工档，保持 %s", limit, na)
		}
		a = na
	}
	return a, reason
}

// 第三级：复核超时兜底。小额放行并事后补查，大额超时拒绝。
func resolveReview(amount int, timedOut bool) (Action, string) {
	if !timedOut {
		return Review, "等待人工复核（不占主链路）"
	}
	if amount >= 10000 {
		return Block, "复核超时且金额 ≥ 1 万：拒绝（fail-close）"
	}
	return Pass, "复核超时且金额 < 1 万：放行 + 事后补查（挂号）"
}

type testCase struct {
	id     string
	amount int
	risk   Risk
	limit  int
	black  bool
	want   Action
}

func main() {
	cases := []testCase{
		{"T01", 500, Low, 5000, false, Pass},
		{"T02", 6000, Low, 5000, false, Verify}, // 矩阵放行，超限升档
		{"T03", 6000, Mid, 50000, false, Verify},
		{"T04", 6000, High, 50000, false, Review},
		{"T05", 20000, Low, 10000, false, Review}, // 矩阵二次验证，超限升档
		{"T06", 20000, High, 50000, false, Block},
		{"T07", 80000, Low, 50000, false, Review},
		{"T08", 12000, Critical, 50000, false, Block},
		{"T09", 300000, Low, 500000, false, Block}, // 硬上限
		{"T10", 100, Low, 50000, true, Block},      // 名单
	}

	fmt.Println("金额区间 × 风险等级 三级联动决策（表驱动，10 组）")
	fmt.Printf("%-4s %-8s %-4s %-7s %-6s %-8s %-8s %s\n", "用例", "金额", "风险", "限额", "名单", "期望", "实际", "结果")
	pass := 0
	for _, c := range cases {
		got, reason := decide(c.amount, c.risk, c.limit, c.black)
		ok := got == c.want
		if ok {
			pass++
		}
		fmt.Printf("%-4s %-8d %-4s %-7d %-6v %-8s %-8s %v\n", c.id, c.amount, c.risk, c.limit, c.black, c.want, got, ok)
		if !ok {
			fmt.Println("    期望不符，原因：", reason)
		}
	}
	fmt.Printf("\n通过 %d/%d\n", pass, len(cases))

	fmt.Println("\n复核超时兜底（SLA = 15 分钟）")
	for _, c := range cases {
		a, _ := decide(c.amount, c.risk, c.limit, c.black)
		if a != Review {
			continue
		}
		got, why := resolveReview(c.amount, true)
		fmt.Printf("  %s %d 元 → 复核超时 → %s（%s）\n", c.id, c.amount, got, why)
	}

	fmt.Println("\n决策分布（10 组）")
	dist := map[Action]int{}
	for _, c := range cases {
		a, _ := decide(c.amount, c.risk, c.limit, c.black)
		dist[a]++
	}
	for _, a := range []Action{Pass, Verify, Review, Block} {
		fmt.Printf("  %-6s %d 组\n", a, dist[a])
	}
}
