// 22.3 φ accrual：用正态尾概率近似，输入心跳间隔样本，输出 φ(t) 与阈值误判
// 模型：真实心跳间隔 ~ LogNormal(中位数 500ms, 对数标准差 0.3)（下界有界、上侧长尾）。
// φ(t) = -log10(P(间隔 > t | 正常))
//
//	≈ -log10(Q((t-μ)/σ))，Q(z)=0.5·erfc(z/√2)，μ/σ 由样本估计。
//
// 运行：go run main.go（go1.27.1 / darwin/arm64 / Apple M1）
package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

const (
	median = 500.0 // 真实间隔中位数 ms
	sigLog = 0.3   // 对数标准差
	nFit   = 10000 // 用于估计 μ/σ 的心跳间隔样本数
	nMC    = 5000000
)

func width(s string) int {
	w := 0
	for _, r := range s {
		if r > 0x2E7F {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func pad(s string, n int) string {
	if d := n - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func row(cells []string, widths []int) {
	line := ""
	for i, c := range cells {
		if i > 0 {
			line += "  "
		}
		line += pad(c, widths[i])
	}
	fmt.Println(line)
}

// 标准正态上尾：P(Z > z)
func upperTail(z float64) float64 { return 0.5 * math.Erfc(z/math.Sqrt2) }

// φ accrual：给定样本均值/标准差，把「距上次心跳已过 t 毫秒」映射成怀疑度
func phi(t, mu, sigma float64) float64 {
	p := upperTail((t - mu) / sigma)
	if p <= 0 {
		return math.Inf(1)
	}
	return -math.Log10(p)
}

// 反解：φ(t)=target 的时刻（φ 对 t 单调递增，二分即可）
func timeAtPhi(target, mu, sigma float64) float64 {
	lo, hi := 0.0, mu+40*sigma
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if phi(mid, mu, sigma) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

func main() {
	r := rand.New(rand.NewSource(20261122))

	// ① 输入：1 万条正常心跳的间隔样本（模拟网络 + GC 抖动造成的长尾）
	samples := make([]float64, nFit)
	sum := 0.0
	for i := range samples {
		samples[i] = median * math.Exp(sigLog*r.NormFloat64())
		sum += samples[i]
	}
	mu := sum / nFit
	ss := 0.0
	for _, v := range samples {
		ss += (v - mu) * (v - mu)
	}
	sigma := math.Sqrt(ss / float64(nFit-1))
	fmt.Printf("样本估计：n=%d  μ=%.1fms  σ=%.1fms（真实分布 LogNormal 中位数 %.0fms, σ_log=%.1f）\n\n",
		nFit, mu, sigma, median, sigLog)

	// ② φ(t) 随时间变化
	fmt.Println("φ 随时间（距上一条心跳到达的毫秒数 → 怀疑度）：")
	row([]string{"t(ms)", "500", "750", "1000", "1100", "1200", "1300", "1400", "1600", "2000"},
		[]int{7, 7, 7, 7, 7, 7, 7, 7, 7, 7})
	cells := []string{"φ(t)"}
	for _, t := range []float64{500, 750, 1000, 1100, 1200, 1300, 1400, 1600, 2000} {
		cells = append(cells, fmt.Sprintf("%.2f", phi(t, mu, sigma)))
	}
	row(cells, []int{7, 7, 7, 7, 7, 7, 7, 7, 7, 7})

	// ③ 三个阈值：判定时刻、名义误判、本机 MC 误判率、经验 φ、每小时误报、检测延迟
	trueSamples := make([]float64, nMC)
	for i := range trueSamples {
		trueSamples[i] = median * math.Exp(sigLog*r.NormFloat64())
	}
	fmt.Println("\n三个 φ 阈值对照（真实分布下的误判由 500 万次采样统计）")
	row([]string{"φ 阈值", "判定时刻", "名义误判 10^-φ", "本机 MC 误判率", "经验 φ", "每小时误报", "平均检测延迟"},
		[]int{8, 10, 16, 16, 8, 12, 14})
	for _, target := range []float64{3, 8, 12} {
		t := timeAtPhi(target, mu, sigma)
		late := 0
		for _, v := range trueSamples {
			if v > t {
				late++
			}
		}
		emp := float64(late) / nMC
		faPerHour := 7200 * emp // 心跳 2 次/秒
		row([]string{
			fmt.Sprintf("%.0f", target),
			fmt.Sprintf("%.0fms", t),
			fmt.Sprintf("%.0e", math.Pow(10, -target)),
			fmt.Sprintf("%.2e", emp),
			fmt.Sprintf("%.2f", -math.Log10(emp)),
			fmt.Sprintf("%.4g", faPerHour),
			fmt.Sprintf("%.0fms", t-median/2),
		}, []int{8, 10, 16, 16, 8, 12, 14})
	}

	// ④ 敏感度：μ/σ 的估计误差会怎样放大 φ（在 φ=8 的判定时刻上重算）
	t8 := timeAtPhi(8, mu, sigma)
	fmt.Printf("\n敏感度（在 φ=8 的判定时刻 %.0fms 上重算）：\n", t8)
	fmt.Printf("  σ 高估 10%%：φ = %.2f（从 8 掉到 %.2f）\n", phi(t8, mu, sigma*1.10), phi(t8, mu, sigma*1.10))
	fmt.Printf("  μ 高估 10ms：φ = %.2f\n", phi(t8, mu+10, sigma))
	fmt.Printf("  μ 低估 10ms：φ = %.2f\n", phi(t8, mu-10, sigma))
	fmt.Printf("  z 值：φ=3 → %.3f；φ=8 → %.3f；φ=12 → %.3f\n",
		invTail(3), invTail(8), invTail(12))
}

// 反解标准正态上尾：Q(z)=10^-phi
func invTail(phiV float64) float64 {
	lo, hi := 0.0, 40.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if upperTail(mid) > math.Pow(10, -phiV) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}
