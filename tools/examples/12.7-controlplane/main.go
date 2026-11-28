package main

import (
	"fmt"
	"math"
)

// 三个量决定控制面形态：
//   ① 集中式：单点吞吐 = 1/服务时间，排队延迟随利用率爆炸（M/M/1）
//   ② 两层：offer 在“框架思考”的 τ 秒内可能过期，过期率 = 1 - e^(-λτ)
//   ③ 共享状态：k 个调度器并发决策，两两冲突的概率随 λ、τ 上升

func mm1(util float64) float64 { // 返回“响应时间 / 服务时间”
	if util >= 1 {
		return math.Inf(1)
	}
	return 1 / (1 - util)
}

func staleProb(lambda, tau float64) float64 { return 1 - math.Exp(-lambda*tau) }

func conflictProb(k int, lambda, tau float64) float64 {
	pairs := float64(k * (k - 1) / 2)
	// 每一对在窗口 τ 内至少发生一次状态变更的概率，近似为 1-e^(-2λτ)
	p := 1 - math.Exp(-2*lambda*tau)
	return 1 - math.Pow(1-p, pairs)
}

func main() {
	fmt.Println("=== 集中式：M/M/1 的排队延迟（单位：服务时间的倍数）===")
	fmt.Printf("%-10s %-14s %-14s\n", "利用率 ρ", "平均排队+服务", "P99 近似")
	for _, rho := range []float64{0.5, 0.8, 0.9, 0.95, 0.99} {
		// P99 用 -ln(0.01)/(1-ρ) 的指数分布尾近似
		p99 := math.Log(100) / (1 - rho)
		fmt.Printf("%-10.2f %-14.1f %-14.1f\n", rho, mm1(rho), p99)
	}
	fmt.Println("→ 利用率 90% 时，平均延迟已是空载的 10 倍；99% 时是 100 倍")

	fmt.Println("\n=== 两层调度：offer 过期率 = 1 - e^(-λτ) ===")
	fmt.Printf("%-10s %-8s %-10s %-10s %-10s\n", "状态变化λ/s", "τ=0.5s", "τ=1s", "τ=2s", "τ=5s")
	for _, lambda := range []float64{1, 5, 10, 20} {
		fmt.Printf("%-10.0f %-8.2f %-10.2f %-10.2f %-10.2f\n",
			lambda, staleProb(lambda, 0.5), staleProb(lambda, 1), staleProb(lambda, 2), staleProb(lambda, 5))
	}
	fmt.Println("→ 1 次/秒的变更、思考 2 秒，86% 的 offer 到手就过期；10 次/秒时几乎必然过期")

	fmt.Println("\n=== 共享状态：k 个调度器一轮决策里“至少一次冲突”的概率（λ=10/s, τ=5ms）===")
	fmt.Printf("%-6s %-12s\n", "k", "冲突概率")
	for _, k := range []int{2, 4, 8, 16, 32} {
		fmt.Printf("%-6d %-12.3f\n", k, conflictProb(k, 10, 0.005))
	}
	fmt.Println("→ 冲突率随 k² 增长；重试能救正确性，但吞吐会被冲突吃掉")

	fmt.Println("\n=== 分片：把“一个集中式”变成 N 个“小集中式” ===")
	fmt.Printf("%-8s %-14s %-14s %-12s\n", "分片数", "总吞吐(相对)", "单分片负载", "跨分片事务占比")
	for _, n := range []int{1, 2, 4, 8, 16} {
		cross := 1 - 1/float64(n) // 简化：随机分片时跨分片比例
		fmt.Printf("%-8d %-14.1f %-14.2f %-12.2f\n", n, float64(n), 1/float64(n), cross)
	}
}
