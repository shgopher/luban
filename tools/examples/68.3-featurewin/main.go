// 实时特征：滑动窗口（5 分钟）内的交易次数与金额，用于频次类规则。
package main

import "fmt"

func main() {
	events := []struct {
		ts     int // 秒
		amount int
	}{
		{0, 100}, {60, 200}, {120, 300}, {240, 400}, {300, 500}, {600, 900},
	}
	now, window := 300, 300
	count, sum := 0, 0
	for _, e := range events {
		if now-e.ts <= window {
			count++
			sum += e.amount
		}
	}
	fmt.Printf("近 %d 秒：笔数=%d 金额=%d\n", window, count, sum)
	fmt.Println("判据：窗口内笔数 > 5 或金额 > 1000 → 触发频次规则")
}
