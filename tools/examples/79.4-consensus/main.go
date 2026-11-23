// 共识度判定：支持 / 中立 / 反对 三票，输出共识层次与下一步动作。
package main

import "fmt"

func level(support, neutral, oppose int) (string, string) {
	total := support + neutral + oppose
	switch {
	case oppose > 0 && oppose*2 >= total:
		return "分裂", "停：先解决反对者的顾虑，或升级决策人"
	case oppose > 0:
		return "服从", "记录反对意见与理由，约定复盘点"
	case support*10 >= total*8:
		return "承诺", "直接执行，写明负责人与截止时间"
	case support*10 >= total*6:
		return "认同", "执行，但把中立者拉进试点"
	default:
		return "观望", "补证据（试点/数据）后再决策"
	}
}

func main() {
	cases := [][3]int{{5, 0, 0}, {4, 1, 0}, {3, 2, 0}, {2, 3, 0}, {1, 1, 3}}
	for _, c := range cases {
		l, next := level(c[0], c[1], c[2])
		fmt.Printf("支持=%d 中立=%d 反对=%d → %s：%s\n", c[0], c[1], c[2], l, next)
	}
}
