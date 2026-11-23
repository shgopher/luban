// go run plan.go：1000 个租户按 tier 分层、按桶号排批次，算每批的影响面。
package main

import (
	"fmt"
	"sort"
)

type T struct {
	id, users int
	tier      string
	qps, acct float64 // qps：平均请求/秒；acct：全挂时每天损失的营收（USD）
}

func h(s string) float64 { // 桶号 = FNV-1a(tenant_id) % 10000；桶号固定，批次只是桶区间
	x := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		x = (x ^ uint32(s[i])) * 16777619
	}
	return float64(x%10000) / 10000
}

func main() {
	tiers := []struct {
		name      string
		n, lo, hi int
		per, acct float64
	}{
		{"internal", 6, 40, 40, .015, 0},
		{"friendly", 6, 180, 260, .016, 120},
		{"small", 640, 3, 30, .010, 60},
		{"mid", 288, 60, 300, .015, 400},
		{"large", 46, 500, 2000, .020, 2500},
		{"flagship", 14, 3000, 9000, .030, 12000},
	}
	var ts []T
	by := map[string][]T{}
	for _, tr := range tiers {
		for i := 0; i < tr.n; i++ {
			u := tr.lo + int(h(fmt.Sprintf("%s-%d", tr.name, i))*float64(tr.hi-tr.lo+1))
			t := T{len(ts), u, tr.name, float64(u) * tr.per, tr.acct}
			ts = append(ts, t)
			by[tr.name] = append(by[tr.name], t)
		}
	}
	for k := range by {
		sort.Slice(by[k], func(a, b int) bool {
			return h(fmt.Sprint(k, by[k][a].id)) < h(fmt.Sprint(k, by[k][b].id))
		})
	}
	pick := func(tier string, lo, hi float64) (ids []int) {
		l := by[tier]
		for i := int(lo * float64(len(l))); i < int(hi*float64(len(l))); i++ {
			ids = append(ids, l[i].id)
		}
		return
	}
	type B struct {
		name, rule string
		ids        []int
	}
	bs := []B{
		{"B0", "内部租户 dogfood", pick("internal", 0, 1)},
		{"B1", "白名单友好客户 + 小客户前 10%", append(pick("friendly", 0, 1), pick("small", 0, .10)...)},
		{"B2", "小客户剩余 90%", pick("small", .10, 1)},
		{"B3", "中型客户前 15%", pick("mid", 0, .15)},
		{"B4", "中型客户剩余 85%", pick("mid", .15, 1)},
		{"B5", "大型客户 100%", pick("large", 0, 1)},
		{"B6", "旗舰客户 100%", pick("flagship", 0, 1)},
	}
	idx := map[int]T{}
	var totalQPS, totalUsers float64
	for _, t := range ts {
		idx[t.id], totalQPS, totalUsers = t, totalQPS+t.qps, totalUsers+float64(t.users)
	}
	qps, risk := map[string]float64{}, map[string]float64{}
	fmt.Println("| 批次 | 放量内容 | 租户数 | 租户占比 | 流量占比 | 影响用户数 | 用户占比 | 预期工单数* | 单日营收在险(USD) |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	for _, b := range bs {
		var q, u, m float64
		for _, id := range b.ids {
			q, u, m = q+idx[id].qps, u+float64(idx[id].users), m+idx[id].acct
		}
		qps[b.name], risk[b.name] = q, m
		fmt.Printf("| %s | %s | %d | %.1f%% | %.1f%% | %.0f | %.1f%% | %.0f | %.0f |\n",
			b.name, b.rule, len(b.ids), float64(len(b.ids))/10, q/totalQPS*100, u, u/totalUsers*100, float64(len(b.ids))*.6, m)
	}
	byQPS := append([]B(nil), bs...)
	sort.Slice(byQPS, func(a, b int) bool { return qps[byQPS[a].name] > qps[byQPS[b].name] })
	fmt.Printf("\n按流量降序（错）：第一批 = %s，%d 租户、%.1f%% 流量、单日营收在险 %.0f USD\n",
		byQPS[0].name, len(byQPS[0].ids), qps[byQPS[0].name]/totalQPS*100, risk[byQPS[0].name])
	fmt.Printf("按爆炸半径 × 可承受损失（对）：第一批 = B0，%d 租户、%.2f%% 流量、营收在险 0 USD\n",
		len(bs[0].ids), qps["B0"]/totalQPS*100)
}
