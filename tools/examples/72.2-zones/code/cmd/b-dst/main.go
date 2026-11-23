// 实测 B：gap（春季不存在的 02:30）与 overlap（秋季重复的 01:30）
// 重点不是"会不会出错"，而是"不同语言/不同时区的默认选择各不相同"。
package main

import (
	"fmt"
	"time"
)

func dumpRow(tag string, t time.Time) {
	name, off := t.Zone()
	fmt.Printf("| %s | %s | %s | %+d s | %s | %d |\n",
		tag, t.Format("2006-01-02 15:04:05 -07:00"),
		t.UTC().Format("2006-01-02T15:04:05Z"), off, name, t.Unix())
}

func gapTest(zone string, y int, mo time.Month, d, hh, mm int) {
	loc, _ := time.LoadLocation(zone)
	wall := time.Date(y, mo, d, hh, mm, 0, 0, loc)
	fmt.Printf("\n### %s 春季 gap：%04d-%02d-%02d %02d:%02d 不存在\n", zone, y, mo, d, hh, mm)
	fmt.Println("| 构造方式 | 本地显示 | UTC | 偏移 | 缩写 | Unix |")
	fmt.Println("|---|---|---|---|---|---|")
	dumpRow("time.Date(墙钟)", wall)
	p, _ := time.ParseInLocation("2006-01-02 15:04:05", fmt.Sprintf("%04d-%02d-%02d %02d:%02d:00", y, mo, d, hh, mm), loc)
	dumpRow("ParseInLocation(墙钟)", p)
	fmt.Printf("| 回读 hour | %d（期望 %d） | | | | |\n", wall.In(loc).Hour(), hh)
	shift := wall.In(loc).Hour() - hh
	fmt.Printf("| 规范化方向 | 墙钟 %02d:%02d → %02d:%02d，移动 %+d 小时（负=往前，正=往后） | | | | |\n",
		hh, mm, wall.In(loc).Hour(), wall.In(loc).Minute(), shift)
}

func overlapTest(zone string, y int, mo time.Month, d, hh, mm int) {
	loc, _ := time.LoadLocation(zone)
	wall := time.Date(y, mo, d, hh, mm, 0, 0, loc)
	// 用同一个墙上小时的两次出现反查：从 UTC 侧枚举前后 3 小时
	var occurrences []time.Time
	base := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
	for k := -48; k <= 96; k++ {
		t := base.Add(time.Duration(k) * 30 * time.Minute).In(loc)
		if t.Hour() == hh && t.Minute() == mm && t.Day() == d {
			occurrences = append(occurrences, t)
		}
	}
	fmt.Printf("\n### %s 秋季 overlap：%04d-%02d-%02d %02d:%02d 出现两次\n", zone, y, mo, d, hh, mm)
	fmt.Println("| 构造方式 | 本地显示 | UTC | 偏移 | 缩写 | Unix |")
	fmt.Println("|---|---|---|---|---|---|")
	dumpRow("time.Date(墙钟)", wall)
	if len(occurrences) == 2 {
		dumpRow("第一次出现(UTC 反查)", occurrences[0])
		dumpRow("第二次出现(UTC 反查)", occurrences[1])
		fmt.Printf("| Go 选了哪次 | %s | | | | |\n",
			map[bool]string{true: "第一次", false: "第二次"}[wall.Unix() == occurrences[0].Unix()])
		fmt.Printf("| 两次的 Unix 差 | %d s | | | | |\n", occurrences[1].Unix()-occurrences[0].Unix())
	}
}

func main() {
	gapTest("America/New_York", 2026, 3, 8, 2, 30)
	gapTest("Europe/Berlin", 2026, 3, 29, 2, 30)

	overlapTest("America/New_York", 2026, 11, 1, 1, 30)
	overlapTest("Europe/Berlin", 2026, 10, 25, 2, 30)
	overlapTest("Australia/Lord_Howe", 2026, 4, 5, 1, 30)

	fmt.Println("\n## B3 offset 字符串 ≠ zone：同一个墙上时间，两条解析路径给出两个瞬时")
	fmt.Println("| 输入 | 解析方式 | 得到的本地时刻 | UTC |")
	fmt.Println("|---|---|---|---|")
	loc, _ := time.LoadLocation("America/New_York")
	f, err := time.Parse(time.RFC3339, "2026-03-08T02:30:00-05:00")
	fmt.Printf("| 2026-03-08T02:30:00-05:00 | time.Parse(RFC3339) | %s | %s |\n",
		f.In(loc).Format("2006-01-02 15:04:05 -07:00"), f.UTC().Format("2006-01-02T15:04:05Z"))
	p2, _ := time.ParseInLocation("2006-01-02T15:04:05", "2026-03-08T02:30:00", loc)
	fmt.Printf("| 2026-03-08 02:30（无偏移） | ParseInLocation(NY) | %s | %s |\n",
		p2.Format("2006-01-02 15:04:05 -07:00"), p2.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Printf("| 差值 | %v | | |\n", p2.Sub(f))
	_ = err

	fmt.Println("\n## B4 换一种写法的同一个 gap（Santiago 在 00:00 切，gap 落在 23:00–01:00）")
	loc2, _ := time.LoadLocation("America/Santiago")
	s := time.Date(2026, 9, 6, 0, 30, 0, 0, loc2)
	dumpRow("Santiago 2026-09-06 00:30", s)
	fmt.Printf("| 回读 | %s | | | | |\n", s.In(loc2).Format("2006-01-02 15:04:05 -07:00"))
}
