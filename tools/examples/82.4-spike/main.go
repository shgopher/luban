// 原型（Spike）计划检查：有没有问题、时间盒、成功判据、丢弃计划。
package main

import "fmt"

type spike struct{ q, box, done, drop string }

func main() {
	list := []spike{
		{"分库分表后跨片查询能否 <200ms", "3 天", "99 分位 <200ms", "原型代码不合入"},
		{"新技术选型", "两周", "", ""},
		{"消息队列能否支撑 5w TPS", "2 天", "压测达到 5w", "废弃"},
	}
	for _, s := range list {
		var errs []string
		if s.q == "" {
			errs = append(errs, "缺问题")
		}
		if s.box == "" {
			errs = append(errs, "缺时间盒")
		}
		if s.done == "" {
			errs = append(errs, "缺成功判据")
		}
		if s.drop == "" {
			errs = append(errs, "缺丢弃计划")
		}
		if len(errs) == 0 {
			fmt.Printf("✓ %s\n", s.q)
		} else {
			fmt.Printf("✗ %-24s %v\n", s.q, errs)
		}
	}
}
