// 对账：以内部分账为基准，逐笔比对渠道账单，输出五类差异。
package main

import "fmt"

type rec struct {
	id, status string
	amt        int
}

func main() {
	inner := map[string]rec{
		"T1": {"T1", "SUCCESS", 1000}, "T2": {"T2", "SUCCESS", 2000},
		"T3": {"T3", "REFUNDED", 500}, "T4": {"T4", "SUCCESS", 300},
	}
	channel := map[string]rec{
		"T1": {"T1", "SUCCESS", 1000}, "T2": {"T2", "SUCCESS", 1800},
		"T3": {"T3", "REFUNDED", 500}, "T5": {"T5", "SUCCESS", 800},
	}
	var long, short, amtDiff, statusDiff int
	for id, i := range inner {
		c, ok := channel[id]
		if !ok {
			short++
			fmt.Printf("短款（渠道无此笔）：%s\n", id)
			continue
		}
		if i.amt != c.amt {
			amtDiff++
			fmt.Printf("金额不一致：%s 内部=%d 渠道=%d\n", id, i.amt, c.amt)
		}
		if i.status != c.status {
			statusDiff++
			fmt.Printf("状态不一致：%s\n", id)
		}
	}
	for id := range channel {
		if _, ok := inner[id]; !ok {
			long++
			fmt.Printf("长款（内部无此笔）：%s\n", id)
		}
	}
	fmt.Printf("差异合计：长款=%d 短款=%d 金额=%d 状态=%d\n", long, short, amtDiff, statusDiff)
}
