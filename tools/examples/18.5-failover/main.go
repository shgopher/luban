// 18.5 心跳超时与误判率：蒙特卡洛仿真
// 模型：心跳按固定周期 H 发送；每条心跳的单程延迟 d = 1ms + Exp(均值 J)（重尾抖动）。
// 检测器在「距上一条心跳到达 > T」时判定疑似失联。
//   - 误判：节点活着，但两条心跳的到达间隔 H + d_i - d_{i-1} > T
//   - 检测延迟：节点真死时，从死亡时刻到判定时刻 = T - age，age ~ U(0,H)
//
// 运行：go run main.go（go1.27.1 / darwin/arm64 / Apple M1）
package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

const trials = 20_000_000 // 每组蒙特卡洛样本数

// 终端显示宽度：中文/全角标点按 2 列算
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

func expSample(r *rand.Rand, mean float64) float64 { return r.ExpFloat64() * mean }

// 一组参数：H=心跳周期(ms)，T=超时(ms)，J=抖动均值(ms)
func measure(h, t, j float64) (pFalse float64, faPerHour, latP50, latP99 float64) {
	r := rand.New(rand.NewSource(20261122))
	late := 0
	for i := 0; i < trials; i++ {
		dPrev, dCur := 1+expSample(r, j), 1+expSample(r, j)
		interval := h + dCur - dPrev
		if interval > t {
			late++
		}
	}
	pFalse = float64(late) / trials
	faPerHour = (3600 / (h / 1000)) * pFalse

	// 真实死亡：最后一跳之后死亡，age ~ U(0,H)；判定在 age 达到 T 时发生
	lat := make([]float64, 2_000_000)
	for i := range lat {
		age := r.Float64() * h
		lat[i] = math.Max(0, t-age)
	}
	sort.Float64s(lat)
	latP50, latP99 = lat[len(lat)/2], lat[len(lat)*99/100]
	return
}

// 由目标误报率反算超时：F = (3600/H_s) * 0.5 * exp(-(T-H)/J)
func recommendTimeout(h, j, fPerHour float64) (t, margin float64) {
	margin = j * math.Log(1800/((h/1000)*fPerHour))
	return h + margin, margin
}

func main() {
	fmt.Println("模型：H=500ms，单程延迟 = 1ms + Exp(均值 J=100ms)，连续检测；每组 2000 万次采样")
	fmt.Println("误判率按「每次心跳」统计；每小时误报 = (3600/H_s) × 误判率")
	fmt.Println()
	row([]string{"超时 T(ms)", "T/H", "每次心跳误判率", "每小时误报", "检测延迟 p50", "检测延迟 p99"},
		[]int{12, 6, 18, 14, 14, 14})
	for _, t := range []float64{700, 1000, 2000} {
		p, fa, p50, p99 := measure(500, t, 100)
		row([]string{fmt.Sprintf("%.0f", t), fmt.Sprintf("%.2f", t/500),
			fmt.Sprintf("%.2e", p), fmt.Sprintf("%.4g", fa),
			fmt.Sprintf("%.0fms", p50), fmt.Sprintf("%.0fms", p99)}, []int{12, 6, 18, 14, 14, 14})
	}

	fmt.Println("\n心跳周期扫描（固定余量 T−H = 500ms = 5J，误判率不变，只看延迟/流量/误报频次）")
	row([]string{"周期 H(ms)", "超时 T(ms)", "检测延迟均值", "心跳消息/秒/节点", "每小时误报"},
		[]int{12, 12, 14, 18, 14})
	for _, h := range []float64{200, 500, 1000} {
		p, _, p50, _ := measure(h, h+500, 100)
		fa := (3600 / (h / 1000)) * p
		row([]string{fmt.Sprintf("%.0f", h), fmt.Sprintf("%.0f", h+500),
			fmt.Sprintf("%.0fms", p50), fmt.Sprintf("%.2f", 1000/h), fmt.Sprintf("%.4g", fa)},
			[]int{12, 12, 14, 18, 14})
	}

	fmt.Println("\n判据表反算：T = H + J·ln(1800/(H_s·F))，检测延迟均值 ≈ T − H/2")
	row([]string{"场景", "J(ms)", "H(ms)", "目标误报/h", "建议超时 T", "检测延迟均值"},
		[]int{26, 8, 8, 12, 14, 16})
	type sc struct {
		name    string
		j, h, f float64
	}
	for _, s := range []sc{
		{"同机房自动切主", 20, 200, 10},
		{"跨可用区自动切主", 100, 500, 1},
		{"跨地域（人工闸门）", 300, 1000, 0.1},
		{"只告警、不摘除", 300, 1000, 10},
		{"STONITH 自动断电", 150, 500, 0.01},
	} {
		t, _ := recommendTimeout(s.h, s.j, s.f)
		row([]string{s.name, fmt.Sprintf("%.0f", s.j), fmt.Sprintf("%.0f", s.h),
			fmt.Sprintf("%.2f", s.f), fmt.Sprintf("%.0fms", t), fmt.Sprintf("%.0fms", t-s.h/2)},
			[]int{26, 8, 8, 12, 14, 16})
	}
}
