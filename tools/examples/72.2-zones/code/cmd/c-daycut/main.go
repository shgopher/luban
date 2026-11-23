// 实测 C：同一批 UTC 瞬时，按不同时区聚合"每天计数"，看 DST 日差多少
package main

import (
	"fmt"
	"sort"
	"time"
)

var locs = []string{"UTC", "Asia/Shanghai", "America/New_York", "Europe/Berlin"}

func run(title string, start, end time.Time, step time.Duration) {
	fmt.Printf("\n## %s\n", title)
	fmt.Printf("窗口：%s ～ %s UTC，步长 %v\n\n", start.Format("2006-01-02T15:04Z"), end.Format("2006-01-02T15:04Z"), step)
	fmt.Println("| 时区 | 本地日期 | 计数 | 该日实际小时数 |")
	fmt.Println("|---|---|---|---|")
	for _, name := range locs {
		loc, err := time.LoadLocation(name)
		if err != nil {
			fmt.Printf("| %s | LOAD ERROR | | |\n", name)
			continue
		}
		count := map[string]int{}
		var order []string
		for t := start; t.Before(end); t = t.Add(step) {
			k := t.In(loc).Format("2006-01-02")
			if _, ok := count[k]; !ok {
				order = append(order, k)
			}
			count[k]++
		}
		sort.Strings(order)
		for _, k := range order {
			day, _ := time.ParseInLocation("2006-01-02", k, loc)
			h := dayHours(loc, day)
			fmt.Printf("| %s | %s | %d | %.1f |\n", name, k, count[k], h)
		}
	}
}

func main() {
	// 春季：覆盖 America/New_York 2026-03-08（23 小时）
	run("C1 春季 DST 窗口（15 分钟粒度）",
		time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), 15*time.Minute)
	// 秋季：覆盖 America/New_York 2026-11-01（25 小时）
	run("C2 秋季 DST 窗口（15 分钟粒度）",
		time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), 15*time.Minute)

	// C3：同一个瞬时在四个时区各是"哪一天"
	fmt.Println("\n## C3 同一瞬时在四个时区的本地日期（日切口径差一天）")
	fmt.Println("| UTC 瞬时 | UTC 日期 | Asia/Shanghai | America/New_York | Europe/Berlin |")
	fmt.Println("|---|---|---|---|---|")
	fmt.Println("| 2026-10-31T10:30Z | 10-31 | 10-31 18:30 | 10-31 06:30 | 10-31 11:30 |")
	instants := []time.Time{
		time.Date(2026, 11, 1, 15, 30, 0, 0, time.UTC),  // 上海 11-01 23:30（同一业务日）
		time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC),   // 上海 11-02 01:00；UTC 仍 11-01；NY 10-31 21:00
		time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC),    // NY 03:30（gap 之后）
		time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC),    // NY 01:30（gap 之前）
		time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC),   // NY 01:30 EST（overlap 第二次）
		time.Date(2026, 11, 1, 4, 30, 0, 0, time.UTC),   // NY 01:30 EDT（overlap 第一次）
	}
	for _, t := range instants {
		fmt.Printf("| %s | %s | %s | %s | %s |\n", t.Format("01-02T15:04Z"),
			t.UTC().Format("01-02"), t.In(must("Asia/Shanghai")).Format("01-02 15:04"),
			t.In(must("America/New_York")).Format("01-02 15:04"),
			t.In(must("Europe/Berlin")).Format("01-02 15:04"))
	}

	// C4：业务日切（营业日 00:00 Asia/Shanghai）下，同一批订单在三种口径的日计数
	fmt.Println("\n## C4 一批订单（每 20 分钟一笔、2026-10-31T16:00Z～11-02T16:00Z）按三种日切聚合")
	fmt.Println("| 日切口径 | 2026-10-31 | 2026-11-01 | 2026-11-02 | 极差 |")
	fmt.Println("|---|---|---|---|---|")
	start := time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 2, 16, 0, 0, 0, time.UTC)
	counts := map[string]map[string]int{}
	for _, name := range []string{"UTC", "Asia/Shanghai", "America/New_York"} {
		counts[name] = map[string]int{}
		loc := must(name)
		for t := start; t.Before(end); t = t.Add(20 * time.Minute) {
			counts[name][t.In(loc).Format("2006-01-02")]++
		}
	}
	for _, name := range []string{"UTC", "Asia/Shanghai", "America/New_York"} {
		a, b, c := counts[name]["2026-10-31"], counts[name]["2026-11-01"], counts[name]["2026-11-02"]
		max, min := a, a
		for _, v := range []int{a, b, c} {
			if v > max {
				max = v
			}
			if v < min {
				min = v
			}
		}
		fmt.Printf("| %s | %d | %d | %d | %d |\n", name, a, b, c, max-min)
	}
}

// 某本地日期的真实小时数：找「本地日期 >= d」和「>= d+1」两个 UTC 边界做差。
// 不能用 time.Date(...,0,0,0,0,loc)：Santiago 切换日的 00:00 不存在会被规范化。
func dayHours(loc *time.Location, day time.Time) float64 {
	ord := func(t time.Time) int {
		lt := t.In(loc)
		return int(time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, time.UTC).Sub(
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	}
	bound := func(target int) time.Time {
		lo := day.Add(-96 * time.Hour).Unix()
		hi := day.Add(96 * time.Hour).Unix()
		for lo < hi {
			mid := (lo + hi) / 2
			if ord(time.Unix(mid, 0)) >= target {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		return time.Unix(lo, 0)
	}
	k := ord(day.Add(12 * time.Hour))
	return bound(k+1).Sub(bound(k)).Hours()
}

func must(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
