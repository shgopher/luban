// 岗位等级判定：技术深度 / 系统设计 / 交付 / 影响力，各 1–5 分。
// 判级不看总分，看「短板」——任何一项低于该级最低要求就降级。
package main

import "fmt"

type level struct {
	name string
	need [4]int // 深度、设计、交付、影响力
}

var levels = []level{
	{"P5 独立交付", [4]int{3, 2, 3, 1}},
	{"P6 模块负责", [4]int{3, 3, 4, 2}},
	{"P7 领域负责", [4]int{4, 4, 4, 3}},
	{"P8 方向定义", [4]int{4, 5, 4, 4}},
}

func main() {
	cand := [4]int{4, 3, 4, 2}
	got := "无"
	for _, l := range levels {
		ok := true
		for i := range l.need {
			if cand[i] < l.need[i] {
				ok = false
			}
		}
		if ok {
			got = l.name
		}
	}
	fmt.Printf("候选人 %v → %s\n", cand, got)
}
