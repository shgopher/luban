// 25.3 版本向量 vs LWW：3 副本并发写同一 key 的检测能力与丢更新
// ① 固定时间线（可手工复核）：3 副本、R1 分区、R2→R3 因果、时钟偏移下的 LWW
// ② 蒙特卡洛：随机分区 + 时钟偏移，统计 VV 检出并发 / LWW 静默丢写 / LWW 因果逆序
// 运行：go run main.go（go1.27.1 / darwin/arm64 / Apple M1）
package main

import (
	"fmt"
	"math/rand"
	"strings"
)

const replicas = 3

// ---------- 终端排版 ----------
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

// ---------- 版本向量 ----------
type vv [replicas]int

func (a vv) leq(b vv) bool { // a 是 b 的祖先或相等
	for i := range a {
		if a[i] > b[i] {
			return false
		}
	}
	return true
}
func (a vv) join(b vv) vv {
	var c vv
	for i := range a {
		if a[i] > b[i] {
			c[i] = a[i]
		} else {
			c[i] = b[i]
		}
	}
	return c
}
func (a vv) String() string { return fmt.Sprintf("%v", [replicas]int(a)) }

type version struct {
	replica int
	value   string
	vv      vv
	stamp   float64 // 墙钟时间戳（LWW 用）
}

// 固定时间线：物理时间 1000ms 起
func trace() {
	fmt.Println("① 固定时间线（3 副本、R2→R3 因果链、R1 分区并发写；R1 时钟 +20ms、R3 −40ms）")
	fmt.Println("   物理时刻  副本  动作                       版本向量              LWW 时间戳")
	events := []struct {
		t     float64
		r     int
		desc  string
		value string
		vv    vv
		stamp float64
	}{
		{1000, 1, "写 B（基线）", "B", vv{0, 1, 0}, 1000},
		{1001, 2, "收到 B（复制）", "-", vv{0, 1, 0}, 0},
		{1002, 2, "写 C（因果晚于 B）", "C", vv{0, 1, 1}, 1002 - 40},
		{1000, 0, "写 A（分区内并发）", "A", vv{1, 0, 0}, 1000 + 20},
	}
	for _, e := range events {
		vvStr := "-"
		if e.value != "-" {
			vvStr = e.vv.String()
		}
		tsStr := "-"
		if e.value != "-" {
			tsStr = fmt.Sprintf("%.0fms", e.stamp)
		}
		row([]string{fmt.Sprintf("%.0fms", e.t), fmt.Sprintf("R%d", e.r+1), e.desc, vvStr, tsStr},
			[]int{10, 5, 26, 20, 12})
	}

	// 合并：R1{A:{R1:1}}  R2{B:{R2:1}, C:{R2:1,R3:1}}  并发判定
	a := version{0, "A", vv{1, 0, 0}, 1020}
	b := version{1, "B", vv{0, 1, 0}, 1000}
	c := version{2, "C", vv{0, 1, 1}, 962}
	fmt.Println()
	row([]string{"策略", "最终值", "保留版本数", "检出冲突", "静默丢写", "因果逆序"},
		[]int{12, 8, 10, 8, 8, 8})
	row([]string{"版本向量", "A / C", "2", "1", "0", "0"}, []int{12, 8, 10, 8, 8, 8})
	row([]string{"LWW", "A", "1", "0", "2", "1"}, []int{12, 8, 10, 8, 8, 8})
	fmt.Println("\n   VV 判定：C 的向量 {0,1,1} 支配 B 的 {0,1,0}（因果、非冲突）；A 与 C 互不支配（并发）。")
	fmt.Println("   LWW 判定：时间戳 A=1020 > B=1000 > C=962，保留 A；丢 B、C 两次写。")
	fmt.Println("   因果逆序：C 在收到 B 之后写，却因 R3 时钟 −40ms 被 B 的时间戳压过——LWW 无法看见因果。")
	fmt.Println("   注意：A 是并发写，LWW 选 A 本身不算错；错在它同时静默扔掉了因果更新的 C。")

	// 反查：时间戳顺序与因果顺序
	fmt.Printf("\n   因果反查：B ≺ C（R3 收到 B 后写 C）；时间戳 B=%.0f > C=%.0f → LWW 选出的顺序与因果相反。\n", b.stamp, c.stamp)
	fmt.Printf("   VV 反查：A 与 C 并发（%v vs %v）→ 唯一真正的冲突是 A↔C，B 已被 C 吸收。\n", a.vv, c.vv)
}

// ---------- 蒙特卡洛 ----------
type mcStat struct {
	conflictRate float64 // 存在并发版本（冲突）的试验占比
	maxBranches  float64 // 并发分支（极大版本）平均数
	wrongWinner  float64 // LWW 选中的版本被其他版本因果支配的试验占比
	vvLost       float64 // 若上层只保留一个值（误用 VV 而不合并），平均丢写
}

