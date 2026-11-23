package main

import "fmt"

// Account 是账务树上的一个科目节点。
// Kind 决定余额方向：资产/费用 借增贷减；负债/权益/收入 贷增借减。
type Account struct {
	Code, Name, Kind, Parent string
	Open, Dr, Cr             int64 // 期初余额、借方发生额、贷方发生额（元）
}

// Entry 是凭证里的一行分录，Dr 与 Cr 只有一个非零。
type Entry struct {
	Acct   string
	Dr, Cr int64
}

type Voucher struct {
	ID      string
	Entries []Entry
}

func debitNormal(kind string) bool { return kind == "资产" || kind == "费用" }

// balance 返回期末余额，正数表示该科目正常方向（资产在借、负债在贷）。
func (a *Account) balance() int64 {
	net := a.Dr - a.Cr
	if debitNormal(a.Kind) {
		return a.Open + net
	}
	return a.Open - net
}

func main() {
	accts := []*Account{
		{Code: "1000", Name: "资产", Kind: "资产"},
		{Code: "1001", Name: "库存现金", Kind: "资产", Parent: "1000", Open: 8000},
		{Code: "1002", Name: "存放中央银行款项", Kind: "资产", Parent: "1000", Open: 5000},
		{Code: "1101", Name: "客户贷款", Kind: "资产", Parent: "1000"},
		{Code: "2000", Name: "负债", Kind: "负债"},
		{Code: "2001", Name: "客户存款", Kind: "负债", Parent: "2000", Open: 3000},
		{Code: "2002", Name: "应付结算款", Kind: "负债", Parent: "2000"},
		{Code: "3000", Name: "所有者权益", Kind: "权益"},
		{Code: "3001", Name: "实收资本", Kind: "权益", Parent: "3000", Open: 10000},
		{Code: "3002", Name: "未分配利润", Kind: "权益", Parent: "3000"},
		{Code: "4000", Name: "收入", Kind: "收入"},
		{Code: "4001", Name: "手续费收入", Kind: "收入", Parent: "4000"},
		{Code: "5000", Name: "费用", Kind: "费用"},
		{Code: "5001", Name: "利息支出", Kind: "费用", Parent: "5000"},
	}
	byCode := map[string]*Account{}
	for _, a := range accts {
		byCode[a.Code] = a
	}

	vouchers := []Voucher{
		{ID: "V001", Entries: []Entry{{"1001", 10000, 0}, {"2001", 0, 10000}}}, // 现金存入
		{ID: "V002", Entries: []Entry{{"1101", 50000, 0}, {"2001", 0, 50000}}}, // 发放贷款
		{ID: "V003", Entries: []Entry{{"2001", 300, 0}, {"4001", 0, 300}}},     // 收手续费
		{ID: "V004", Entries: []Entry{{"5001", 120, 0}, {"2002", 0, 120}}},     // 计提利息支出
		{ID: "V005", Entries: []Entry{{"1002", 2000, 0}, {"1001", 0, 2000}}},   // 现金交存央行
		{ID: "V006", Entries: []Entry{{"1001", 500, 0}, {"2001", 0, 400}}},     // 故意不平衡
	}

	fmt.Println("== 1. 凭证校验与过账（每张凭证先自校验，不平衡整张拒绝）==")
	okCnt, badCnt := 0, 0
	for _, v := range vouchers {
		var d, c int64
		for _, e := range v.Entries {
			d += e.Dr
			c += e.Cr
		}
		if d != c {
			badCnt++
			fmt.Printf("%s ✗ 借 %d ≠ 贷 %d（差额 %d）→ 整张凭证拒绝入账\n", v.ID, d, c, d-c)
			continue
		}
		okCnt++
		for _, e := range v.Entries {
			byCode[e.Acct].Dr += e.Dr
			byCode[e.Acct].Cr += e.Cr
		}
		fmt.Printf("%s ✓ 借 %d = 贷 %d\n", v.ID, d, c)
	}
	fmt.Printf("过账汇总：接受 %d 张，拒绝 %d 张\n\n", okCnt, badCnt)

	// 只取叶子科目做主表
	var leaves []*Account
	for _, a := range accts {
		if a.Parent != "" {
			leaves = append(leaves, a)
		}
	}

	fmt.Println("\n== 2. 期末余额表（正 = 该科目正常方向）==")
	fmt.Printf("%-6s %-14s %-4s %10s\n", "科目", "名称", "方向", "余额")
	for _, a := range leaves {
		dir := "借"
		if !debitNormal(a.Kind) {
			dir = "贷"
		}
		fmt.Printf("%-6s %-14s %-4s %10d\n", a.Code, a.Name, dir, a.balance())
	}

	// 科目树：父科目余额必须等于所有子科目之和
	children := map[string][]*Account{}
	for _, a := range accts {
		if a.Parent != "" {
			children[a.Parent] = append(children[a.Parent], a)
		}
	}
	var roll func(code string) int64
	roll = func(code string) int64 {
		if len(children[code]) == 0 {
			return byCode[code].balance()
		}
		var s int64
		for _, ch := range children[code] {
			s += roll(ch.Code)
		}
		return s
	}
	fmt.Println("\n== 3. 科目树汇总（父 = 子之和）==")
	for _, root := range []string{"1000", "2000", "3000", "4000", "5000"} {
		var childSum int64
		for _, ch := range children[root] {
			childSum += roll(ch.Code)
		}
		mark := "✓"
		if childSum != roll(root) {
			mark = "✗"
		}
		fmt.Printf("%s %-10s 汇总 %8d | 子科目之和 %8d %s\n", root, byCode[root].Name, roll(root), childSum, mark)
	}

	fmt.Println("\n== 4. 试算平衡（借方余额合计 = 贷方余额合计）==")
	var sumDr, sumCr int64
	for _, a := range leaves {
		b := a.balance()
		if debitNormal(a.Kind) {
			sumDr += b
		} else {
			sumCr += b
		}
	}
	fmt.Printf("借方余额合计 %8d\n贷方余额合计 %8d\n差异 %12d\n", sumDr, sumCr, sumDr-sumCr)

	assets := roll("1000")
	liab := roll("2000")
	equity := roll("3000")
	income := roll("4000")
	expense := roll("5000")
	fmt.Printf("\n== 5. 资产负债表恒等式 ==\n资产 %d = 负债 %d + 权益 %d + (收入 %d - 费用 %d) = %d → 差异 %d\n",
		assets, liab, equity, income, expense, liab+equity+income-expense,
		assets-(liab+equity+income-expense))
}
