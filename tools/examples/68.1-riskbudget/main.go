// 风控决策时延预算：各阶段 P99 相加，超过预算就必须异步化。
package main

import "fmt"

func main() {
	stages := []struct {
		name string
		p99  float64
	}{
		{"参数校验", 1}, {"名单查询", 2}, {"实时特征", 8},
		{"规则引擎", 5}, {"模型打分", 25}, {"决策与落库", 6},
	}
	var sum float64
	for _, s := range stages {
		sum += s.p99
		fmt.Printf("%-10s %5.0f ms\n", s.name, s.p99)
	}
	fmt.Printf("合计 %.0f ms；预算 100 ms → ", sum)
	if sum > 100 {
		fmt.Println("超预算：把模型打分或特征查询改成异步/预计算")
	} else {
		fmt.Println("在预算内")
	}
}
