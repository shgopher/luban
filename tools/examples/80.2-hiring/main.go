// 面试评分卡：四个维度按权重折算，低于门槛直接不通过。
package main

import "fmt"

type dim struct {
	name   string
	weight float64
	gate   float64 // 该维度最低分（1–5）
}

var dims = []dim{
	{"编码", 0.30, 3},
	{"基础与系统", 0.30, 3},
	{"设计", 0.25, 3},
	{"沟通与协作", 0.15, 2},
}

func decide(score map[string]float64) (float64, string) {
	var total float64
	for _, d := range dims {
		if score[d.name] < d.gate {
			return 0, "不通过： " + d.name + " 低于门槛"
		}
		total += score[d.name] * d.weight
	}
	switch {
	case total >= 3.8:
		return total, "strong hire"
	case total >= 3.2:
		return total, "hire"
	default:
		return total, "no hire"
	}
}

func main() {
	cands := []map[string]float64{
		{"编码": 4, "基础与系统": 4, "设计": 3, "沟通与协作": 4},
		{"编码": 2, "基础与系统": 5, "设计": 5, "沟通与协作": 4},
		{"编码": 3, "基础与系统": 3, "设计": 3, "沟通与协作": 3},
	}
	for i, c := range cands {
		s, verdict := decide(c)
		fmt.Printf("候选人%d 加权=%.2f 结论=%s\n", i+1, s, verdict)
	}
}
