// 计量聚合器：验证「幂等去重 + 按业务时间归期 + 迟到单独计数 + 账单可重放」
// 纯标准库：go run meter.go
package main

import (
	"fmt"
	"sort"
	"time"
)

type Event struct {
	EventID   string
	TenantID  string
	Dimension string
	Qty       int64
	Occurred  time.Time // 业务时间：决定归哪个账期
	Received  time.Time // 到达时间：决定是否已过封账时间
	Source    string
}

func ts(month, day, hour, min int) time.Time {
	return time.Date(2026, time.Month(month), day, hour, min, 0, 0, time.UTC)
}

// 2026-10 账期在 2026-11-04 00:00 封账（T+3 grace，含 11-03 全天）
var closedAt = map[string]time.Time{"2026-10": ts(11, 4, 0, 0)}

func periodOf(e Event) string { return e.Occurred.Format("2006-01") }

// 到达时间 >= 封账时间 → 该账期已出过账单，事件只能走补记
func isClosed(period string, recv time.Time) bool {
	t, ok := closedAt[period]
	return ok && !recv.Before(t)
}

type key struct{ tenant, period string }

type Agg struct {
	seen   map[string]bool
	usage  map[key]int64 // (租户, 账期) → 用量
	adjust map[key]int64 // 已封账账期的迟到补记
	dups   int
	late   int
	ooo    int // 乱序到达次数：业务时间早于上一条已接受事件
	last   time.Time
	dedupe func(Event) string
}

func newAgg(dedupe func(Event) string) *Agg {
	return &Agg{seen: map[string]bool{}, usage: map[key]int64{}, adjust: map[key]int64{}, dedupe: dedupe}
}

func (a *Agg) Add(e Event) string {
	k := a.dedupe(e)
	if a.seen[k] {
		a.dups++
		return "重复→丢弃"
	}
	a.seen[k] = true
	p := periodOf(e)
	kk := key{e.TenantID, p}
	if isClosed(p, e.Received) {
		a.adjust[kk] += e.Qty
		a.late++
		return "迟到→补记(调整单)"
	}
	a.usage[kk] += e.Qty
	if !a.last.IsZero() && e.Occurred.Before(a.last) {
		a.ooo++
	}
	a.last = e.Occurred
	return "计入账期 " + p
}

// 正确幂等键：租户 + 事件 ID
func tenantKey(e Event) string { return e.TenantID + "|" + e.EventID }

// 错误幂等键：只有事件 ID
func idOnly(e Event) string { return e.EventID }

func sortedUsage(m map[key]int64) []key {
	ks := make([]key, 0, len(m))
	for k, v := range m {
		if v != 0 {
			ks = append(ks, k)
		}
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].tenant != ks[j].tenant {
			return ks[i].tenant < ks[j].tenant
		}
		return ks[i].period < ks[j].period
	})
	return ks
}

