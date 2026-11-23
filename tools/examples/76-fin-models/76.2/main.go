package main

import (
	"fmt"
	"sort"
)

type Txn struct {
	ID           string
	Payer, Payee string
	Amt          int64 // 分
}

// fee 按万六计费、四舍五入到分，由付款方承担。
func fee(amt int64) int64 { return (amt*6 + 5000) / 10000 }

func yuan(v int64) string { return fmt.Sprintf("%.2f", float64(v)/100) }

func main() {
	txns := []Txn{
		{"T01", "A", "B", 12345},
		{"T02", "B", "C", 80000},
		{"T03", "C", "A", 250000},
		{"T04", "D", "A", 66666},
		{"T05", "A", "D", 999},
		{"T06", "B", "A", 1500000},
		{"T07", "C", "D", 43210},
		{"T08", "D", "B", 70000},
		{"T09", "A", "C", 550000},
		{"T10", "B", "D", 100},
	}

	fmt.Println("== 1. 交易明细（T 日受理，金额以元计）==")
	var totalAmt, totalFee int64
	for _, t := range txns {
		f := fee(t.Amt)
		totalAmt += t.Amt
		totalFee += f
		fmt.Printf("%s %s→%s 金额 %10s 手续费 %6s\n", t.ID, t.Payer, t.Payee, yuan(t.Amt), yuan(f))
	}
	fmt.Printf("合计：交易 %d 笔、金额 %s 元、手续费 %s 元\n\n", len(txns), yuan(totalAmt), yuan(totalFee))

	// 双边轧差：同一对机构之间方向相反的交易先对冲
	pair := map[string]int64{}
	for _, t := range txns {
		pair[t.Payer+">"+t.Payee] += t.Amt
	}
	keys := make([]string, 0, len(pair))
	for k := range pair {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("== 2. 双边轧差（同一对机构合并）==")
	seen := map[string]bool{}
	var bilateralAmt int64
	bilateralCnt := 0
	for _, k := range keys {
		var a, b string
		for i := 0; i < len(k); i++ {
			if k[i] == '>' {
				a, b = k[:i], k[i+1:]
			}
		}
		fwd, rev := a+">"+b, b+">"+a
		if seen[fwd] || seen[rev] {
			continue
		}
		seen[fwd], seen[rev] = true, true
		netv := pair[fwd] - pair[rev]
		if netv == 0 {
			fmt.Printf("%s↔%s 完全对冲，净额 0.00\n", a, b)
			continue
		}
		from, to := a, b
		if netv < 0 {
			netv, from, to = -netv, b, a
		}
		bilateralAmt += netv
		bilateralCnt++
		fmt.Printf("%s→%s 净额 %s\n", from, to, yuan(netv))
	}
	fmt.Printf("双边轧差：%d 笔划拨，合计 %s 元\n\n", bilateralCnt, yuan(bilateralAmt))

	// 多边轧差：每个成员对全体轧出一个净额，含手续费
	net := map[string]int64{}
	for _, t := range txns {
		f := fee(t.Amt)
		net[t.Payer] -= t.Amt + f // 付款方：付出本金 + 手续费
		net[t.Payee] += t.Amt     // 收款方：收到本金
		net["CC"] += f            // 清算机构：收手续费
	}
	members := []string{"A", "B", "C", "D", "CC"}
	fmt.Println("== 3. 多边净额（含手续费；正 = 应收，负 = 应付）==")
	var sum, multiAmt int64
	multiCnt := 0
	for _, m := range members {
		sum += net[m]
		if net[m] != 0 {
			multiCnt++
		}
		if net[m] < 0 {
			multiAmt -= net[m]
		}
		fmt.Printf("%-3s 净额 %12s 元\n", m, yuan(net[m]))
	}
	fmt.Printf("Σ净额 = %d（清算守恒，必须为 0）\n", sum)
	fmt.Printf("多边净额：%d 笔划拨，应付合计 %s 元\n\n", multiCnt, yuan(multiAmt))

	fmt.Println("== 4. 划拨笔数对比（净额结算的价值）==")
	fmt.Printf("逐笔全额  %2d 笔，资金流 %s 元\n", len(txns), yuan(totalAmt+totalFee))
	fmt.Printf("双边轧差  %2d 笔，资金流 %s 元\n", bilateralCnt, yuan(bilateralAmt))
	fmt.Printf("多边净额  %2d 笔，资金流 %s 元\n\n", multiCnt, yuan(multiAmt))

	// 头寸检查：净付方在结算窗口开盘前必须有足额头寸
	pos := map[string]int64{"A": 800000, "B": 300000, "C": 2000000, "D": 100000}
	fmt.Println("== 5. 结算前头寸检查（净付方）==")
	for _, m := range []string{"A", "B", "C", "D"} {
		if net[m] >= 0 {
			continue
		}
		need := -net[m]
		mark := "✓ 足额"
		if pos[m] < need {
			mark = "✗ 头寸不足，排队至下一窗口"
		}
		fmt.Printf("%s 需付 %12s 头寸 %12s %s\n", m, yuan(need), yuan(pos[m]), mark)
	}

	fmt.Println("\n== 6. 清算结算时间线 ==")
	type step struct{ at, what string }
	for _, s := range []step{
		{"T 日 20:00", "交易截单，渠道停止受理当日新单"},
		{"T 日 20:30", "生成清算文件，逐笔明细落库"},
		{"T 日 21:00", "多边轧差，产出各成员净额与净额确认书"},
		{"T 日 23:00", "净额确认书回执，成员确认应收应付"},
		{"T+1 09:00", "结算窗口开盘，头寸不足的成员排队"},
		{"T+1 09:30", "资金划拨指令发出（大额支付系统，报文以官方文档为准）"},
		{"T+1 10:00", "结算完成，回单落地；账务侧记账"},
		{"T+1 11:00", "总分核对 + 渠道对账"},
	} {
		fmt.Printf("%-10s %s\n", s.at, s.what)
	}
}
