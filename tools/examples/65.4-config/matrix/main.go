package main

// 版本矩阵算例：平台版本 P × 租户配置 schema 版本 C × 数据模型版本 D
// 输入是假设的场景参数（算例），不是任何真实客户数据。
// 核心结论：名义乘积 |P|×|C|×|D| 是上界；要验证的是"可达组合"。

import "fmt"

type platform struct {
	name    string
	cfgCap  int // 该平台版本能解析的最高配置 schema 版本（旧代码读不了新配置）
	dmFloor int // 能读的最老数据模型版本
	dmCeil  int // 能读的最新数据模型版本
}

func dmWidth(p platform) int { return p.dmCeil - p.dmFloor + 1 }

// reachable 按兼容规则过滤后的可达组合数
func reachable(ps []platform) int {
	n := 0
	for _, p := range ps {
		n += p.cfgCap * dmWidth(p)
	}
	return n
}

func hours(combos, minutes int) float64 { return float64(combos*minutes) / 60 }

func main() {
	const (
		totalPlatform = 4  // 一年 4 个 minor（季度发布火车）
		windowN       = 3  // 支持窗口：最近 3 个 minor
		cfgVersions   = 30 // 治理前：线上仍被租户使用的配置 schema 版本数
		dmVersions    = 6  // 数据模型（库 schema）版本数
		testMinutes   = 25 // 每个组合的回归验证成本（假设）
	)

	// 支持窗口内，从最新往前排：越老的平台认识的配置版本越少，
	// 因为新配置项是随新平台发布的。
	full := []platform{
		{"P0 最新", cfgVersions, dmVersions - 1, dmVersions},
		{"P1 上一版", 12, dmVersions - 2, dmVersions - 1},
		{"P2 上上版", 4, dmVersions - 3, dmVersions - 2},
		{"P3 窗口外", 2, dmVersions - 4, dmVersions - 3},
	}

	fmt.Printf("输入：平台一年 %d 个 minor（季度发布），支持窗口 N=%d，配置版本 |C|=%d，数据模型版本 |D|=%d，单组合验证 %.0f 分钟\n",
		totalPlatform, windowN, cfgVersions, dmVersions, float64(testMinutes))
	fmt.Println()

	fmt.Println("① 支持窗口长度 → 组合数（名义乘积 vs 可达组合）")
	fmt.Println("N   在线P   名义 |P|×|C|×|D|   可达组合   占比      全测工时")
	for n := 1; n <= 4; n++ {
		ps := full[:n]
		naive := n * cfgVersions * dmVersions
		r := reachable(ps)
		fmt.Printf("%d   %d      %6d            %6d    %5.1f%%   %6.1fh\n",
			n, n, naive, r, 100*float64(r)/float64(naive), hours(r, testMinutes))
	}
	fmt.Println()

	win := full[:windowN]
	naive := windowN * cfgVersions * dmVersions
	r := reachable(win)

	fmt.Println("② 治理前：|P|×|C|×|D| 与可达组合")
	fmt.Printf("   名义乘积 = %d × %d × %d = %d\n", len(win), cfgVersions, dmVersions, naive)
	fmt.Println("   每个平台版本实际能读的配置与数据模型范围：")
	fmt.Println("   平台版本      C 上限   D 范围     该版本组合数")
	sum := 0
	for _, p := range win {
		c := p.cfgCap * dmWidth(p)
		sum += c
		fmt.Printf("   %-12s  %4d    v%d..v%d     %4d\n", p.name, p.cfgCap, p.dmFloor, p.dmCeil, c)
	}
	fmt.Printf("   可达组合 = %d（名义乘积的 %.1f%%），全测工时 = %.1fh ≈ %.1f 人日\n",
		sum, 100*float64(sum)/float64(naive), hours(sum, testMinutes), hours(sum, testMinutes)/8)
	fmt.Println()

	// 治理：把线上在用的配置 schema 版本从 30 压到 8（可迁移的全部迁到受控版本）
	const cfgAfter = 8
	fixed := []platform{
		{"P0 最新", cfgAfter, dmVersions - 1, dmVersions},
		{"P1 上一版", cfgAfter, dmVersions - 2, dmVersions - 1},
		{"P2 上上版", 4, dmVersions - 3, dmVersions - 2},
	}
	r2 := reachable(fixed)
	fmt.Printf("③ 配置版本 %d → %d 之后\n", cfgVersions, cfgAfter)
	fmt.Printf("   可达组合 %d → %d，少 %d 个组合\n", sum, r2, sum-r2)
	fmt.Printf("   全测工时 %.1fh → %.1fh，省 %.1fh ≈ %.1f 人日\n",
		hours(sum, testMinutes), hours(r2, testMinutes),
		hours(sum-r2, testMinutes), hours(sum-r2, testMinutes)/8)
	fmt.Println()

	fmt.Println("④ 测试策略：全测 vs 抽样（按爆炸半径，不是按版本号顺序）")
	fullTest := len(win) * 2 // 每个平台版本 × 2 个数据模型边界 = 必须全测的契约跨越组合
	sampled := r - fullTest
	fmt.Printf("   必须全测：%d 个组合（每平台 × D 边界）= %.1fh\n", fullTest, hours(fullTest, testMinutes))
	fmt.Printf("   抽样 1/3：%d 个组合 → %d 个 = %.1fh\n", sampled, sampled/3, hours(sampled/3, testMinutes))
	fmt.Printf("   合计每次发布回归 ≈ %.1fh\n", hours(fullTest+sampled/3, testMinutes))
}
