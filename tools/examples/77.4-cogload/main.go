// 团队认知负荷：系统数 × 技术栈数 × 服务数 / 人数，超过阈值说明该拆或该减范围。
package main

import "fmt"

type team struct {
	name                         string
	systems, stacks, svc, people int
}

func main() {
	ts := []team{
		{"订单团队", 3, 4, 3, 6},
		{"全能小组", 9, 7, 12, 5},
		{"平台团队", 2, 3, 5, 5},
	}
	for _, t := range ts {
		load := float64(t.systems*t.stacks+t.svc) / float64(t.people)
		verdict := "正常"
		if load > 8 {
			verdict = "超载：先减范围（还服务/还系统），再谈扩编"
		} else if load > 4 {
			verdict = "偏高：列出下周要碰的系统，砍掉不常碰的"
		}
		fmt.Printf("%-8s 负荷=%5.1f  %s\n", t.name, load, verdict)
	}
}
