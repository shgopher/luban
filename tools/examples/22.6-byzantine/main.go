package main

import "fmt"

// OM(1) 的可运行模型：一个指挥官 + n-1 个下属，最多 1 个叛徒。
// 叛徒的行为：当指挥官时对不同人报不同值；当下属时对不同的听众报不同的“我听到的值”。
// 这是 OM(m) 的最小非平凡情形，足以看出 3m+1 的界。

type world struct {
	n       int
	traitor map[int]bool
}

// 指挥官 c 把 value 发给每个下属；叛徒指挥官对每人报的值可以不同
func (w *world) commanderSend(c, value int) map[int]int {
	got := map[int]int{}
	for i := 0; i < w.n; i++ {
		if i == c {
			continue
		}
		v := value
		if w.traitor[c] {
			// 让叛徒指挥官对编号大的人报相反值，制造分裂
			if i%2 == 1 {
				v = 1 - value
			}
		}
		got[i] = v
	}
	return got
}

// 每个下属把自己听到的值广播给其他下属；叛徒下属可以各说各话
func (w *world) broadcast(from, commander, heard int) map[int]int {
	rep := map[int]int{}
	for i := 0; i < w.n; i++ {
		if i == from || i == commander {
			continue // 只发给其他下属，不回传给指挥官
		}
		v := heard
		if w.traitor[from] {
			v = 1 - heard // 叛徒对所有听众报相反值
		}
		rep[i] = v
	}
	return rep
}

func majority(vs []int) int {
	cnt := map[int]int{}
	for _, v := range vs {
		cnt[v]++
	}
	best, bc := 0, -1
	for v, c := range cnt {
		if c > bc || (c == bc && v < best) {
			best, bc = v, c
		}
	}
	// 平票按 RETREAT(0) 处理：这是忠实实现里“无法形成多数”的默认动作
	if bc*2 <= len(vs) {
		return 0
	}
	return best
}

func (w *world) om1(commander, value int) (map[int]int, int) {
	heard := w.commanderSend(commander, value)
	msgs := len(heard)
	// 每个下属先广播自己听到的值，形成一张「谁对谁说了什么」的表
	reports := map[int]map[int]int{}
	for j := 0; j < w.n; j++ {
		if j == commander {
			continue
		}
		reports[j] = w.broadcast(j, commander, heard[j])
		msgs += len(reports[j])
	}
	decisions := map[int]int{}
	for i := 0; i < w.n; i++ {
		if i == commander {
			continue
		}
		votes := []int{heard[i]}
		for j := 0; j < w.n; j++ {
			if j == commander || j == i {
				continue
			}
			if v, ok := reports[j][i]; ok {
				votes = append(votes, v)
			}
		}
		decisions[i] = majority(votes)
	}
	return decisions, msgs
}

func run(name string, n, commander, value int, traitors map[int]bool) {
	fmt.Printf("=== %s ===\n", name)
	w := &world{n: n, traitor: traitors}
	d, msgs := w.om1(commander, value)
	agree, all := true, true
	first := -1
	for i := 0; i < n; i++ {
		if i == commander || traitors[i] {
			continue
		}
		fmt.Printf("  忠诚下属 %d 的决策 = %d\n", i, d[i])
		if first == -1 {
			first = d[i]
		} else if d[i] != first {
			agree = false
		}
		if d[i] != value {
			all = false
		}
	}
	fmt.Printf("  忠诚下属是否一致：%v；是否等于指挥官命令 (%d)：%v；消息数：%d\n\n", agree, value, all, msgs)
}

func main() {
	run("n=4, m=1：指挥官忠诚，3 号叛徒", 4, 0, 1, map[int]bool{3: true})
	run("n=3, m=1：指挥官忠诚，2 号叛徒", 3, 0, 1, map[int]bool{2: true})
	run("n=3, m=1：指挥官叛徒，对 1 报 1、对 2 报 0", 3, 0, 1, map[int]bool{0: true})

	fmt.Println("=== 消息数：OM(1) = (n-1) + (n-1)(n-2) ===")
	for _, n := range []int{3, 4, 7, 10} {
		m := (n - 1) + (n-1)*(n-2)
		fmt.Printf("  n=%-3d 需要 %-4d 条消息（OM(1)）\n", n, m)
	}

	fmt.Println("\n=== quorum 相交下界 2q-n（n=3f+1, q=2f+1 时恒为 f+1）===")
	for _, f := range []int{0, 1, 2, 3} {
		n, q := 3*f+1, 2*f+1
		fmt.Printf("  f=%d → n=%-3d q=%-3d 2q-n=%d\n", f, n, q, 2*q-n)
	}
}
