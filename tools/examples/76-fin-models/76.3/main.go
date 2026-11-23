package main

import "fmt"

type Rec struct {
	ID     string
	Amt    int64 // 分
	Status string
}

type Line struct {
	Vid, Note string
	DrAcct    string
	CrAcct    string
	Amt       int64
}

func yuan(v int64) string { return fmt.Sprintf("%.2f", float64(v)/100) }

func main() {
	// 内部账（账务系统）与渠道账单（以官方文档为准的字段）
	inner := []Rec{
		{"P01", 10000, "SUCCESS"},
		{"P02", 25000, "SUCCESS"},
		{"P03", 200000, "SUCCESS"},
		{"P04", 5000, "SUCCESS"},
		{"P05", 8800, "SUCCESS"},
	}
	channel := []Rec{
		{"P01", 10000, "SUCCESS"},
		{"P02", 25000, "REFUND"},
		{"P03", 180000, "SUCCESS"},
		{"P04", 5000, "SUCCESS"},
		{"P06", 30000, "SUCCESS"},
	}

	innerMap := map[string]Rec{}
	for _, r := range inner {
		innerMap[r.ID] = r
	}
	chMap := map[string]Rec{}
	for _, r := range channel {
		chMap[r.ID] = r
	}

	fmt.Println("== 1. 逐笔比对（内部账 vs 渠道账单）==")
	var lines []Line
	var longCnt, shortCnt, amtCnt, statusCnt int
	vid := 100
	newVid := func() string { vid++; return fmt.Sprintf("CZ%03d", vid) }
	addLine := func(note, dr, cr string, amt int64) {
		lines = append(lines, Line{newVid(), note, dr, cr, amt})
	}
	for _, in := range inner {
		ch, ok := chMap[in.ID]
		if !ok {
			shortCnt++
			fmt.Printf("%s 短款：内部 %s 元、渠道无此笔 → 查渠道状态，未受理则冲正\n", in.ID, yuan(in.Amt))
			addLine("短款冲正 "+in.ID, "2001", "1002", in.Amt) // 反方向：冲掉内部已记的收款
			continue
		}
		if in.Amt != ch.Amt {
			amtCnt++
			diff := in.Amt - ch.Amt
			side := "内部多记"
			if diff < 0 {
				side = "内部少记"
				diff = -diff
			}
			fmt.Printf("%s 金额不一致：内部 %s、渠道 %s（%s %s）→ 差额入损溢\n",
				in.ID, yuan(in.Amt), yuan(ch.Amt), side, yuan(diff))
			if in.Amt > ch.Amt {
				addLine("金额差冲正 "+in.ID, "2001", "1002", diff)
			} else {
				addLine("金额差补记 "+in.ID, "1002", "2001", diff)
			}
			continue
		}
		if in.Status != ch.Status {
			statusCnt++
			fmt.Printf("%s 状态不一致：内部 %s、渠道 %s → 以渠道为准补状态\n", in.ID, in.Status, ch.Status)
			addLine("退款补记 "+in.ID, "2001", "1002", in.Amt)
			continue
		}
		fmt.Printf("%s 一致：%s 元 %s\n", in.ID, yuan(in.Amt), in.Status)
	}
	for _, ch := range channel {
		if _, ok := innerMap[ch.ID]; !ok {
			longCnt++
			fmt.Printf("%s 长款：渠道 %s 元、内部无此笔 → 按渠道流水补单\n", ch.ID, yuan(ch.Amt))
			addLine("长款补单 "+ch.ID, "1002", "2002", ch.Amt)
		}
	}
	fmt.Printf("差异合计：长款 %d、短款 %d、金额 %d、状态 %d\n\n", longCnt, shortCnt, amtCnt, statusCnt)

	fmt.Println("== 2. 系统自动生成的差错处理凭证（每张都必须借贷相等）==")
	var drSum, crSum int64
	for _, l := range lines {
		drSum += l.Amt
		crSum += l.Amt
		fmt.Printf("%s %-16s 借 %s / 贷 %s  %s 元\n", l.Vid, l.Note, l.DrAcct, l.CrAcct, yuan(l.Amt))
	}
	fmt.Printf("凭证合计：借 %s = 贷 %s → 差异 %s\n\n", yuan(drSum), yuan(crSum), yuan(drSum-crSum))

	// 总分核对：总账科目余额应等于其下明细账户余额之和
	fmt.Println("== 3. 总分核对（总账 2001 客户存款 vs 明细账户合计）==")
	glBalance := int64(100000) // 总账余额
	detailSum := int64(99999)  // 明细账户逐户合计
	suspense := glBalance - detailSum
	fmt.Printf("总账余额 %s 元\n明细合计 %s 元\n差额     %s 元\n", yuan(glBalance), yuan(detailSum), yuan(suspense))
	if suspense != 0 {
		fmt.Printf("差额挂账：借 2001 %s / 贷 5002 待处理损益 %s（次日核销）\n",
			yuan(suspense), yuan(suspense))
	}
	fmt.Printf("挂账后校验：明细 %s + 挂账 %s = 总账 %s → %v\n\n",
		yuan(detailSum), yuan(suspense), yuan(glBalance), detailSum+suspense == glBalance)

	fmt.Println("== 4. 差错处理时限（经验值）==")
	for _, s := range []struct {
		t, what string
	}{
		{"T+1 03:00", "日终对账完成，差异写入挂账池（带原流水号与差异类型）"},
		{"T+1 06:00", "自动核查：查渠道单、查回调日志、查计费规则"},
		{"T+1 12:00", "自动核查不通过的转人工，双人复核"},
		{"T+1 18:00", "补单/冲正/确认无差异，全部闭环"},
		{"T+7", "复盘：同类差异是否重复出现，修上游根因"},
	} {
		fmt.Printf("%-9s %s\n", s.t, s.what)
	}
}
