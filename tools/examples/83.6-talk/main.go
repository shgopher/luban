// 技术演讲时长预算：按「每页分钟数 × 页数 + 开场 + Q&A」校验是否超时。
package main

import "fmt"

func main() {
	slides := 18
	perSlide := 1.2 // 经验值：技术页 1–1.5 分钟
	open, qa := 3.0, 10.0
	total := float64(slides)*perSlide + open + qa
	budget := 30.0
	fmt.Printf("页数=%d 预计=%.0f 分钟（预算 %.0f）", slides, total, budget)
	if total > budget {
		fmt.Printf(" → 超时 %.0f 分钟：先砍页，别砍 Q&A\n", total-budget)
	} else {
		fmt.Println(" → 留有余量")
	}
}
