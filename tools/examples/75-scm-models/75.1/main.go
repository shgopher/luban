// main.go —— go1.27.1 darwin/arm64，go run main.go
package main

import (
	"fmt"
	"math"
	"math/rand"
)

// 需求：日均 100，周末 ×1.3，噪声 σ=12
func demand(n int, seed int64) []float64 {
	r := rand.New(rand.NewSource(seed))
	d := make([]float64, n)
	for i := range d {
		w := 1.0
		if i%7 >= 5 {
			w = 1.3
		}
		d[i] = 100*w + r.NormFloat64()*12
	}
	return d
}

// kind：ma=4 周移动平均，ses=指数平滑 α=0.3，sn=季节朴素（上周同日）
func forecast(d []float64, kind string) []float64 {
	f := make([]float64, len(d))
	for i := range d {
		switch {
		case kind == "ma" && i >= 4:
			f[i] = (d[i-1] + d[i-2] + d[i-3] + d[i-4]) / 4
		case kind == "ses" && i > 0:
			f[i] = 0.3*d[i-1] + 0.7*f[i-1]
		case kind == "sn" && i >= 7:
			f[i] = d[i-7]
		default:
			f[i] = d[0]
		}
	}
	return f
}

func score(d, f []float64) (m, b float64) {
	for i := 7; i < len(d); i++ { // 前 7 天预热，不评分
		m += math.Abs(d[i]-f[i]) / d[i]
		b += d[i] - f[i]
	}
	n := float64(len(d) - 7)
	return 100 * m / n, 100 * b / n
}

// kind=sQ：库存位置 ≤ s 时订 Q；kind=rs：每 R 天补到 S。lost sales 口径
func sim(kind string, s, Q, S float64, R int, d []float64, lt int) (fill, inv float64, stock, orders int) {
	onHand := 700.0
	onOrder := make([]float64, lt+1)
	last := 0
	for t := range d {
		onHand += onOrder[t%len(onOrder)]
		onOrder[t%len(onOrder)] = 0
		ship := math.Min(onHand, d[t])
		onHand -= ship
		if ship < d[t] {
			stock++
		}
		fill += ship
		ip := onHand + sum(onOrder) // 库存位置 = 在手 + 在途（本节 lost-sales，已分配=0）
		var place float64
		if kind == "sQ" && ip <= s {
			place = Q
		}
		if kind == "rs" && t-last >= R {
			last = t
			place = S - ip
		}
		if place > 0 {
			onOrder[(t+lt)%len(onOrder)] += place
			orders++
		}
		inv += onHand
	}
	return 100 * fill / sum(d), inv / float64(len(d)), stock, orders
}

func sum(a []float64) float64 {
	s := 0.0
	for _, v := range a {
		s += v
	}
	return s
}

// 4 级链：每级 order-up-to + 12 期移动平均，提前期 2；shared 时各级都看末端需求
func chain(shared bool) []float64 {
	const tiers, n, lt, win = 4, 600, 2, 12
	r := rand.New(rand.NewSource(7))
	end := make([]float64, n)
	for i := range end {
		end[i] = math.Max(0, 100+r.NormFloat64()*12)
	}
	dem := append([]float64(nil), end...)
	out := make([]float64, tiers)
	for k := 0; k < tiers; k++ {
		orders := make([]float64, n)
		ip := 100.0 * (lt + 1)
		for t := 0; t < n; t++ {
			ip -= dem[t]
			base := dem[:t+1]
			if shared && k > 0 {
				base = end[:t+1]
			}
			hat := 100.0
			if t >= win {
				hat = sum(base[t+1-win:t+1]) / win
			}
			q := math.Max(0, hat*float64(lt+1)+1.65*12*math.Sqrt(lt+1)-ip)
			orders[t] = q
			ip += q
		}
		out[k] = variance(orders[50:]) / variance(end[50:])
		dem = orders
	}
	return out
}

func variance(a []float64) float64 {
	m, s := 0.0, 0.0
	for _, v := range a {
		m += v
	}
	m /= float64(len(a))
	for _, v := range a {
		s += (v - m) * (v - m)
	}
	return s / float64(len(a))
}

func main() {
	d := demand(365, 42)
	meanD, sdD := sum(d[:200])/200, 0.0
	for _, v := range d[:200] {
		sdD += (v - meanD) * (v - meanD)
	}
	sdD = math.Sqrt(sdD / 200)
	fmt.Printf("需求序列：365 天，前 200 天均值 %.1f 件/天，标准差 %.1f 件/天\n\n", meanD, sdD)

	fmt.Println("预测方法（提前 1 天，358 个评分点）：")
	for _, k := range []string{"ma", "ses", "sn"} {
		m, b := score(d, forecast(d, k))
		fmt.Printf("  %-4s MAPE %5.1f%%   偏差 %+5.1f%%\n", k, m, b)
	}

	lt, z := 14, 1.65
	s := meanD*float64(lt) + z*sdD*math.Sqrt(float64(lt))
	S7 := meanD*float64(7+lt) + z*sdD*math.Sqrt(float64(7+lt))
	S14 := meanD*float64(14+lt) + z*sdD*math.Sqrt(float64(14+lt))
	fmt.Printf("\n补货策略（提前期 %d 天，z=1.65；s=%.0f S(7)=%.0f S(14)=%.0f）：\n", lt, s, S7, S14)
	fmt.Printf("%-20s %-8s %-10s %-9s %s\n", "策略", "满足率", "平均库存", "缺货天数", "订货次数")
	for _, c := range []struct {
		name, kind string
		s, Q, S    float64
		R          int
	}{
		{"(s,Q) 每日检查", "sQ", s, 700, 0, 0},
		{"(R,S) 每 7 天", "rs", 0, 0, S7, 7},
		{"(R,S) 每 14 天", "rs", 0, 0, S14, 14},
	} {
		fill, inv, stock, orders := sim(c.kind, c.s, c.Q, c.S, c.R, d, lt)
		fmt.Printf("%-20s %5.1f%%  %6.0f 件  %5d 天   %d 次\n", c.name, fill, inv, stock, orders)
	}

	fmt.Println("\n牛鞭效应（4 级链，MA(12)，提前期 2）：")
	own, sh := chain(false), chain(true)
	prev := 1.0
	for k := 0; k < 4; k++ {
		inc := own[k] / prev
		fmt.Printf("  第 %d 级：本级 %.2fx  相对末端 %.2fx  共享信息后 %.2fx\n", k+1, inc, own[k], sh[k])
		prev = own[k]
	}
}