func main() {
	feed := []Event{
		{"e-001", "t-001", "api_calls", 100, ts(10, 5, 10, 0), ts(10, 5, 10, 0), "gateway"},
		{"e-002", "t-001", "api_calls", 200, ts(10, 20, 11, 0), ts(10, 20, 11, 0), "gateway"},
		{"e-002", "t-001", "api_calls", 200, ts(10, 20, 11, 0), ts(10, 20, 11, 0), "gateway"}, // 上游重试
		{"e-101", "t-002", "api_calls", 70, ts(10, 15, 9, 0), ts(10, 15, 9, 0), "gateway"},
		{"e-001", "t-002", "api_calls", 100, ts(10, 6, 9, 0), ts(10, 6, 9, 0), "openapi"},     // 跨租户同 id
		{"e-003", "t-001", "api_calls", 50, ts(10, 31, 23, 58), ts(11, 5, 9, 0), "batch-sdk"}, // 迟到 2 天
		{"e-003", "t-001", "api_calls", 50, ts(10, 31, 23, 58), ts(11, 5, 9, 5), "batch-sdk"}, // 迟到事件的重复投递
		{"e-004", "t-001", "api_calls", 300, ts(11, 10, 8, 0), ts(11, 10, 8, 0), "gateway"},
		{"e-005", "t-001", "api_calls", 120, ts(11, 2, 9, 0), ts(11, 3, 10, 0), "gateway"}, // 乱序到达
	}

	fmt.Println("== 事件流（按到达顺序喂入，含重复、迟到、乱序）==")
	fmt.Printf("%-3s %-6s %-6s %-16s %-16s %5s  %s\n", "#", "租户", "事件ID", "业务时间 occurred", "到达时间 received", "数量", "处理结果")
	agg := newAgg(tenantKey)
	for i, e := range feed {
		st := agg.Add(e)
		fmt.Printf("%-3d %-6s %-6s %-16s %-16s %5d  %s\n", i+1, e.TenantID, e.EventID,
			e.Occurred.Format("2006-01-02 15:04"), e.Received.Format("2006-01-02 15:04"), e.Qty, st)
	}

	fmt.Println("\n== 按业务时间归期（2026-10 账期已于 2026-11-04 00:00 封账）==")
	for _, k := range sortedUsage(agg.usage) {
		fmt.Printf("%s  %s  api_calls = %d\n", k.tenant, k.period, agg.usage[k])
	}
	fmt.Printf("迟到补记（不改已发账单）：")
	for _, k := range sortedUsage(agg.adjust) {
		fmt.Printf(" %s %s = %d", k.tenant, k.period, agg.adjust[k])
	}
	fmt.Printf("\n统计：输入 %d 条；重复丢弃 %d 条；迟到 %d 条；乱序到达 %d 次；计入账期 %d 条\n",
		len(feed), agg.dups, agg.late, agg.ooo, len(feed)-agg.dups-agg.late)

	// 账单重算：全量重放事件日志（按业务时间排序），结果必须与增量聚合一致
	replay := newAgg(tenantKey)
	sorted := append([]Event(nil), feed...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Occurred.Before(sorted[j].Occurred) })
	for _, e := range sorted {
		replay.Add(e)
	}
	same := true
	for k, v := range agg.usage {
		if replay.usage[k] != v {
			same = false
		}
	}
	for k, v := range agg.adjust {
		if replay.adjust[k] != v {
			same = false
		}
	}
	fmt.Println("\n== 账单重算（封账前全量重放事件日志，按业务时间排序喂入）==")
	fmt.Printf("重放：t-001 2026-10 = %d，t-001 2026-11 = %d，t-002 2026-10 = %d，2026-10 补记 = %d\n",
		replay.usage[key{"t-001", "2026-10"}], replay.usage[key{"t-001", "2026-11"}],
		replay.usage[key{"t-002", "2026-10"}], replay.adjust[key{"t-001", "2026-10"}])
	fmt.Printf("与增量聚合一致 = %v\n", same)

	// 反例：幂等键漏掉租户维度
	bad := newAgg(idOnly)
	for _, e := range feed {
		bad.Add(e)
	}
	fmt.Println("\n== 反例：幂等键只用 event_id（漏 tenant_id）==")
	fmt.Printf("正确 key (tenant|event_id)：t-002 2026-10 = %d\n", agg.usage[key{"t-002", "2026-10"}])
	fmt.Printf("错误 key (event_id)       ：t-002 2026-10 = %d  ← 少收 %d 次（占 t-002 十月用量的 %.1f%%）\n",
		bad.usage[key{"t-002", "2026-10"}],
		agg.usage[key{"t-002", "2026-10"}]-bad.usage[key{"t-002", "2026-10"}],
		float64(agg.usage[key{"t-002", "2026-10"}]-bad.usage[key{"t-002", "2026-10"}])*100/float64(agg.usage[key{"t-002", "2026-10"}]))

	// 封账后直接改数 vs 出调整单
	issued := agg.usage[key{"t-001", "2026-10"}]
	late := agg.adjust[key{"t-001", "2026-10"}]
	fmt.Println("\n== 封账后直接改数 vs 出调整单 ==")
	fmt.Printf("已发账单 t-001 2026-10 = %d；把迟到 %d 直接并进 10 月 = %d，差额 %.1f%%\n",
		issued, late, issued+late, float64(late)*100/float64(issued))
}
