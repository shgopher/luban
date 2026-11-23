// 实测 A：2026 年各时区的偏移切换点 + 每个本地"天"的实际小时数
package main

import (
	"fmt"
	"sort"
	"time"
)

var zones = []string{
	"America/New_York",
	"Europe/Berlin",
	"Australia/Lord_Howe",
	"Pacific/Chatham",
	"America/Santiago",
	"Asia/Shanghai",
}

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func dayOrd(loc *time.Location, t time.Time) int {
	lt := t.In(loc)
	dt := time.Date(lt.Year(), lt.Month(), lt.Day(), 0, 0, 0, 0, time.UTC)
	return int(dt.Sub(base).Hours() / 24)
}

// 本地日期序号 target 的第一个 UTC 瞬时。二分而不是 time.Date(...,0,0,0,0,loc)：
// Santiago 的 00:00 在切换日不存在，time.Date 会被规范化，日界就算错了。
func boundary(loc *time.Location, target int, around time.Time) time.Time {
	lo := around.Add(-96 * time.Hour).Unix()
	hi := around.Add(96 * time.Hour).Unix()
	for lo < hi {
		mid := (lo + hi) / 2
		if dayOrd(loc, time.Unix(mid, 0)) >= target {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return time.Unix(lo, 0)
}

func main() {
	fmt.Println("## A1 2026 年偏移切换点（UTC 切换时刻 / 本地时刻 / 偏移 / 缩写）")
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			fmt.Printf("| %s | LOAD ERROR: %v |\n", name, err)
			continue
		}
		fmt.Printf("\n### %s\n", name)
		fmt.Println("| UTC 切换时刻 | 切换前本地 | 偏移前 | 缩写 | 切换后本地 | 偏移后 | 缩写 | Δ |")
		fmt.Println("|---|---|---|---|---|---|---|---|")
		t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for {
			_, zb := t.In(loc).ZoneBounds()
			if zb.IsZero() || !zb.Before(end) {
				break
			}
			beforeT := zb.Add(-time.Second)
			beforeName, beforeOff := beforeT.In(loc).Zone()
			afterName, afterOff := zb.In(loc).Zone()
			fmt.Printf("| %s | %s | %+d s | %s | %s | %+d s | %s | %+d min |\n",
				zb.UTC().Format("2006-01-02T15:04:05Z"),
				beforeT.In(loc).Format("2006-01-02 15:04:05"),
				beforeOff, beforeName,
				zb.In(loc).Format("2006-01-02 15:04:05"),
				afterOff, afterName,
				(afterOff-beforeOff)/60)
			t = zb
		}
	}

	fmt.Println("\n## A2 2026 年每个本地日期的实际小时数（只列非 24 小时的天）")
	fmt.Println("| 时区 | 非 24h 天数 | 23h | 23.5h | 24.5h | 25h | 举例（日期=小时数） |")
	fmt.Println("|---|---|---|---|---|---|---|")
	for _, name := range zones {
		loc, _ := time.LoadLocation(name)
		buckets := map[float64][]string{}
		non := 0
		for i := 0; i < 365; i++ {
			d := base.AddDate(0, 0, i)
			start := boundary(loc, i, d)
			end := boundary(loc, i+1, d.AddDate(0, 0, 1))
			h := end.Sub(start).Hours()
			if h != 24 {
				non++
				buckets[h] = append(buckets[h], d.Format("2006-01-02"))
			}
		}
		var keys []float64
		for k := range buckets {
			keys = append(keys, k)
		}
		sort.Float64s(keys)
		fmt.Printf("| %s | %d | %d | %d | %d | %d | ", name, non,
			len(buckets[23]), len(buckets[23.5]), len(buckets[24.5]), len(buckets[25]))
		first := true
		for _, k := range keys {
			for _, ds := range buckets[k] {
				if !first {
					fmt.Print("; ")
				}
				fmt.Printf("%s=%.1fh", ds, k)
				first = false
			}
		}
		fmt.Println(" |")
	}
}
