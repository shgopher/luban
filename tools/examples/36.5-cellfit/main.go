// 单元化可行性算账：先算「不做会不会更亏」，再决定要不要动工。
//
// 它不设计单元、不给路由算法（那是正文 36.5 的事），只回答一个决策问题：
// 给定故障损失、跨单元调用比例、改造投入与 P99 预算，单元化这一刀该不该现在砍。
package main

import (
	"flag"
	"fmt"
	"math"
)

type input struct {
	users        float64 // 日活用户数
	cells        int     // 计划单元数
	crossPct     float64 // 跨单元请求占比（0–1）
	lossPerMin   float64 // 全站故障每分钟损失（元）
	rtoNowMin    float64 // 现状恢复时间（分钟）
	rtoNewMin    float64 // 单元隔离后切流时间（分钟）
	failsPerYear float64 // 年故障次数
	personMonths float64 // 改造人月
	pmCost       float64 // 人月成本（元）
	infraMonth   float64 // 现有基础设施月成本（元）
	infraInc     float64 // 单元化带来的基础设施增量比例
	hops         int     // 页面串联的跨单元调用数
	fastMs       float64 // 单跳快的耗时
	slowMs       float64 // 单跳慢的耗时（GC/锁/跨单元排队）
	slowProb     float64 // 单跳变慢的概率
	p99BudgetMs  float64 // 业务可接受的页面 P99
	crossWrite   bool    // 是否存在跨地域写冲突（单元化要解决的第一问题）
	opsReady     bool    // 是否已有单元级灰度与演练能力
}

type result struct {
	lossNowYear   float64 // 现状年故障损失
	lossCellYear  float64 // 单元化后年故障损失
	yearGain      float64 // 年减少的故障损失
	infraDeltaYr  float64 // 每年多花的增量基础设施
	netGainYear   float64 // 净年收益
	oneTimeCost   float64 // 一次性投入
	paybackMonths float64 // 回本月数
	pageP99       float64 // 跨单元调用后的页面 P99
	radiusPct     float64 // 单单元故障影响用户比例
	gates         []gate
	verdict       string
	reason        string
}

type gate struct {
	name    string
	ok      bool
	okMsg   string // 满足时的事实
	failMsg string // 不满足时的下一步
}

func compute(in input) result {
	var r result
	r.radiusPct = 100 / float64(in.cells)

	// 故障损失：影响比例 × 停机时长 × 每分钟损失 × 年故障次数
	r.lossNowYear = in.failsPerYear * in.rtoNowMin * in.lossPerMin
	cellShare := 1 / float64(in.cells)
	r.lossCellYear = in.failsPerYear * in.rtoNewMin * in.lossPerMin * cellShare
	r.yearGain = r.lossNowYear - r.lossCellYear

	r.infraDeltaYr = in.infraInc * in.infraMonth * 12
	r.netGainYear = r.yearGain - r.infraDeltaYr

	// 一次性投入：改造人月 + 迁移期双跑 3 个月的基础设施增量
	r.oneTimeCost = in.personMonths*in.pmCost + in.infraInc*in.infraMonth*3
	if r.netGainYear > 0 {
		r.paybackMonths = r.oneTimeCost / (r.netGainYear / 12)
	} else {
		r.paybackMonths = math.Inf(1)
	}

	// 页面 P99：串联 K 跳，只要有一跳变慢，整页就慢。
	// 全快概率 = (1-p)^K；它低于 99% 时，页面 P99 落在「有一跳慢」的那一档。
	allFast := math.Pow(1-in.slowProb, float64(in.hops))
	if allFast >= 0.99 {
		r.pageP99 = float64(in.hops) * in.fastMs
	} else {
		r.pageP99 = in.slowMs + float64(in.hops-1)*in.fastMs
	}

	r.gates = []gate{
		{"① 有真实的跨地域写冲突", in.crossWrite,
			"存在跨地域写冲突，且只有单元化能同时消掉冲突与隔离故障",
			"没有跨地域写冲突：先按 36.4 的成本曲线买同城双活，别为隔离付单元化的钱"},
		{"② 回本 ≤ 24 个月", r.paybackMonths <= 24,
			fmt.Sprintf("一次性 %.0f 万，净年收益 %.0f 万，回本 %.1f 个月", r.oneTimeCost/1e4, r.netGainYear/1e4, r.paybackMonths),
			fmt.Sprintf("回本 %.1f 个月（>24）：先投备份与演练，或只对核心交易做单元化，缩小改造面", r.paybackMonths)},
		{"③ 跨单元请求 < 90%", in.crossPct < 0.9,
			fmt.Sprintf("当前 %.1f%%（实测 4 单元 74.77%%，16 单元 93.68%%）", in.crossPct*100),
			fmt.Sprintf("当前 %.1f%% ≥ 90%%：改路由键或按关系聚类（同城/同商户/同群）打映射表，而不是继续加单元", in.crossPct*100)},
		{"④ 页面 P99 在预算内", r.pageP99 <= in.p99BudgetMs,
			fmt.Sprintf("%.0f ms / 预算 %.0f ms（%d 跳串联，单跳 %.0f%% 概率慢到 %.0f ms）",
				r.pageP99, in.p99BudgetMs, in.hops, in.slowProb*100, in.slowMs),
			fmt.Sprintf("%.0f ms 超预算 %.0f ms：降串联跳数（聚合层并发、本地缓存），把跨单元调用改成异步 + 最终一致",
				r.pageP99, in.p99BudgetMs)},
		{"⑤ 有单元级灰度与演练", in.opsReady,
			"能按单元灰度发布、能按单元做故障演练",
			"缺单元级灰度或演练：先补（36.7），否则故障只是换一种形态"},
	}

	for _, g := range r.gates {
		if !g.ok {
			r.verdict = "现在别做"
			r.reason = "卡在 " + g.name + " —— " + g.failMsg
			return r
		}
	}
	r.verdict = "可以做，按四步落地"
	r.reason = "五道门全过；先搬读流量，再搬写，切流前必须双写 + 双向校验"
	return r
}

