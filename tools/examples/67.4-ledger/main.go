// 复式记账：有借必有贷、借贷必相等；日终试算平衡。
package main

import "fmt"

type entry struct {
	debit, credit int
	account       string
}

func main() {
	entries := []entry{
		{1000, 0, "银行存款"}, {0, 1000, "商户待结算"},
		{20, 0, "手续费收入"}, {0, 20, "银行存款"},
	}
	var debit, credit int
	for _, e := range entries {
		debit += e.debit
		credit += e.credit
	}
	fmt.Printf("借方合计=%d 贷方合计=%d 差额=%d\n", debit, credit, debit-credit)
	if debit == credit {
		fmt.Println("试算平衡：通过")
	} else {
		fmt.Println("试算不平衡：分录有错")
	}
}
