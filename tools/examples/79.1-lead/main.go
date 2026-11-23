// 决策可逆性打分：影响面 / 可逆性 / 成本 / 时间压力，各 1–5 分。
// 分数越高 = 越该走轻流程（自己拍板）；越低 = 越该走重流程（评审 + 记录 + 退路）。
package main

import "fmt"

type decision struct {
	name               string
	impact, reversible int // 影响面、可逆性（5 = 极易回退）
	cost, urgency      int // 成本、时间压力
}

func weight(d decision) float64 {
	// 影响面与成本越大、越不可逆，越该慎重
	return float64(d.impact*2+(6-d.reversible)*2+d.cost) / 5.0
}

func main() {
	ds := []decision{
		{"换一个日志库", 1, 5, 2, 2},
		{"订单表加索引", 2, 4, 1, 3},
		{"拆出支付服务", 4, 2, 4, 2},
		{"自研分布式存储", 5, 1, 5, 1},
	}
	for _, d := range ds {
		w := weight(d)
		flow := "轻流程：负责人拍板"
		switch {
		case w >= 2.5 && w < 3.5:
			flow = "中流程：写 ADR + 1 次评审"
		case w >= 3.5:
			flow = "重流程：ADR + 评审 + 试点 + 回滚预案"
		}
		fmt.Printf("%-14s 权重=%.1f → %s\n", d.name, w, flow)
	}
}