func main() {
	in := input{}
	flag.Float64Var(&in.users, "users", 5e6, "日活用户数")
	flag.IntVar(&in.cells, "cells", 4, "计划单元数")
	flag.Float64Var(&in.crossPct, "cross", 0.75, "跨单元请求占比，0–1（实测 4 单元≈0.75）")
	flag.Float64Var(&in.lossPerMin, "loss-min", 20000, "全站故障每分钟损失（元）")
	flag.Float64Var(&in.rtoNowMin, "rto-now", 240, "现状恢复时间（分钟）")
	flag.Float64Var(&in.rtoNewMin, "rto-new", 5, "单元隔离后切流时间（分钟）")
	flag.Float64Var(&in.failsPerYear, "fails", 2, "年故障次数")
	flag.Float64Var(&in.personMonths, "pm", 60, "改造人月")
	flag.Float64Var(&in.pmCost, "pm-cost", 40000, "人月成本（元）")
	flag.Float64Var(&in.infraMonth, "infra", 2e5, "现有基础设施月成本（元）")
	flag.Float64Var(&in.infraInc, "infra-inc", 0.6, "基础设施增量比例")
	flag.IntVar(&in.hops, "hops", 3, "页面串联的跨单元调用数")
	flag.Float64Var(&in.fastMs, "fast", 5, "单跳快耗时 ms")
	flag.Float64Var(&in.slowMs, "slow", 100, "单跳慢耗时 ms")
	flag.Float64Var(&in.slowProb, "slowp", 0.01, "单跳变慢概率")
	flag.Float64Var(&in.p99BudgetMs, "p99", 300, "业务可接受的页面 P99 ms")
	flag.BoolVar(&in.crossWrite, "cross-write", true, "是否存在跨地域写冲突")
	flag.BoolVar(&in.opsReady, "ops", false, "是否已有单元级灰度与演练能力")
	flag.Parse()

	r := compute(in)

	fmt.Println("== 输入 ==")
	fmt.Printf("  日活 %.0f 万 / %d 单元 / 跨单元请求 %.0f%% / 年故障 %.1f 次\n",
		in.users/1e4, in.cells, in.crossPct*100, in.failsPerYear)
	fmt.Printf("  现状 RTO %.0f 分 → 单元隔离后 %.0f 分 / 每分钟损失 %.0f 元\n",
		in.rtoNowMin, in.rtoNewMin, in.lossPerMin)

	fmt.Println("\n== 爆炸半径与损失 ==")
	fmt.Printf("  单单元故障影响 %.1f%% 用户；年故障损失 %.0f 万 → %.0f 万（减少 %.0f 万）\n",
		r.radiusPct, r.lossNowYear/1e4, r.lossCellYear/1e4, r.yearGain/1e4)
	fmt.Printf("  增量基础设施 %.0f 万/年；净年收益 %.0f 万\n", r.infraDeltaYr/1e4, r.netGainYear/1e4)

	fmt.Println("\n== 跨单元调用的 P99 税 ==")
	fmt.Printf("  %d 跳串联 → 页面 P99 ≈ %.0f ms（预算 %.0f ms）\n", in.hops, r.pageP99, in.p99BudgetMs)

	fmt.Println("\n== 一次性投入与回本 ==")
	fmt.Printf("  改造 %.0f 人月 × %.0f 元 + 迁移期双跑 = %.0f 万\n", in.personMonths, in.pmCost, r.oneTimeCost/1e4)
	if math.IsInf(r.paybackMonths, 1) {
		fmt.Println("  回本：永远回不来（净年收益 ≤ 0）")
	} else {
		fmt.Printf("  回本：%.1f 个月\n", r.paybackMonths)
	}

	fmt.Println("\n== 五道门 ==")
	for _, g := range r.gates {
		mark := "✗"
		if g.ok {
			mark = "✓"
		}
		note := g.okMsg
		if !g.ok {
			note = g.failMsg
		}
		fmt.Printf("  %s %-22s %s\n", mark, g.name, note)
	}

	fmt.Println("\n== 结论 ==")
	fmt.Printf("  %s\n  理由：%s\n", r.verdict, r.reason)
}
