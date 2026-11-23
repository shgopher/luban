// 接口人数量与响应时间模型：本机仿真，不是线上实测。
// 模型把「接口人机制」拆成三段时间，任何一段都能单独推翻结论：
//  1. 基础处理 B：单件请求本身的评审/验收工时；
//  2. 排队等待 W：每个接口人是一个单服务台（M/M/1），ρ=λ/μ 越接近 1，等待爆炸；
//  3. 跨接口人协调 p*C：请求涉及提供方与消费方两方，落在不同接口人手里的概率 (N-1)/N。
//
// 直接对接：提供方只留 s 比例工时接受咨询，按 M/M/1 折算单件总等待 ≈ B/s。
// 总成本 = 接口人 FTE + 等待造成的阻塞成本 R*T*e（e 为每件请求多等 1 天折算的人天）。
package main

import (
	"fmt"
	"math"
)

type model struct {
	name     string
	H        float64 // 每个接口人每天的请求处理能力（件/天）
	B        float64 // 单件基础处理时长（天）
	C        float64 // 跨接口人协调一次的额外延迟（天）
	D        float64 // 文档自助可吸收的请求比例
	docDelay float64 // 自助查询耗时（天）
	S        float64 // 提供方留给直接咨询的工时比例
	L        float64 // 直接对接找错人的平均成本（天）
	e        float64 // 等待成本：每件请求多等 1 天，消费方折算损失的人天
	fteIface float64 // 一个接口人折合 FTE
	fteDoc   float64 // 文档维护折合 FTE
	maxN     int
}

// ifaceDelay 返回 N 个接口人时的平均响应时间（天）；利用率 >= 1 视为不可行。
func ifaceDelay(m model, n int, req float64) (float64, float64, bool) {
	lambda := req / float64(n)
	rho := lambda / m.H
	if rho >= 1 { // 到达率超过服务能力，队列无界，本题无有限解
		return math.Inf(1), rho, false
	}
	wait := rho / (m.H - lambda)        // M/M/1 平均排队等待（天）
	pCross := float64(n-1) / float64(n) // 两方请求落在不同接口人手里的概率
	t := m.B + wait + pCross*m.C
	return t, rho, true
}

// bestIface 在 1..maxN 里找总成本（FTE + 阻塞成本）最小的接口人数。
func bestIface(m model, req float64) (n int, t, rho, cost float64, ok bool) {
	cost = math.Inf(1)
	for i := 1; i <= m.maxN; i++ {
		ti, ri, feasible := ifaceDelay(m, i, req)
		if !feasible {
			continue
		}
		ci := m.fteIface*float64(i) + req*ti*m.e
		if ci < cost {
			n, t, rho, cost, ok = i, ti, ri, ci, true
		}
	}
	return
}

// bestDelay 找响应时间最短的接口人数（只看延迟，不看成本）。
func bestDelay(m model, req float64) (n int, t float64) {
	t = math.Inf(1)
	for i := 1; i <= m.maxN; i++ {
		ti, _, feasible := ifaceDelay(m, i, req)
		if feasible && ti < t {
			n, t = i, ti
		}
	}
	return
}

// docMode 文档自助 + 剩余请求仍由接口人兜底（人数也取成本最优）。
func docMode(m model, req float64) (n int, t, cost float64, ok bool) {
	remain := req * (1 - m.D)
	n, tRemain, _, _, ok := bestIface(m, remain)
	if !ok {
		return 0, math.Inf(1), math.Inf(1), false
	}
	t = m.D*m.docDelay + (1-m.D)*tRemain
	cost = m.fteIface*float64(n) + m.fteDoc + req*t*m.e
	return n, t, cost, true
}

// directDelay 提供方留 s 工时接受咨询时的单件总等待：B/s + 找错人成本。
func directDelay(m model) float64 { return m.B/m.S + m.L }

func row(m model, req float64) string {
	n, t, rho, cost, ok := bestIface(m, req)
	if !ok {
		return fmt.Sprintf("不可行")
	}
	return fmt.Sprintf("N*=%d T=%.2f天 ρ=%.2f 成本=%.2f人天/天", n, t, rho, cost)
}

