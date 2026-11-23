// 架构能力评估：拆边界 / 算代价 / 失败模式 / 落地路径，各 1–5 分。
// 输出最弱项与「是否具备独立架构能力」。
package main

import "fmt"

var names = [4]string{"拆边界", "算代价", "失败模式", "落地路径"}

func assess(s [4]int) (string, string) {
	min, weak := 5, 0
	sum := 0
	for i, v := range s {
		sum += v
		if v < min {
			min, weak = v, i
		}
	}
	avg := float64(sum) / 4
	switch {
	case min <= 2:
		return "不具备", "最弱：" + names[weak] + "（先补这一项）"
	case avg >= 4:
		return "强", "最弱：" + names[weak] + "（可独立负责架构）"
	default:
		return "一般", "最弱：" + names[weak] + "（需人带）"
	}
}

func main() {
	cands := [][4]int{{5, 4, 4, 4}, {4, 2, 4, 4}, {3, 3, 3, 3}}
	for i, c := range cands {
		v, why := assess(c)
		fmt.Printf("候选人%d %v → %s，%s\n", i+1, c, v, why)
	}
}
