// 团队能力矩阵：每人 × 每项能力（1–5 分），算缺口与单点。
package main

import (
	"fmt"
	"sort"
)

var need = map[string]int{"Go": 4, "分布式": 4, "数据库": 4, "K8s": 3, "可观测": 3}

func main() {
	team := map[string]map[string]int{
		"A": {"Go": 5, "分布式": 4, "数据库": 3, "K8s": 2, "可观测": 3},
		"B": {"Go": 4, "分布式": 3, "数据库": 5, "K8s": 3, "可观测": 2},
		"C": {"Go": 3, "分布式": 2, "数据库": 2, "K8s": 4, "可观测": 2},
	}
	var gaps []string
	for skill, want := range need {
		best, holders := 0, 0
		for _, p := range team {
			if s := p[skill]; s > best {
				best = s
			}
			if p[skill] >= want {
				holders++
			}
		}
		flag := ""
		if holders < 2 {
			flag = " ← 单点/缺口"
		}
		gaps = append(gaps, fmt.Sprintf("%-6s 需求=%d 最高=%d 达标人数=%d%s", skill, want, best, holders, flag))
	}
	sort.Strings(gaps)
	for _, g := range gaps {
		fmt.Println(g)
	}
}