func main() {
	base := model{H: 1.6, B: 0.15, C: 1.0, D: 0.6, docDelay: 0.05,
		S: 0.10, L: 0.25, e: 2.0, fteIface: 0.4, fteDoc: 0.1, maxN: 8}
	fmt.Println("== 接口人数量 N 与平均响应时间（天），三组负载参数 ==")
	fmt.Printf("参数：H=%.2f件/天 B=%.2f天 C=%.2f天 e=%.1f人天/天/件；N=1..8\n", base.H, base.B, base.C, base.e)
	for _, req := range []float64{0.3, 1.2, 3.6} {
		fmt.Printf("\n[负载 R=%.2f 件/天] ", req)
		for n := 1; n <= 8; n++ {
			t, rho, ok := ifaceDelay(base, n, req)
			if !ok {
				fmt.Printf("N=%d:不可行 ", n)
				continue
			}
			fmt.Printf("N=%d:%.2f ", n, t)
			_ = rho
		}
		delayN, delayT := bestDelay(base, req)
		fmt.Printf("\n  → 延迟最优 N=%d（%.2f天）；成本最优 %s\n", delayN, delayT, row(base, req))
		if dn, dt, dc, dok := docMode(base, req); dok {
			fmt.Printf("  → 文档自助模式：接口人 N=%d，T=%.2f天，成本=%.2f人天/天\n", dn, dt, dc)
		}
		direct := directDelay(base)
		fmt.Printf("  → 直接对接：T=%.2f天，成本=%.2f人天/天\n", direct, req*direct*base.e)
	}

	fmt.Println("\n== 基线 R=1.20 件/天 下四种模式对比 ==")
	req := 1.2
	n1t, _, _ := ifaceDelay(base, 1, req)
	n2t, _, _ := ifaceDelay(base, 2, req)
	dn, dt, dc, _ := docMode(base, req)
	_, _, _, c1, _ := bestIface(base, req)
	fmt.Printf("单接口人 N=1      T=%.2f天  成本=%.2f人天/天\n", n1t, base.fteIface+req*n1t*base.e)
	fmt.Printf("双接口人 N=2      T=%.2f天  成本=%.2f人天/天\n", n2t, 2*base.fteIface+req*n2t*base.e)
	fmt.Printf("接口人+文档 N=%d   T=%.2f天  成本=%.2f人天/天\n", dn, dt, dc)
	fmt.Printf("直接对接          T=%.2f天  成本=%.2f人天/天\n", directDelay(base), req*directDelay(base)*base.e)
	fmt.Printf("（参考：成本最优接口人模式 %.2f 人天/天）\n", c1)

	fmt.Println("\n== 取消接口人的回本点：等待成本 e 三档，扫描 R ==")
	for _, e := range []float64{0.5, 1.0, 2.0} {
		m := base
		m.e = e
		cross := math.NaN()
		for req := 0.01; req <= 3.0; req += 0.01 {
			_, _, _, costIf, ok := bestIface(m, req)
			if !ok {
				continue
			}
			if dcost := req * directDelay(m) * m.e; dcost < math.Min(costIf, docCost(m, req)) {
				cross = req
			} else if !math.IsNaN(cross) {
				break
			}
		}
		fmt.Printf("e=%.1f 人天/天/件：R < %.2f 件/天（约 %.1f 件/周）时，直接对接总成本最低\n", e, cross, cross*5)
	}

	fmt.Println("\n== 敏感性：文档自助吸收率 D 下降后，接口人+文档是否仍划算（R=1.2, e=2）==")
	for _, d := range []float64{0.6, 0.4, 0.2} {
		m := base
		m.D = d
		_, t, c, _ := docMode(m, 1.2)
		fmt.Printf("D=%.1f：T=%.2f天 成本=%.2f人天/天\n", d, t, c)
	}
	fmt.Println("\n环境：go1.27.1 darwin/arm64（Apple M1）；本机计算，非线上实测")
}

func docCost(m model, req float64) float64 {
	_, _, c, ok := docMode(m, req)
	if !ok {
		return math.Inf(1)
	}
	return c
}
