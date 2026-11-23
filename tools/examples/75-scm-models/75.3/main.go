package main

import (
	"fmt"
	"math/rand"
	"sort"
)

const (
	skuN     = 100  // SKU 数
	orderN   = 500  // 订单数
	zones    = 6    // 库区数
	walkMPS  = 1.2  // 拣货员行走速度 m/s
	pickSec  = 8.0  // 每拣选行耗时
	setupOrd = 20.0 // 单订单拣选的准备/交接耗时
	setupWav = 30.0 // 每波次的准备/交接耗时
	waveSize = 8    // 波次合并订单数
)

// 库位到发货口的单程距离：0 区最近 10 m，每往内一区 +25 m
func binDist(zone int) float64 { return 10 + 25*float64(zone) }

type order struct{ lines []int } // SKU 下标

// Zipf 抽样：前 20% 的 SKU 承担约 70% 的拣选行（商品热度长尾）
func genOrders(seed int64) []order {
	r := rand.New(rand.NewSource(seed))
	cum := make([]float64, skuN)
	sum := 0.0
	for i := 0; i < skuN; i++ {
		sum += 1.0 / float64(i+1)
		cum[i] = sum
	}
	pickSKU := func() int { return sort.SearchFloat64s(cum, r.Float64()*sum) }
	os := make([]order, orderN)
	for i := range os {
		n := 1 + r.Intn(3)
		seen := map[int]bool{}
		for len(os[i].lines) < n {
			k := pickSKU()
			if seen[k] {
				continue
			}
			seen[k] = true
			os[i].lines = append(os[i].lines, k)
		}
	}
	return os
}

// 库位分配：随机；或 ABC——热 SKU 近、冷 SKU 远
func assignBins(random bool, seed int64) []int {
	r := rand.New(rand.NewSource(seed))
	bin := make([]int, skuN)
	if random {
		for i := range bin {
			bin[i] = r.Intn(zones)
		}
		return bin
	}
	for i := range bin {
		switch {
		case i < 20: // A 类：0-1 区
			bin[i] = i % 2
		case i < 50: // B 类：2-3 区
			bin[i] = 2 + i%2
		default: // C 类：4-5 区
			bin[i] = 4 + i%2
		}
	}
	return bin
}

type perf struct {
	walkPerOrder, linesPerOrder, secPerOrder, ordersPerHour, walkShare float64
}

// 单订单拣选（摘果式）：走到最远库位再原路返回
func evalSingle(orders []order, bin []int) perf {
	var dist, lines float64
	for _, o := range orders {
		maxD := 0.0
		for _, s := range o.lines {
			if d := binDist(bin[s]); d > maxD {
				maxD = d
			}
		}
		dist += 2 * maxD
		lines += float64(len(o.lines))
	}
	return finish(dist, lines, float64(len(orders)), setupOrd)
}

// 波次拣选：8 单合一波，同一库位只去一次、同一 SKU 只拣一次（播种墙分单）
func evalWave(orders []order, bin []int) perf {
	var dist, lines float64
	for i := 0; i < len(orders); i += waveSize {
		end := i + waveSize
		if end > len(orders) {
			end = len(orders)
		}
		wave := orders[i:end]
		uniq := map[int]bool{}
		maxD := 0.0
		for _, o := range wave {
			for _, s := range o.lines {
				uniq[s] = true
				if d := binDist(bin[s]); d > maxD {
					maxD = d
				}
			}
		}
		dist += 2 * maxD
		lines += float64(len(uniq))
	}
	return finish(dist, lines, float64(len(orders)), setupWav/waveSize)
}

func finish(dist, lines, n, setup float64) perf {
	walkPer := dist / n
	linesPer := lines / n
	walkSec := walkPer / walkMPS
	secPer := walkSec + linesPer*pickSec + setup
	return perf{walkPer, linesPer, secPer, 3600 / secPer, walkSec / secPer * 100}
}

func main() {
	orders := genOrders(2026)
	bRandom, bABC := assignBins(true, 7), assignBins(false, 7)

	p1 := evalSingle(orders, bRandom)
	p2 := evalSingle(orders, bABC)
	p3 := evalWave(orders, bABC)

	fmt.Printf("仓库：%d 个库区、%d 个 SKU、%d 张订单（每单 1-3 行，Zipf 热度）；行走 1.2 m/s、每行 8 s\n\n",
		zones, skuN, orderN)
	fmt.Printf("%-30s %-10s %-10s %-10s %-10s %s\n", "方案", "行走 m/单", "行数/单", "秒/单", "单/人时", "行走占比")
	rows := []struct {
		name string
		p    perf
	}{
		{"① 随机库位 + 单订单拣选", p1},
		{"② ABC 库位 + 单订单拣选", p2},
		{"③ ABC 库位 + 8 单波次", p3},
	}
	for _, r := range rows {
		fmt.Printf("%-30s %8.1f  %7.1f  %8.1f  %8.1f  %6.0f%%\n",
			r.name, r.p.walkPerOrder, r.p.linesPerOrder, r.p.secPerOrder, r.p.ordersPerHour, r.p.walkShare)
	}
	fmt.Printf("\n①→③ 提升 %.1f 倍：行走 %.0f→%.0f m/单，行数 %.1f→%.1f 行/单；单靠 ABC 只提升 %.0f%%\n",
		p3.ordersPerHour/p1.ordersPerHour, p1.walkPerOrder, p3.walkPerOrder,
		p1.linesPerOrder, p3.linesPerOrder, (p2.ordersPerHour/p1.ordersPerHour-1)*100)

	fmt.Println("\n盘点：5000 个库位，对比工作量与“差异从发生到被发现”的平均延迟：")
	fmt.Printf("%-32s %-12s %-12s %s\n", "策略", "每天盘位数", "全盘周期", "平均发现延迟")
	for _, c := range []struct {
		name      string
		perDay    int
		cycleDays float64
		delay     float64
	}{
		{"全盘：每月一次", 5000 / 30, 30, 15},
		{"循环盘点：A 类每周、其余按月", 250, 20, 10},
		{"动碰盘点：拣完即盘 + 差异日清", 400, 12.5, 1},
	} {
		fmt.Printf("%-32s %6d 个     %5.1f 天     %5.1f 天\n", c.name, c.perDay, c.cycleDays, c.delay)
	}
}
