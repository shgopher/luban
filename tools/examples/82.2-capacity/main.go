// 容量估算：利特尔法则算并发，再按单机容量与冗余算机器数。
// 并发 = QPS × 平均延迟；机器数 = 峰值 QPS / (单机 QPS) × 冗余系数。
package main

import "fmt"

func main() {
	type step struct {
		name         string
		qps, delayMs float64 // 峰值 QPS、平均延迟
		perNodeQPS   float64 // 单机可承载 QPS
		redundancy   float64
	}
	steps := []step{
		{"下单", 8000, 30, 500, 1.5},
		{"支付回调", 2000, 120, 200, 1.5},
		{"查询", 30000, 10, 2000, 1.3},
	}
	for _, s := range steps {
		conc := s.qps * s.delayMs / 1000 // 同时在处理中的请求数
		nodes := s.qps / s.perNodeQPS * s.redundancy
		fmt.Printf("%-8s 峰值QPS=%-6.0f 并发≈%-5.0f 机器≈%.0f 台（含冗余 %.1f）\n",
			s.name, s.qps, conc, nodes, s.redundancy)
	}
}
