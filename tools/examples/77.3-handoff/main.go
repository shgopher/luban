// 交接链长度：沿服务依赖图找最长链，链上每个跨团队边 = 一次交接。
// 交接次数越多，一次改动的协调成本越高（近似 2^n 的沟通路径）。
package main

import "fmt"

type edge struct{ from, to, team string }

func main() {
	edges := []edge{
		{"web", "order", "A"}, {"order", "payment", "A→B"},
		{"payment", "risk", "B→C"}, {"risk", "user", "C→D"},
		{"order", "inventory", "A→B"},
	}
	byFrom := map[string][]edge{}
	for _, e := range edges {
		byFrom[e.from] = append(byFrom[e.from], e)
	}
	var walk func(node string, depth int, chain string)
	best := 0
	walk = func(node string, depth int, chain string) {
		if depth > best {
			best = depth
		}
		if depth > 6 { // 防环
			return
		}
		for _, e := range byFrom[node] {
			walk(e.to, depth+1, chain+" -> "+e.to)
		}
	}
	walk("web", 0, "web")
	fmt.Printf("最长调用链长度=%d 跳（每跳都可能跨团队）\n", best)
	fmt.Println("经验判据：链长 > 4 跳，或跨 3 个以上团队，一次改动就要开会协调")
}
