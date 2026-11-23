// 68.3 特征新鲜度 vs 决策质量：
// 1) 消融：离线 / +实时 / +准实时三档特征对 AUC 与拦截率(FPR=1%) 的影响；
// 2) 决策等待预算 W=10/30/80ms 时，准实时特征的就绪覆盖率、有效 AUC、拦截率、端到端 P99；
// 3) 对照：把准实时特征预计算成“读”（2ms），不再等待。
// 20,000 条样本、欺诈率 2%、固定种子；本机仿真，非线上实测。
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const (
	n      = 100000
	fraudZ = 2.0537 // P(z > 2.0537) ≈ 2%
	fixed  = 7.0    // 规则 5ms + 决策 2ms
)

// 特征观测：共同难点 u（模型无法消除的个体差异）+ 各自噪声 ε。
// 实时特征噪声最小，准实时次之，离线最大；三档都观测同一个 latent z。
type sample struct {
	z            float64
	fraud        bool
	off, rt, nrt float64
	nrtLat       float64 // 准实时特征的就绪耗时（现算）
	rtLat        float64
}

func lnorm(med, sigma float64, r *rand.Rand) float64 {
	return med * math.Exp(sigma*r.NormFloat64())
}

// AUC：Mann-Whitney，按分数排序后对正样本取平均秩（处理并列）。
func auc(scores []float64, fraud []bool) float64 {
	idx := make([]int, len(scores))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return scores[idx[a]] < scores[idx[b]] })
	ranks := make([]float64, len(scores))
	i := 0
	for i < len(idx) {
		j := i
		for j+1 < len(idx) && scores[idx[j+1]] == scores[idx[i]] {
			j++
		}
		avg := float64(i+j)/2.0 + 1
		for k := i; k <= j; k++ {
			ranks[idx[k]] = avg
		}
		i = j + 1
	}
	var nPos, nNeg int
	var sumRankPos float64
	for k := range scores {
		if fraud[k] {
			nPos++
			sumRankPos += ranks[k]
		} else {
			nNeg++
		}
	}
	return (sumRankPos - float64(nPos*(nPos+1))/2) / (float64(nPos) * float64(nNeg))
}

// 拦截率：固定误杀率 FPR=1%，取正常样本分数的 99 分位为阈值，看能拦住多少欺诈。
func recallAtFPR(scores []float64, fraud []bool, fpr float64) float64 {
	var pos, neg []float64
	for i, f := range fraud {
		if f {
			pos = append(pos, scores[i])
		} else {
			neg = append(neg, scores[i])
		}
	}
	sort.Float64s(neg)
	idx := int(math.Ceil((1-fpr)*float64(len(neg)))) - 1
	thr := neg[idx]
	var hit int
	for _, s := range pos {
		if s > thr {
			hit++
		}
	}
	return 100 * float64(hit) / float64(len(pos))
}

func combine(samples []sample, useRT, useNRT bool) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		sum, k := s.off, 1.0
		if useRT {
			sum += s.rt
			k++
		}
		if useNRT {
			sum += s.nrt
			k++
		}
		out[i] = sum / k
	}
	return out
}

func pct(xs []float64, p float64) float64 {
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	idx := int(math.Ceil(p/100*float64(len(ys)))) - 1
	return ys[idx]
}

func main() {
	r := rand.New(rand.NewSource(7))
	samples := make([]sample, n)
	for i := range samples {
		z := r.NormFloat64()
		u := 1.2 * r.NormFloat64() // 共同难点：三档特征都带着它
		samples[i] = sample{
			z:      z,
			fraud:  z > fraudZ,
			off:    z + u + 1.19*r.NormFloat64(),
			rt:     z + u + 1.62*r.NormFloat64(),
			nrt:    z + u + 1.85*r.NormFloat64(),
			rtLat:  lnorm(2.0, 0.40, r),
			nrtLat: lnorm(12.0, 0.80, r),
		}
	}
	fraud := make([]bool, n)
	nf := 0
	for i, s := range samples {
		fraud[i] = s.fraud
		if s.fraud {
			nf++
		}
	}

	fmt.Printf("=== 消融：三档特征对排序质量的影响（%d 条，欺诈率 %.2f%%；本机仿真） ===\n", n, 100*float64(nf)/float64(n))
	fmt.Printf("%-22s %8s %14s\n", "特征集合", "AUC", "拦截率@FPR1%")

	offScore := combine(samples, false, false)
	rtScore := combine(samples, true, false)
	fullScore := combine(samples, true, true)
	aL := []struct {
		name string
		s    []float64
	}{{"仅离线(off)", offScore}, {"离线+实时(off+rt)", rtScore}, {"全量(off+rt+nrt)", fullScore}}
	var prevAUC, prevRec float64
	for i, x := range aL {
		a, rec := auc(x.s, fraud), recallAtFPR(x.s, fraud, 0.01)
		if i == 0 {
			fmt.Printf("%-22s %8.3f %13.1f%%\n", x.name, a, rec)
		} else {
			fmt.Printf("%-22s %8.3f %13.1f%%   (ΔAUC=%+.3f, Δ拦截=%+.1fpp)\n", x.name, a, rec, a-prevAUC, rec-prevRec)
		}
		prevAUC, prevRec = a, rec
	}

	fmt.Println("\n=== 决策等待预算 W：覆盖率 / 有效 AUC / 拦截率 / 端到端延迟 ===")
	fmt.Printf("%-9s %9s %9s %14s %8s %8s\n", "W", "覆盖率", "有效AUC", "拦截率@FPR1%", "P50", "P99")
	for _, w := range []float64{10, 30, 80} {
		scores := make([]float64, n)
		lats := make([]float64, n)
		ready := 0
		for i, s := range samples {
			hasNRT := s.nrtLat <= w
			if hasNRT {
				ready++
			}
			sum, k := s.off+s.rt, 2.0
			if hasNRT {
				sum += s.nrt
				k++
			}
			scores[i] = sum / k
			wait := s.nrtLat
			if wait > w {
				wait = w
			}
			lats[i] = wait + fixed
		}
		coverage := 100 * float64(ready) / float64(n)
		fmt.Printf("W=%-7.0f %8.1f%% %9.3f %13.1f%% %7.1f %8.1f\n",
			w, coverage, auc(scores, fraud), recallAtFPR(scores, fraud, 0.01), pct(lats, 50), pct(lats, 99))
	}

	// 对照：准实时特征预计算成“读”（2ms 就绪），不再把 12–80ms 的现算放进请求。
	preLat := 2.0
	lats := make([]float64, n)
	for i, s := range samples {
		wait := s.rtLat
		if wait < preLat {
			wait = preLat
		}
		lats[i] = wait + fixed
	}
	fmt.Printf("%-9s %8.1f%% %9.3f %13.1f%% %7.1f %8.1f  ← 预计算：准实时特征按“读”处理\n",
		"预计算", 100.0, auc(fullScore, fraud), recallAtFPR(fullScore, fraud, 0.01), pct(lats, 50), pct(lats, 99))
	fmt.Printf("\n(时间单位 ms；覆盖率 = 准实时特征在 W 内就绪的比例；欺诈样本 %d 条/%d)\n", nf, n)
}
