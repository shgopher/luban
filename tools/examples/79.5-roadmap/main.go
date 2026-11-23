// 路线图分波检查：每一波必须有「可交付物 + 退出标准 + 回滚点」，且时长 ≤ 6 个月。
package main

import "fmt"

type wave struct {
	name     string
	months   int
	deliver  string
	exit     string
	rollback string
}

func check(ws []wave) {
	for _, w := range ws {
		var errs []string
		if w.deliver == "" {
			errs = append(errs, "缺可交付物")
		}
		if w.exit == "" {
			errs = append(errs, "缺退出标准")
		}
		if w.rollback == "" {
			errs = append(errs, "缺回滚点")
		}
		if w.months > 6 {
			errs = append(errs, "单波超过 6 个月")
		}
		if len(errs) == 0 {
			fmt.Printf("%-12s ✓ 通过（%d 个月）\n", w.name, w.months)
			continue
		}
		fmt.Printf("%-12s ✗ %v\n", w.name, errs)
	}
}

func main() {
	check([]wave{
		{"W1 拆读写", 3, "读写分离上线", "P99 降 30%", "切回单库"},
		{"W2 拆订单", 5, "订单服务独立", "回归通过率 100%", "保留双写 2 周"},
		{"W3 数据迁移", 9, "全量迁到新库", "", "保留旧库只读"},
	})
}
