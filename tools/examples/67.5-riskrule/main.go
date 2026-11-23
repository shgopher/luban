// 交易风控：规则按顺序短路，命中即拒绝，记录命中规则。
package main

import "fmt"

type txn struct {
	user      string
	amount    int
	newDevice bool
	count5m   int
}

func main() {
	txns := []txn{
		{"u1", 8000, false, 1},
		{"u2", 60000, false, 1},
		{"u1", 12000, true, 6},
		{"u3", 300, false, 1},
	}
	for _, t := range txns {
		verdict := "通过"
		switch {
		case t.amount > 50000:
			verdict = "拒绝：超单笔限额"
		case t.count5m > 5:
			verdict = "拒绝：5 分钟频次超限"
		case t.newDevice && t.amount > 10000:
			verdict = "拒绝：新设备大额"
		}
		fmt.Printf("%-3s 金额=%-6d → %s\n", t.user, t.amount, verdict)
	}
}
