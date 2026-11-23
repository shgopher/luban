package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// 一单的四段耗时（小时）：揽收 → 干线 → 分拨 → 末端
type segs struct{ pick, line, hub, last float64 }

func (s segs) total() float64 { return s.pick + s.line + s.hub + s.last }

// 8% 的包裹遇异常：6 成发生在干线、4 成发生在分拨，额外 24-72h
func sampleShipment(r *rand.Rand) (segs, bool) {
	s := segs{
		pick: 4 + r.Float64()*8,
		line: 24 * (0.6 + r.Float64()*0.8),
		hub:  6 + r.Float64()*12,
		last: 8 + r.Float64()*22,
	}
	if r.Float64() < 0.08 {
		if r.Float64() < 0.6 {
			s.line += 24 + r.Float64()*48
		} else {
			s.hub += 24 + r.Float64()*48
		}
		return s, true
	}
	return s, false
}

func mean(a []float64) float64 {
	s := 0.0
	for _, v := range a {
		s += v
	}
	return s / float64(len(a))
}

func percentile(a []float64, p float64) float64 {
	b := append([]float64(nil), a...)
	sort.Float64s(b)
	i := int(math.Ceil(p/100*float64(len(b)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(b) {
		i = len(b) - 1
	}
	return b[i]
}

func main() {
	// 校准集：2000 单历史，取分段均值与分位数
	rc := rand.New(rand.NewSource(1))
	var cp, cl, ch, cls []float64
	for i := 0; i < 2000; i++ {
		s, _ := sampleShipment(rc)
		cp, cl, ch, cls = append(cp, s.pick), append(cl, s.line), append(ch, s.hub), append(cls, s.last)
	}
	meanPick, meanLine := mean(cp), mean(cl)
	medianHub, medianLast := percentile(ch, 50), percentile(cls, 50)

	// 测试集：800 单
	r := rand.New(rand.NewSource(2026))
	n := 800
	actual := make([]float64, n)
	p1, p2, p3 := make([]float64, n), make([]float64, n), make([]float64, n)
	anomaly := 0
	for i := 0; i < n; i++ {
		s, bad := sampleShipment(r)
		if bad {
			anomaly++
		}
		actual[i] = s.total()
		p1[i] = 96 // 对外固定 4 日达
		p2[i] = meanPick + meanLine + percentile(ch, 50) + percentile(cls, 50)
		// 到分拨中心：揽收 + 干线已发生（实际值），分拨 + 末端按中位数估
		p3[i] = s.pick + s.line + medianHub + medianLast
	}

	fmt.Printf("测试集 800 单：实际送达平均 %.1fh（P90 %.1fh），其中 %d 单（%.0f%%）遇异常延误\n",
		mean(actual), percentile(actual, 90), anomaly, 100*float64(anomaly)/float64(n))
	fmt.Printf("历史校准：揽收 %.1fh / 干线 %.1fh / 分拨中位 %.1fh / 末端中位 %.1fh\n\n",
		meanPick, meanLine, medianHub, medianLast)

	names := []string{"M1 固定 4 日达（96h）", "M2 下单时分段均值", "M3 分位初值 + 到分拨后更新"}
	preds := [][]float64{p1, p2, p3}
	fmt.Printf("%-28s %-8s %-9s %-13s %-14s %s\n",
		"方法", "MAE", "P90误差", "96h达成率", "95%达成的承诺", "超时预警召回")
	for i, p := range preds {
		var ae, overshoot []float64
		hit96, tp, fn := 0, 0, 0
		for k := 0; k < n; k++ {
			e := p[k] - actual[k]
			ae = append(ae, math.Abs(e))
			overshoot = append(overshoot, actual[k]-p[k]) // 实际比预测晚多少
			if actual[k] <= 96 {
				hit96++
			}
			if p[k] > 96 && actual[k] > 96 {
				tp++
			}
			if p[k] <= 96 && actual[k] > 96 {
				fn++
			}
		}
		// 要 95% 达成，承诺 = 平均预测 + 历史“实际-预测”的 P95 缓冲
		buffer := percentile(overshoot, 95)
		promise := mean(p) + buffer
		fmt.Printf("%-28s %5.1fh  %6.1fh    %3.0f%%          %6.1fh        %3.0f%%\n",
			names[i], mean(ae), percentile(ae, 90), 100*float64(hit96)/float64(n),
			promise, 100*float64(tp)/float64(tp+fn))
	}
}
