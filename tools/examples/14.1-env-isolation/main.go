// 14.1 多环境隔离 · 隔离成本/风险计算器
// 四档隔离（共享 / 独立账号 / 独立 VPC / 独立集群+独立账号）在给定环境数下的
// 年成本、故障爆炸半径、年期望损失与年总成本，并给出盈亏平衡环境数。
// 所有金额单位：元/月（月成本）或 元/年（年成本）；参数为示例值，可替换后重算。
package main

import (
	"fmt"
	"math"
)

// 示例参数（单位：元）
const (
	envCost     = 8000.0   // 每环境基础资源（计算/存储/带宽）
	sharedMgmt  = 3000.0   // 共享管理面（堡垒机/日志/监控/镜像仓库）
	accountCost = 500.0    // 每环境“独立账号”增量
	vpcCost     = 1500.0   // 每环境“独立 VPC”增量（NAT/VPN/对等连接）
	clusterCost = 3000.0   // 每环境“独立集群”增量
	pYear       = 0.25     // 单环境每年发生一次“凭证/主机失陷”事故的概率
	lossPerEnv  = 300000.0 // 单环境一次事故的直接损失
)

// 每环境年风险 = 概率 × 单环境损失
var riskPerEnv = pYear * lossPerEnv

type tier struct {
	name    string
	fixed   float64 // 与环境数无关的月成本
	perEnv  float64 // 每环境月成本
	radiusF func(n int) int
	note    string
}

// 四档：每档只加一个“隔离杠杆”
var tiers = []tier{
	{"共享账号+共享VPC", sharedMgmt, envCost, func(n int) int { return n }, "网络与凭证都共享"},
	{"独立账号（同VPC）", sharedMgmt, envCost + accountCost, func(n int) int { return n }, "凭证独立，网络仍通"},
	{"独立VPC（共享账号）", sharedMgmt, envCost + vpcCost, func(n int) int { return n }, "网络独立，凭证仍共享"},
	{"独立集群+独立账号", 0, envCost + accountCost + vpcCost + clusterCost, func(n int) int { return 1 }, "网络与凭证都独立"},
}

func monthly(t tier, n int) float64 { return t.fixed + t.perEnv*float64(n) }

// annualTotal：年成本 + 年期望损失（期望损失 = 每环境风险 × 爆炸半径）
func annualTotal(t tier, n int) float64 {
	r := t.radiusF(n)
	return monthly(t, n)*12 + riskPerEnv*float64(r)
}

func wan(v float64) float64 { return v / 10000 } // 元 → 万元

func main() {
	fmt.Println("== 参数（示例值，单位：元） ==")
	fmt.Printf("  每环境资源 %.0f/月；共享管理面 %.0f/月；独立账号 +%.0f/月；独立 VPC +%.0f/月；独立集群 +%.0f/月\n",
		envCost, sharedMgmt, accountCost, vpcCost, clusterCost)
	fmt.Printf("  单环境事故概率 %.0f%%/年；单次事故损失 %.0f 元/环境 → 每环境年风险 %.0f 元\n",
		pYear*100, lossPerEnv, riskPerEnv)

	n := 3 // 以 3 套环境（dev/staging/prod）为例
	fmt.Printf("\n== 四档隔离矩阵（环境数 n=%d） ==\n", n)
	fmt.Printf("%-18s %-10s %-10s %-8s %-12s %-12s %s\n",
		"档位", "月成本/万", "年成本/万", "爆炸半径", "年期望损失/万", "年总成本/万", "相对共享档")
	base := annualTotal(tiers[0], n)
	for _, t := range tiers {
		m := monthly(t, n)
		r := t.radiusF(n)
		total := annualTotal(t, n)
		fmt.Printf("%-18s %-10.1f %-10.1f %-8d %-12.1f %-12.1f %+.1f 万\n",
			t.name, wan(m), wan(m*12), r, wan(riskPerEnv*float64(r)), wan(total), wan(total-base))
	}

	fmt.Println("\n== 环境数敏感表：共享档 vs 全隔离档（年总成本/万） ==")
	fmt.Printf("%-6s %-14s %-14s %-10s %s\n", "环境数", "共享账号+共享VPC", "独立集群+独立账号", "更优档位", "差额/万")
	for k := 2; k <= 8; k++ {
		a := annualTotal(tiers[0], k)
		b := annualTotal(tiers[3], k)
		better, diff := "共享档", a-b
		if b < a {
			better, diff = "全隔离", a-b
		}
		fmt.Printf("%-6d %-14.1f %-14.1f %-10s %.1f\n", k, wan(a), wan(b), better, wan(diff))
	}

	// 盈亏平衡：全隔离档与共享档的年总成本相等时的环境数
	// 年成本差 = 12*(perEnv3 - perEnv0)*n + 12*fixed3 - 12*fixed0；风险节省 = riskPerEnv*(n-1)
	costPerEnvDiff := 12 * (tiers[3].perEnv - tiers[0].perEnv)
	fixedDiff := 12 * (tiers[3].fixed - tiers[0].fixed) // 为负：全隔离没有共享管理面
	breakEven := (riskPerEnv + fixedDiff) / (riskPerEnv - (tiers[3].perEnv-tiers[0].perEnv)*12)
	if breakEven < 0 {
		breakEven = 0
	}
	fmt.Printf("\n== 盈亏平衡 ==\n")
	fmt.Printf("  每多一套环境，全隔离档比共享档每年多花 %.0f 元，但少担 %.0f 元期望损失\n",
		costPerEnvDiff, riskPerEnv)
	fmt.Printf("  解得盈亏平衡环境数 n* = %.2f → 环境数 ≥ %d 时，全隔离档的年总成本更低\n",
		breakEven, int(math.Ceil(breakEven)))

	// 反推：n=3 时，单次事故损失要多大，全隔离才划算？
	// 风险节省 = 2*P*loss，需 > 年成本差
	costDiff3 := annualTotal(tiers[3], 3) - annualTotal(tiers[0], 3) + 2*riskPerEnv
	needLoss := costDiff3 / (2 * pYear)
	fmt.Printf("  n=3 时：单次事故损失超过 %.0f 元（概率 %.0f%%/年）全隔离才开始划算\n", needLoss, pYear*100)
}
