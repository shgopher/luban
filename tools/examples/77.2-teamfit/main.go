// 团队类型推荐：按「面向谁 + 交付什么 + 复杂度」选四种团队类型之一。
package main

import "fmt"

type team struct {
	name     string
	users    bool // 直接面向最终用户/业务流
	internal bool // 为其他团队提供能力
	deep     bool // 需要长期深耕的复杂子系统
	services int  // 拥有的服务数
}

func classify(t team) (string, string) {
	switch {
	case t.deep:
		return "复杂子系统团队", "按稀缺专长组队，接口按 X-as-a-Service 提供"
	case t.internal && t.services > 5:
		return "平台团队（先拆）", "服务数 > 5，按能力域拆分，否则平台自己变成瓶颈"
	case t.internal && !t.users:
		return "平台团队", "把内部能力做成产品：SLA + 文档 + 自助"
	case t.services <= 3:
		return "流对齐团队", "端到端拥有一个业务流，独立部署"
	default:
		return "赋能团队（或拆分）", "服务数 > 3，先按业务流拆，再谈赋能"
	}
}

func main() {
	ts := []team{
		{"订单", true, false, false, 3},
		{"支付网关", true, false, false, 2},
		{"容器平台", false, true, false, 5},
		{"风控引擎", false, false, true, 2},
		{"数据中台", false, true, false, 9},
	}
	for _, t := range ts {
		k, how := classify(t)
		fmt.Printf("%-8s → %-16s %s\n", t.name, k, how)
	}
}