func genVersions(r *rand.Rand, writes int, dropRate float64, skewMs float64) []version {
	seen := make([][]int, replicas) // 每个副本已见过的版本下标
	skew := [replicas]float64{}
	for i := range skew {
		skew[i] = (r.Float64()*2 - 1) * skewMs
	}
	vs := make([]version, 0, writes)
	for w := 0; w < writes; w++ {
		rep := w % replicas
		var v vv
		for _, idx := range seen[rep] {
			v = v.join(vs[idx].vv)
		}
		v[rep]++
		vw := version{replica: rep, value: fmt.Sprintf("v%d", w), vv: v,
			stamp: float64(w*10) + skew[rep] + r.Float64()}
		vs = append(vs, vw)
		idx := len(vs) - 1
		seen[rep] = append(seen[rep], idx)
		// 复制给其他副本：随机分区时按 dropRate 丢消息
		for other := 0; other < replicas; other++ {
			if other == rep {
				continue
			}
			if r.Float64() >= dropRate {
				seen[other] = append(seen[other], idx)
			}
		}
	}
	return vs
}

func analyze(vs []version) mcStat {
	n := len(vs)
	dominated := make([]bool, n) // 被其他版本严格支配（有因果更晚的写）
	anyConflict := false
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if vs[i].vv.leq(vs[j].vv) && vs[i].vv != vs[j].vv {
				dominated[i] = true
			}
			if !vs[i].vv.leq(vs[j].vv) && !vs[j].vv.leq(vs[i].vv) {
				anyConflict = true
			}
		}
	}
	heads := 0
	for i := 0; i < n; i++ {
		if !dominated[i] {
			heads++
		}
	}
	winner := 0
	for i := range vs {
		if vs[i].stamp > vs[winner].stamp {
			winner = i
		}
	}
	conflict := 0.0
	if anyConflict {
		conflict = 1
	}
	wrong := 0.0
	if dominated[winner] {
		wrong = 1
	}
	// 若把 VV 结果「只留一个」误用：保留 stamp 最大的，丢掉其余极大版本
	return mcStat{conflictRate: conflict, maxBranches: float64(heads),
		wrongWinner: wrong, vvLost: float64(heads - 1)}
}

func main() {
	trace()

	const tries, writes = 500, 100
	fmt.Printf("\n② 蒙特卡洛（3 副本 × 同一 key × 100 次写 × %d 轮；写入轮流落到各副本）\n", tries)
	row([]string{"场景", "VV 检出并发率", "VV 需合并分支", "LWW 选错率", "VV 误用丢写", "LWW 丢写"},
		[]int{30, 12, 14, 12, 12, 10})
	type scene struct {
		name     string
		dropRate float64
		skew     float64
	}
	for _, s := range []scene{
		{"无分区、无时钟偏移", 0.0, 0.0},
		{"随机丢复制 30%、无偏移", 0.30, 0.0},
		{"随机丢复制 30%、偏移 ±50ms", 0.30, 50.0},
	} {
		r := rand.New(rand.NewSource(20261122))
		var sum mcStat
		for k := 0; k < tries; k++ {
			vs := genVersions(r, writes, s.dropRate, s.skew)
			st := analyze(vs)
			sum.conflictRate += st.conflictRate
			sum.maxBranches += st.maxBranches
			sum.wrongWinner += st.wrongWinner
			sum.vvLost += st.vvLost
		}
		f := func(x float64) float64 { return x / tries }
		row([]string{s.name,
			fmt.Sprintf("%.0f%%", 100*f(sum.conflictRate)),
			fmt.Sprintf("%.1f 个", f(sum.maxBranches)),
			fmt.Sprintf("%.0f%%", 100*f(sum.wrongWinner)),
			fmt.Sprintf("%.1f 次", f(sum.vvLost)),
			fmt.Sprintf("%d 次", writes-1)},
			[]int{30, 12, 14, 12, 12, 10})
	}
	fmt.Println("\n   读数：LWW 无论哪一行都只保留 1 个值 → 99 次写被静默覆盖；VV 保留全部并发分支（需业务合并），")
	fmt.Println("   但 VV 无法替你决定 A 和 C 谁对——它只回答「这两个写谁也没见过谁」。")

	fmt.Println("\n③ 元数据开销（每个 key 每次写都要带）")
	row([]string{"副本数 N", "版本向量（8B×N）", "LWW 时间戳", "100B 值时的元数据占比", "1000B 值时的占比"},
		[]int{10, 18, 14, 22, 20})
	for _, n := range []int{3, 10, 100} {
		vvBytes := 8 * n
		row([]string{fmt.Sprintf("%d", n), fmt.Sprintf("%dB", vvBytes), "8B",
			fmt.Sprintf("%.0f%%", 100*float64(vvBytes)/100),
			fmt.Sprintf("%.0f%%", 100*float64(vvBytes)/1000)},
			[]int{10, 18, 14, 22, 20})
	}
}
