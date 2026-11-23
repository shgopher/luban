package main

import (
	"fmt"
	"math/rand"
)

var cities = []string{"上海", "杭州", "广州", "深圳", "北京", "成都"}

// 每行：上海仓、广州仓、天津仓到该城市的距离（km）
var dist = [][]int{
	{20, 1450, 1100},
	{180, 1250, 1250},
	{1450, 20, 2150},
	{1500, 140, 2200},
	{1200, 2150, 130},
	{2000, 1600, 1800},
}
var skus = []string{"A", "B", "C"}

// 各仓各 SKU 现存量：每个仓都有一个 SKU 偏少，制造“整单发不全”
var initial = [][]int{
	{60, 900, 900}, // 华东：A 少
	{900, 60, 900}, // 华南：B 少
	{900, 900, 60}, // 华北：C 少
}
var stock [][]int

type line struct {
	sku string
	qty int
}

type order struct {
	id      int
	city    int
	lines   []line
	pkgNo   int
	days    int
	freight float64 // 包裹成本（含距离运费与操作费）
	missed  bool
}

// 时效分档：≤50km 次日、≤500km 2 天、≤1500km 3 天、更远 4 天
func daysOf(km int) int {
	switch {
	case km <= 50:
		return 1
	case km <= 500:
		return 2
	case km <= 1500:
		return 3
	}
	return 4
}

// 单个包裹：首重 8 + 续重 2 元/kg（每件 0.5kg）+ 距离 0.006 元/km·件 + 操作 3 元/件
func parcelCost(km, qty int) float64 {
	return 8 + 2*float64(qty)*0.5 + 0.006*float64(km)*float64(qty) + 3*float64(qty)
}

func idx(s string) int {
	for i, v := range skus {
		if v == s {
			return i
		}
	}
	return -1
}

func genOrders(n int, seed int64) []order {
	r := rand.New(rand.NewSource(seed))
	os := make([]order, n)
	for i := range os {
		os[i].id = i + 1
		os[i].city = r.Intn(len(cities))
		nl := 1 + r.Intn(3)
		used := map[int]bool{}
		for len(os[i].lines) < nl {
			k := r.Intn(3)
			if used[k] {
				continue
			}
			used[k] = true
			os[i].lines = append(os[i].lines, line{skus[k], 1 + r.Intn(4)})
		}
	}
	return os
}

// 策略 A：单仓优先——只找“能整单发全”的仓，按送达最快选；找不到就挂起（不拆单）
func routeSingle(o *order) {
	best, bestDays := -1, 99
	for w := 0; w < 3; w++ {
		ok := true
		for _, l := range o.lines {
			if stock[w][idx(l.sku)] < l.qty {
				ok = false
				break
			}
		}
		if ok && daysOf(dist[o.city][w]) < bestDays {
			best, bestDays = w, daysOf(dist[o.city][w])
		}
	}
	if best < 0 {
		o.missed, o.days = true, 99
		return
	}
	o.pkgNo, o.days = 1, bestDays
	for _, l := range o.lines {
		stock[best][idx(l.sku)] -= l.qty
		o.freight += parcelCost(dist[o.city][best], l.qty)
	}
}

// 策略 B：时效优先——每行独立选“送达最快且有货”的仓，允许拆单
func routeFast(o *order) {
	chosen := map[int]bool{}
	for _, l := range o.lines {
		wi, wd := -1, 99
		for w := 0; w < 3; w++ {
			if stock[w][idx(l.sku)] < l.qty {
				continue
			}
			if d := daysOf(dist[o.city][w]); d < wd {
				wi, wd = w, d
			}
		}
		if wi < 0 {
			o.missed, o.days = true, 99
			return
		}
		chosen[wi] = true
		stock[wi][idx(l.sku)] -= l.qty
		o.freight += parcelCost(dist[o.city][wi], l.qty)
		if wd > o.days {
			o.days = wd
		}
	}
	o.pkgNo = len(chosen)
	if o.pkgNo > 1 {
		o.freight += 12 * float64(o.pkgNo-1) // 拆单附加操作费
	}
}

