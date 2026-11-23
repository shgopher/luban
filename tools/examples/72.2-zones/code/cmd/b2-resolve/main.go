// 把"墙上时间 + zone"显式解析成瞬时：gap/overlap 的候选集合必须自己算，
// 不依赖 time.Date 的未定义选择。
package main

import (
	"fmt"
	"sort"
	"time"
)

// candidates 返回该墙钟在 loc 中真实存在的所有瞬时（gap 时为空，overlap 时为 2 个）。
func candidates(loc *time.Location, y int, mo time.Month, d, hh, mm, ss int) []time.Time {
	want := time.Date(y, mo, d, hh, mm, ss, 0, time.UTC) // 仅作墙钟字段载体
	seen := map[int64]time.Time{}
	// 在 ±36 小时窗口里采样不同偏移，反推候选瞬时（overlap 的第二次出现离"假 UTC"可能差 13 小时以上）
	for k := -72; k <= 72; k++ {
		probe := want.Add(time.Duration(k) * 30 * time.Minute)
		_, off := probe.In(loc).Zone()
		cand := want.Add(-time.Duration(off) * time.Second)
		lt := cand.In(loc)
		if lt.Year() == y && lt.Month() == mo && lt.Day() == d &&
			lt.Hour() == hh && lt.Minute() == mm && lt.Second() == ss {
			seen[cand.Unix()] = cand
		}
	}
	var out []time.Time
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

func main() {
	cases := []struct {
		zone       string
		y          int
		mo         time.Month
		d, hh, mm  int
	}{
		{"America/New_York", 2026, 3, 8, 2, 30},
		{"Europe/Berlin", 2026, 3, 29, 2, 30},
		{"America/New_York", 2026, 11, 1, 1, 30},
		{"Europe/Berlin", 2026, 10, 25, 2, 30},
		{"Australia/Lord_Howe", 2026, 4, 5, 1, 30},
	}
	fmt.Println("## gap/overlap 显式解析：候选瞬时个数与内容")
	fmt.Println("| zone | 墙钟 | 候选数 | 候选 UTC |")
	fmt.Println("|---|---|---|---|")
	for _, c := range cases {
		loc, _ := time.LoadLocation(c.zone)
		cs := candidates(loc, c.y, c.mo, c.d, c.hh, c.mm, 0)
		var utc []string
		for _, t := range cs {
			utc = append(utc, t.UTC().Format("15:04:05Z"))
		}
		fmt.Printf("| %s | %04d-%02d-%02d %02d:%02d | %d | %v |\n",
			c.zone, c.y, c.mo, c.d, c.hh, c.mm, len(cs), utc)
	}
	fmt.Println("\n## 策略：gap 的三种显式行为（以 NY 2026-03-08 02:30 为例）")
	loc, _ := time.LoadLocation("America/New_York")
	cs := candidates(loc, 2026, 3, 8, 2, 30, 0)
	fmt.Printf("- 候选数=%d（墙钟不存在）\n", len(cs))
	// 跳过：不触发；顺延：取切换后的第一个合法瞬时 07:00Z（03:00 EDT）；提前：06:59:59Z
	transition := time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC)
	fmt.Printf("- 跳过：不触发\n- 顺延到 gap 结束：%s\n- 提前到 gap 开始：%s\n",
		transition.In(loc).Format("2006-01-02 15:04:05 MST"),
		transition.Add(-time.Second).In(loc).Format("2006-01-02 15:04:05 MST"))
}
