// 支付资损账：日交易额、手续费收入、资损容忍线，以及差异挂账的成本。
package main

import "fmt"

func main() {
	orders := 1500000.0 // 日支付笔数
	ticket := 200.0     // 客单价
	gmv := orders * ticket
	feeRate := 0.006 // 渠道费率 0.6%
	fee := gmv * feeRate
	tolerance := 0.0001 // 资损容忍 0.01%
	lossBudget := gmv * tolerance
	fmt.Printf("日 GMV=%.0f 元（%.2f 亿）\n", gmv, gmv/1e8)
	fmt.Printf("渠道手续费=%.0f 元/日（%.1f 万）\n", fee, fee/1e4)
	fmt.Printf("资损容忍=%.0f 元/日（%.1f 万）\n", lossBudget, lossBudget/1e4)
	fmt.Printf("单笔资损 200 元 → 容忍 %.0f 笔/日；差异率 0.02%% ≈ %.0f 笔\n",
		lossBudget/200, orders*0.0002)
}
