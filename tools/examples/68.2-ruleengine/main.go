// 规则引擎：按优先级短路，命中即返回，并记录命中原因（可解释）。
package main

import "fmt"

type rule struct {
	id       string
	priority int
	match    func(amount int, newDevice bool) bool
	action   string
}

func main() {
	rules := []rule{
		{"R3 新设备大额", 30, func(a int, n bool) bool { return n && a > 10000 }, "人工复核"},
		{"R1 黑名单", 10, func(a int, n bool) bool { return false }, "拒绝"},
		{"R2 超限额", 20, func(a int, n bool) bool { return a > 50000 }, "拒绝"},
	}
	// 按优先级排序后顺序匹配
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].priority < rules[i].priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
	amount, newDevice := 60000, true
	for _, r := range rules {
		if r.match(amount, newDevice) {
			fmt.Printf("命中 %s（优先级 %d）→ %s\n", r.id, r.priority, r.action)
			return
		}
	}
	fmt.Println("全部未命中 → 放行")
}
