package main

import "fmt"

// 枚举 n 个节点里所有大小为 q 的集合，检查「任意两个是否必相交」以及最小交集大小。
// 这是 PBFT 安全性的全部算术：跨视图的两个 quorum 只要交出一个诚实节点，
// 新主就一定能从视图切换消息里学到「已经提交的请求」。

func subsets(n, q int) [][]int {
	var out [][]int
	var cur []int
	var rec func(start int)
	rec = func(start int) {
		if len(cur) == q {
			cp := append([]int(nil), cur...)
			out = append(out, cp)
			return
		}
		for i := start; i < n; i++ {
			cur = append(cur, i)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)
	return out
}

func intersectSet(a, b []int) []int {
	set := map[int]bool{}
	for _, x := range a {
		set[x] = true
	}
	var out []int
	for _, y := range b {
		if set[y] {
			out = append(out, y)
		}
	}
	return out
}

func intersect(a, b []int) int {
	set := map[int]bool{}
	for _, x := range a {
		set[x] = true
	}
	c := 0
	for _, y := range b {
		if set[y] {
			c++
		}
	}
	return c
}

// 返回：最小交集、是否存在不相交的一对、给出反例（如果存在）
func check(n, q int) (int, bool, []int, []int) {
	ss := subsets(n, q)
	min := q
	disjoint := false
	var d1, d2 []int
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			c := intersect(ss[i], ss[j])
			if c < min {
				min = c
			}
			if c == 0 && !disjoint {
				disjoint, d1, d2 = true, ss[i], ss[j]
			}
		}
	}
	return min, disjoint, d1, d2
}

func main() {
	fmt.Println("=== 任意两个 quorum 的最小交集 ===")
	fmt.Printf("%-4s %-4s %-8s %-10s %s\n", "n", "q", "最小交集", "可不相交", "场景")
	for _, c := range []struct {
		n, q int
		name string
	}{
		{4, 3, "PBFT n=4 f=1（正确：q=2f+1）"},
		{7, 5, "PBFT n=7 f=2"},
		{4, 2, "错误配置：q=f+1=2（照抄崩溃模型）"},
		{3, 2, "Raft n=3（崩溃模型，q=f+1）"},
	} {
		min, disjoint, a, b := check(c.n, c.q)
		note := "任意两组必相交"
		if disjoint {
			note = fmt.Sprintf("反例 %v 与 %v 完全不相交", a, b)
		}
		fmt.Printf("%-4d %-4d %-8d %-10v %s\n", c.n, c.q, min, disjoint, note)
	}

	fmt.Println("\n=== 每请求消息数（n 个副本，含主）===")
	fmt.Printf("%-4s %-12s %-12s %-10s %-8s %-8s\n", "n", "pre-prepare", "prepare", "commit", "reply", "合计")
	for _, n := range []int{4, 7, 10} {
		pre := n - 1              // 主 → 每个备
		prep := (n - 1) * (n - 1) // 每个备 → 所有其他副本
		com := n * (n - 1)        // 每个副本（含主）→ 所有其他副本
		reply := n                // 每个副本 → 客户端
		total := pre + prep + com + reply
		fmt.Printf("%-4d %-12d %-12d %-10d %-8d %-8d\n", n, pre, prep, com, reply, total)
	}
	fmt.Println("（不含签名校验与重传；渐近为 2n²，n=4 时论文粗算 32）")

	fmt.Println("\n=== 视图切换：新主必须拿到哪些证据 ===")
	// 已提交的请求：至少 2f+1 个副本 commit 过
	n, f := 4, 1
	commitQ := []int{0, 1, 2}
	// 新视图的视图切换 quorum：至少 2f+1 个 view-change 消息
	viewQ := []int{1, 2, 3}
	inter := intersectSet(commitQ, viewQ)
	fmt.Printf("  n=%d f=%d：commit quorum=%v，view-change quorum=%v\n", n, f, commitQ, viewQ)
	fmt.Printf("  交集=%v（%d 个），其中至少 1 个诚实 → 新主必能学到该请求\n", inter, len(inter))
	if len(inter) < f+1 {
		fmt.Println("  ✗ 交集不足 f+1，安全性被破坏")
	} else {
		fmt.Println("  ✓ 交集 ≥ f+1，满足 PBFT 的跨视图安全条件")
	}
}