// 策略 C：成本优先——包裹成本 + 每多 1 个包裹加 12 元拆单费 + 每超 1 天罚 30 元
func routeCost(o *order) {
	bestTotal := 1e18
	bestDays, bestPkg := 99, 0
	var bestAssign []int
	for mask := 1; mask < 8; mask++ {
		assign := make([]int, len(o.lines))
		cost := 0.0
		days, ok := 0, true
		for i, l := range o.lines {
			wi, wd := -1, 99
			for w := 0; w < 3; w++ {
				if mask&(1<<w) == 0 || stock[w][idx(l.sku)] < l.qty {
					continue
				}
				if d := daysOf(dist[o.city][w]); d < wd {
					wi, wd = w, d
				}
			}
			if wi < 0 {
				ok = false
				break
			}
			assign[i] = wi
			cost += parcelCost(dist[o.city][wi], l.qty)
			if wd > days {
				days = wd
			}
		}
		if !ok {
			continue
		}
		used := map[int]bool{}
		for _, w := range assign {
			used[w] = true
		}
		pkgs := len(used)
		if pkgs > 1 {
			cost += 12 * float64(pkgs-1)
		}
		if days > 2 {
			cost += 30 * float64(days-2)
		}
		if cost < bestTotal {
			bestTotal, bestDays, bestPkg, bestAssign = cost, days, pkgs, assign
		}
	}
	if bestPkg == 0 {
		o.missed, o.days = true, 99
		return
	}
	o.pkgNo, o.days = bestPkg, bestDays
	for i, l := range o.lines {
		o.freight += parcelCost(dist[o.city][bestAssign[i]], l.qty)
		stock[bestAssign[i]][idx(l.sku)] -= l.qty
	}
	if bestPkg > 1 {
		o.freight += 12 * float64(bestPkg-1)
	}
}

type agg struct {
	name    string
	freight float64
	pkgs    int
	splits  int
	late    int
	pen     float64
	missed  int
}

func run(name string, fn func(*order)) agg {
	stock = [][]int{{60, 900, 900}, {900, 60, 900}, {900, 900, 60}}
	os := genOrders(200, 11)
	a := agg{name: name}
	ok := 0
	for i := range os {
		fn(&os[i])
		if os[i].missed {
			a.missed++
			continue
		}
		ok++
		a.freight += os[i].freight
		a.pkgs += os[i].pkgNo
		if os[i].pkgNo > 1 {
			a.splits++
		}
		if os[i].days > 2 {
			a.late++
			a.pen += 30 * float64(os[i].days-2)
		}
	}
	a.freight /= float64(ok)
	return a
}

func main() {
	fmt.Println("200 单、3 仓、3 SKU；承诺 2 天达；运费 = 首重 8 + 续重 2/kg + 0.006 元/km·件 + 操作 3/件")
	fmt.Printf("%-12s %-12s %-13s %-7s %-7s %-12s %-10s %s\n",
		"策略", "运费/单", "单均总成本", "包裹数", "拆单数", "超 2 天单数", "违约金", "未履约单")
	rows := []struct {
		name string
		fn   func(*order)
	}{{"A 单仓优先", routeSingle}, {"B 时效优先", routeFast}, {"C 成本优先", routeCost}}
	for _, r := range rows {
		a := run(r.name, r.fn)
		total := (a.freight*float64(200-a.missed) + a.pen) / 200
		fmt.Printf("%-12s %7.1f 元   %7.1f 元    %5d   %5d      %5d        %4.0f 元    %d\n",
			a.name, a.freight, total, a.pkgs, a.splits, a.late, a.pen, a.missed)
	}

	fmt.Println("\n库存分配口径（SKU Z：现存量 100，已被未支付订单预占 30，在途 0，每单要 3 件）：")
	onHand, reserved := 100, 30
	for _, mode := range []string{"只看现存量", "看 ATP（现存量-预占+在途）"} {
		avail := onHand
		if mode == "看 ATP（现存量-预占+在途）" {
			avail = onHand - reserved
		}
		passed := avail / 3
		shippable := (onHand - reserved) / 3
		fmt.Printf("  %-26s 放行 %2d 单，实际可发 %2d 单，超卖 %2d 单（%d 件）\n",
			mode, passed, shippable, passed-shippable, (passed-shippable)*3)
	}
}
