// 68.1 一次决策的延迟预算：4 段（设备指纹/名单/规则/模型）+ 固定开销 7ms（参数校验 1 + 决策落库 6）。
// 每段用对数正态分布模拟长尾；对比「无分段 deadline」与「分段 deadline + 超时降级」，
// 以及模型劣化 3 倍时的 P50/P99 与超时率。固定随机种子，可复现。
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const (
	n          = 10000
	budget     = 100.0 // 端到端预算 ms
	fixedCost  = 7.0   // 参数校验 1 + 决策落库 6
	modelRetry = 3.0   // 模型超时后走规则兜底的额外耗时
)

type segment struct {
	name   string
	median float64
	sigma  float64
	limit  float64 // 分段 deadline（预算）
	action string
}

var segs = []segment{
	{"设备指纹", 2.0, 0.50, 8, "用缓存指纹 + 标记 feature_missing"},
	{"名单", 1.2, 0.40, 5, "按未命中处理 + 告警，大额转人工"},
	{"规则", 3.0, 0.50, 12, "跳过规则，模型分兜底"},
	{"模型", 22.0, 0.30, 60, "规则兜底（+3ms）"},
}

func lnorm(med, sigma float64, r *rand.Rand) float64 {
	return med * math.Exp(sigma*r.NormFloat64())
}

func pct(xs []float64, p float64) float64 {
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	idx := int(math.Ceil(p/100*float64(len(ys)))) - 1
	if idx < 0 {
		idx = 0
	}
	return ys[idx]
}

type result struct {
	name      string
	p50, p99  float64
	timeoutRt float64
	degradeRt float64
	modelTO   float64
}

// run 对同一批原始延迟应用不同配置：deadline 是否生效、模型是否劣化。
func run(name string, raw [][4]float64, deadline bool) result {
	totals := make([]float64, 0, n)
	timeouts, degraded, modelTO := 0, 0, 0
	for _, r := range raw {
		total := fixedCost
		for i, v := range r {
			if deadline && v > segs[i].limit {
				degraded++
				if i == 3 {
					modelTO++
					total += segs[i].limit + modelRetry
				} else {
					total += segs[i].limit
				}
				continue
			}
			total += v
		}
		if total > budget {
			timeouts++
		}
		totals = append(totals, total)
	}
	return result{
		name: name, p50: pct(totals, 50), p99: pct(totals, 99),
		timeoutRt: 100 * float64(timeouts) / n,
		degradeRt: 100 * float64(degraded) / n,
		modelTO:   100 * float64(modelTO) / n,
	}
}

func main() {
	r := rand.New(rand.NewSource(42))
	base := make([][4]float64, n) // 基线原始延迟
	bad := make([][4]float64, n)  // 模型劣化 3 倍
	for i := 0; i < n; i++ {
		for j, s := range segs {
			base[i][j] = lnorm(s.median, s.sigma, r)
			bad[i][j] = base[i][j]
		}
		bad[i][3] = base[i][3] * 3
	}

	fmt.Println("=== 基线各段延迟分布（n=10,000 次决策，固定种子 42；本机仿真） ===")
	fmt.Printf("%-8s %8s %8s %8s %10s  %s\n", "段", "P50", "P99", "预算", "P99/预算", "超时降级动作")
	for j, s := range segs {
		xs := make([]float64, n)
		for i := 0; i < n; i++ {
			xs[i] = base[i][j]
		}
		p50, p99 := pct(xs, 50), pct(xs, 99)
		fmt.Printf("%-8s %8.1f %8.1f %8.0f %9.0f%%  %s\n", s.name, p50, p99, s.limit, 100*p99/s.limit, s.action)
	}
	fmt.Printf("%-8s %8s %8s %8s %10s  %s\n", "固定开销", "7.0", "7.0", "—", "—", "参数校验 1 + 决策落库 6（同步最小记录）")
	sumP99 := 7.0
	for j := range segs {
		xs := make([]float64, n)
		for i := 0; i < n; i++ {
			xs[i] = base[i][j]
		}
		sumP99 += pct(xs, 99)
	}
	fmt.Printf("\n各段 P99 之和 + 固定开销 = %.1fms；总预算 %.0fms；判据「≤ 预算×0.8」= %.0fms → %v\n",
		sumP99, budget, budget*0.8, boolToStr(sumP99 <= budget*0.8))
	budgets := 7.0
	for _, s := range segs {
		budgets += s.limit
	}
	fmt.Printf("各段预算之和 + 固定开销 = %.1fms；判据「≤ 总预算」= %.0fms → %v\n",
		budgets, budget, boolToStr(budgets <= budget))

	baseNo := run("A 基线·无分段deadline", base, false)
	baseYes := run("B 基线·分段deadline", base, true)
	badNo := run("C 模型×3·无分段deadline", bad, false)
	badYes := run("D 模型×3·分段deadline", bad, true)
	all := []result{baseNo, baseYes, badNo, badYes}
	fmt.Println("\n=== 端到端：P50 / P99 / >100ms 超时率 / 分段降级率 ===")
	fmt.Printf("%-24s %7s %7s %9s %11s %10s\n", "场景", "P50", "P99", "超时率", "分段降级率", "其中模型超时")
	for _, x := range all {
		fmt.Printf("%-24s %6.1f %7.1f %8.2f%% %10.1f%% %9.1f%%\n", x.name, x.p50, x.p99, x.timeoutRt, x.degradeRt, x.modelTO)
	}
	fmt.Println("\n(时间单位 ms；超时率 = 端到端 > 100ms 的比例；分段降级率 = 至少一段超其 deadline 的比例)")
}

func boolToStr(b bool) string {
	if b {
		return "满足"
	}
	return "不满足"
}
