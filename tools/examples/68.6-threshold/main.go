package main

// 68.6 阈值该定在哪：把总成本写成阈值的函数，扫一遍找最低点，
// 再用解析判据 f_bad/f_good = (N_good·C_fp)/(N_bad·C_fn) 复核，
// 最后看成本比变化时阈值与通过率怎么动。
// 分数分布为假设值（本机仿真），成本单价按 20:300 的量级设置。

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const (
	goodN    = 100_000 // 好用户
	badN     = 2_000   // 坏用户（先验 2%）
	muG, sdG = 0.35, 0.18
	muB, sdB = 0.72, 0.16
	cfp      = 20.0  // 误杀单价
	cfn      = 300.0 // 漏杀单价（基准 15 倍）
)

// draw 生成一批分数并排序，后续用二分统计 FP/FN
func draw(rng *rand.Rand, n int, mu, sd float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = rng.NormFloat64()*sd + mu
	}
	sort.Float64s(s)
	return s
}

// fpAt 分数 ≥ t 的好用户数（误杀）
func fpAt(sorted []float64, t float64) int {
	return len(sorted) - sort.SearchFloat64s(sorted, t)
}

// fnAt 分数 < t 的坏用户数（漏杀）
func fnAt(sorted []float64, t float64) int {
	return sort.SearchFloat64s(sorted, t)
}

// evaluate 给定阈值与单价，返回 (FP, FN, 通过率, 总成本)
func evaluate(good, bad []float64, t, fpCost, fnCost float64) (int, int, float64, float64) {
	fp := fpAt(good, t)
	fn := fnAt(bad, t)
	passed := (goodN - fp) + fn // 放行 = 好人放行 + 坏人放行
	passRate := float64(passed) / float64(goodN+badN)
	cost := float64(fp)*fpCost + float64(fn)*fnCost
	return fp, fn, passRate, cost
}

// bestThreshold 以 0.001 为步长扫 [0.01, 0.99]，返回成本最低的阈值
func bestThreshold(good, bad []float64, fpCost, fnCost float64) (float64, float64, int, int, float64) {
	bt, bc := 0.0, math.MaxFloat64
	var bfp, bfn int
	var bpass float64
	for t := 0.01; t <= 0.99; t += 0.001 {
		fp, fn, pass, cost := evaluate(good, bad, t, fpCost, fnCost)
		if cost < bc {
			bt, bc, bfp, bfn, bpass = t, cost, fp, fn, pass
		}
	}
	return bt, bc, bfp, bfn, bpass
}

// bestAccuracy 返回准确率最高的阈值，用来对照"按准确率选阈值"的代价
func bestAccuracy(good, bad []float64) (float64, float64) {
	bt, bacc := 0.0, 0.0
	for t := 0.01; t <= 0.99; t += 0.001 {
		fp := fpAt(good, t)
		fn := fnAt(bad, t)
		tp := badN - fn
		tn := goodN - fp
		acc := float64(tp+tn) / float64(goodN+badN)
		if acc > bacc {
			bt, bacc = t, acc
		}
	}
	return bt, bacc
}

// theoreticalRoot 用解析判据反解阈值：f_bad/f_good = RHS（二分）
func theoreticalRoot(fpCost, fnCost float64) float64 {
	rhs := float64(goodN) * fpCost / (float64(badN) * fnCost)
	ratio := func(t float64) float64 {
		fg := math.Exp(-(t-muG)*(t-muG)/(2*sdG*sdG)) / (sdG * math.Sqrt(2*math.Pi))
		fb := math.Exp(-(t-muB)*(t-muB)/(2*sdB*sdB)) / (sdB * math.Sqrt(2*math.Pi))
		return fb / fg
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		if ratio(mid) < rhs {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func main() {
	rng := rand.New(rand.NewSource(42))
	good := draw(rng, goodN, muG, sdG)
	bad := draw(rng, badN, muB, sdB)
	total := float64(goodN + badN)

	// ① 固定单价 20:300，扫阈值
	fmt.Printf("好用户=%d 坏用户=%d（先验 %.1f%%），误杀单价=%.0f，漏杀单价=%.0f\n\n",
		goodN, badN, 100*float64(badN)/total, cfp, cfn)
	fmt.Printf("%-8s %-8s %-8s %-9s %-10s\n", "阈值", "误杀FP", "漏杀FN", "通过率", "总成本")
	fmt.Println("-------------------------------------------------------")
	for _, t := range []float64{0.30, 0.40, 0.50, 0.60, 0.70, 0.80, 0.90} {
		fp, fn, pass, cost := evaluate(good, bad, t, cfp, cfn)
		fmt.Printf("%-8.2f %-8d %-8d %-9s %-10.0f\n", t, fp, fn, fmt.Sprintf("%.2f%%", pass*100), cost)
	}

	bt, bc, bfp, bfn, bpass := bestThreshold(good, bad, cfp, cfn)
	fmt.Printf("\n最低成本阈值=%.3f：误杀=%d 漏杀=%d 通过率=%.2f%% 总成本=%.0f\n",
		bt, bfp, bfn, bpass*100, bc)
	_, _, _, c50 := evaluate(good, bad, 0.50, cfp, cfn)
	fmt.Printf("对照：阈值 0.50 的成本=%.0f，多花 %.1f%%\n", c50, 100*(c50/bc-1))

	accT, acc := bestAccuracy(good, bad)
	_, _, _, costAcc := evaluate(good, bad, accT, cfp, cfn)
	fmt.Printf("准确率最高阈值=%.3f（准确率 %.2f%%），成本=%.0f，比最低成本高 %.1f%%\n",
		accT, acc*100, costAcc, 100*(costAcc/bc-1))

	root := theoreticalRoot(cfp, cfn)
	fmt.Printf("解析判据反解阈值=%.3f（f_bad/f_good = %.2f），与扫描结果相差 %.3f\n",
		root, float64(goodN)*cfp/(float64(badN)*cfn), math.Abs(root-bt))

	// ② 敏感度：漏杀单价变化时，最优阈值与通过率怎么动
	fmt.Printf("\n%-10s %-10s %-10s %-10s %-8s %-8s %-10s\n",
		"误杀单价", "漏杀单价", "倍数", "最优阈值", "通过率", "误杀", "总成本")
	fmt.Println("-----------------------------------------------------------------------------")
	for _, mult := range []float64{5, 10, 15, 30, 60} {
		fc := cfp * mult
		t, cost, fp, fn, pass := bestThreshold(good, bad, cfp, fc)
		fmt.Printf("%-10.0f %-10.0f %-10.0f %-10.3f %-9s %-8d %-10.0f\n",
			cfp, fc, mult, t, fmt.Sprintf("%.2f%%", pass*100), fp, cost)
		_ = fn
	}

	// ③ 单价估错 2 倍时，阈值偏多少
	tHalf, _, _, _, _ := bestThreshold(good, bad, cfp, cfn/2)
	tDouble, _, _, _, _ := bestThreshold(good, bad, cfp, cfn*2)
	fmt.Printf("\n单价估错的影响：漏杀单价 150 → 阈值 %.3f；300 → %.3f；600 → %.3f（单价翻倍阈值移动约 %.2f）\n",
		tHalf, bt, tDouble, math.Abs(tDouble-bt))
}
