package main

import (
	"fmt"
	"time"
)

type Limit struct{ Single, Day, Month int64 } // 元

type Txn struct {
	ID       string
	Cust     string
	Card     string
	Hour     int
	Amt      int64
	NewPayee bool
}

var limits = map[string]Limit{
	"C001": {Single: 50000, Day: 200000, Month: 1000000},
	"C002": {Single: 20000, Day: 50000, Month: 200000},
	"C003": {Single: 100000, Day: 500000, Month: 5000000},
}

var blackCards = map[string]bool{"6222****9999": true}
var grayCust = map[string]bool{"C002": true}
var whiteCust = map[string]bool{"C003": true}

// decide 返回决策与原因码；拒绝级原因优先于复核级原因。
func decide(t Txn, lim Limit, dayUsed, monthUsed int64) (string, []string) {
	var deny, review []string
	if blackCards[t.Card] {
		deny = append(deny, "R01 黑名单-卡号")
	}
	if t.Amt > lim.Single {
		deny = append(deny, "R02 超单笔限额")
	}
	if dayUsed+t.Amt > lim.Day {
		deny = append(deny, "R03 超日累计限额")
	}
	if monthUsed+t.Amt > lim.Month {
		deny = append(deny, "R04 超月累计限额")
	}
	// 白名单只免“名单类”拦截，限额与行为规则照常执行
	if !whiteCust[t.Cust] && grayCust[t.Cust] && t.Amt > 10000 {
		review = append(review, "R11 灰名单+大额")
	}
	if t.Hour < 6 && t.Amt > 10000 {
		review = append(review, "R12 深夜大额")
	}
	if t.NewPayee && t.Amt > 30000 {
		review = append(review, "R13 新对手方大额")
	}
	switch {
	case len(deny) > 0:
		return "拒绝", deny
	case len(review) > 0:
		return "复核", review
	}
	return "通过", []string{"R99 无命中"}
}

func main() {
	txns := []Txn{
		{"F01", "C001", "6222****0001", 14, 8000, false},
		{"F02", "C001", "6222****0001", 14, 60000, false}, // 超单笔
		{"F03", "C001", "6222****0001", 14, 45000, false}, // 当日累计 8000+45000=53000，未超 200000
		{"F04", "C001", "6222****0001", 14, 155000, true}, // 日累计 208000 > 200000
		{"F05", "C002", "6222****0002", 14, 18000, false}, // 灰名单 + 大额(>10000)，未超单笔
		{"F06", "C002", "6222****0002", 14, 9000, false},  // 灰名单但小额
		{"F07", "C003", "6222****0003", 2, 80000, true},   // 白名单，深夜大额 + 新对手方
		{"F08", "C003", "6222****0003", 2, 20000, false},  // 深夜大额（>10000）
		{"F09", "C001", "6222****9999", 14, 100, false},   // 黑名单卡号
		{"F10", "C001", "6222****0001", 14, 4000, true},   // 新对手方但小额
	}

	used := map[string]int64{} // 当日累计
	month := map[string]int64{"C001": 300000, "C002": 120000, "C003": 900000}
	var pass, review, deny int
	var totalNs int64
	fmt.Println("== 1. 逐笔实时决策（规则 + 名单 + 限额）==")
	fmt.Printf("%-4s %-5s %-7s %-4s %9s  %-4s %-22s %s\n", "单号", "客户", "时间", "金额", "日累计后", "决策", "原因码", "耗时")
	for _, t := range txns {
		lim := limits[t.Cust]
		start := time.Now()
		dec, reasons := decide(t, lim, used[t.Cust], month[t.Cust])
		cost := time.Since(start)
		totalNs += cost.Nanoseconds()
		after := used[t.Cust] + t.Amt
		switch dec {
		case "通过":
			pass++
			used[t.Cust] = after // 只有通过才占用额度
		case "复核":
			review++
		case "拒绝":
			deny++
		}
		fmt.Printf("%-4s %-5s %02d:00   %9d %9d  %-4s %-22s %v\n",
			t.ID, t.Cust, t.Hour, t.Amt, after, dec, joinReasons(reasons), cost)
	}
	fmt.Printf("决策分布：通过 %d、复核 %d、拒绝 %d（共 %d 笔）\n", pass, review, deny, len(txns))
	fmt.Printf("单笔决策平均耗时：%.2f µs（本机计算，仅含规则匹配，不含 IO/模型打分）\n\n",
		float64(totalNs)/float64(len(txns))/1000)

	fmt.Println("== 2. 各客户当日额度占用（只有“通过”才占用）==")
	for _, c := range []string{"C001", "C002", "C003"} {
		fmt.Printf("%s 日累计 %7d / 限额 %7d（%.1f%%）\n", c, used[c], limits[c].Day,
			float64(used[c])*100/float64(limits[c].Day))
	}

	fmt.Println("\n== 3. 决策级联判据（从便宜到昂贵，命中即止）==")
	for _, s := range []struct {
		layer, cost, hit string
	}{
		{"名单（卡号/证件/设备）", "微秒级，内存表", "R01 黑名单直接拒绝，不进模型"},
		{"限额（单笔/日/月）", "微秒级，缓存计数", "R02-R04 硬约束，无解释空间"},
		{"行为规则（时段/新对手方）", "微秒级，特征直读", "R11-R13 转复核，不直接拒绝"},
		{"模型评分", "数毫秒到数十毫秒", "只在前面全不命中时打分，分数进复核队列"},
		{"人工复核", "分钟到小时", "复核结论回写名单与阈值"},
	} {
		fmt.Printf("%-24s %-14s %s\n", s.layer, s.cost, s.hit)
	}
}

func joinReasons(rs []string) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += "+"
		}
		out += r
	}
	return out
}
