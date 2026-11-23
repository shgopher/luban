// 实测 G：定时任务与序列化的时区口径
// 1) "每 24 小时加一次" vs "按本地墙钟每天重算" 在跨 DST 后差多少
// 2) 每天 02:30（gap 时刻）/ 01:30（overlap 时刻）一年到底触发几次
// 3) time.Time 的 JSON 序列化丢掉了什么
package main

import (
	"encoding/json"
	"fmt"
	"time"
)

func main() {
	ny, _ := time.LoadLocation("America/New_York")

	fmt.Println("## G1 每天 09:00 America/New_York：\"每次 +24h\" vs \"按本地墙钟重算\"")
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, ny)
	// 方式一：固定 24h 步长（很多手写调度器就这么干）
	driftDays := 0
	by24 := start
	var mid24 time.Time
	for i := 0; i < 364; i++ {
		by24 = by24.Add(24 * time.Hour)
		if i == 180 { // 2026-06-30/07-01 附近
			mid24 = by24
		}
		if by24.In(ny).Hour() != 9 || by24.In(ny).Minute() != 0 {
			driftDays++
		}
	}
	// 方式二：按本地墙钟重算（正确做法）
	var mids []time.Time
	for i := 0; i < 365; i++ {
		d := time.Date(2026, 1, 1, 9, 0, 0, 0, ny).AddDate(0, 0, i)
		mids = append(mids, d)
	}
	midWall := mids[180]
	fmt.Println("| 方式 | 2026-07-01 前后那次触发 | 本地钟面 | 一年中钟面≠09:00 的天数 | 第 365 次 |")
	fmt.Println("|---|---|---|---|---|")
	fmt.Printf("| 每次 +24h | %s | %s | %d | %s |\n",
		mid24.Format("2006-01-02 15:04 MST"), mid24.In(ny).Format("15:04"), driftDays,
		by24.Format("2006-01-02 15:04 MST"))
	fmt.Printf("| 按本地墙钟重算 | %s | %s | 0 | %s |\n",
		midWall.Format("2006-01-02 15:04 MST"), midWall.In(ny).Format("15:04"),
		mids[364].Format("2006-01-02 15:04 MST"))

	fmt.Println("\n## G2 每天 02:30 America/New_York（2026-03-08 该墙钟不存在），两种策略一年触发次数")
	fmt.Println("| 策略 | 2026 年触发次数 | 缺的那天 | 缺的那天实际触发 |")
	fmt.Println("|---|---|---|---|")
	skip, shift := 0, 0
	var missing string
	for i := 0; i < 365; i++ {
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, ny).AddDate(0, 0, i)
		f := time.Date(day.Year(), day.Month(), day.Day(), 2, 30, 0, 0, ny)
		if f.In(ny).Hour() == 2 && f.In(ny).Minute() == 30 {
			skip++
			shift++
		} else {
			missing = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			shift++ // 顺延策略：规范化到 03:30 仍然触发
		}
	}
	fmt.Printf("| 跳过（发现墙钟被改写就丢弃） | %d | %s | 不触发 |\n", skip, missing)
	fmt.Printf("| 顺延（Go 规范化后照常触发） | %d | %s | 03:30 EDT |\n", shift, missing)

	fmt.Println("\n## G3 每天 01:30 America/New_York（2026-11-01 该墙钟出现两次）")
	wallCount, instCount := 0, 0
	seen := map[int64]bool{}
	for i := 0; i < 365; i++ {
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, ny).AddDate(0, 0, i)
		f := time.Date(day.Year(), day.Month(), day.Day(), 1, 30, 0, 0, ny)
		if f.In(ny).Hour() == 1 && f.In(ny).Minute() == 30 {
			wallCount++
			seen[f.Unix()] = true
			// 再查这个墙钟在 UTC 侧是否还有第二个瞬时
			for k := -3; k <= 3; k++ {
				t := f.Add(time.Duration(k) * time.Hour)
				if t.In(ny).Hour() == 1 && t.In(ny).Minute() == 30 {
					seen[t.Unix()] = true
				}
			}
		}
	}
	instCount = len(seen)
	fmt.Println("| 口径 | 次数 | 含义 |")
	fmt.Println("|---|---|---|")
	fmt.Printf("| 按本地日期各触发一次 | %d | 墙钟只出现一次的普通日 + 重复日的第一次 |\n", wallCount)
	fmt.Printf("| 本地 01:30 实际出现的瞬时数 | %d | 重复日有两个 01:30：EDT(-4) 与 EST(-5) |\n", instCount)

	fmt.Println("\n## G4 time.Time 的 JSON：RFC3339 只带偏移，不带 zone id")
	fmt.Println("| 输入（offset-only） | 解析后加 6 个月 | 差异 |")
	fmt.Println("|---|---|---|")
	fixed, _ := time.Parse(time.RFC3339, "2026-07-15T09:00:00-04:00")
	zoned := fixed.In(must("America/New_York"))
	fmt.Printf("| 2026-07-15T09:00:00-04:00 | 固定偏移加 6 个月: %s | — |\n", fixed.AddDate(0, 6, 0).Format("2006-01-02 15:04 -07:00"))
	fmt.Printf("| 同上，先 In(zone) 再加 6 个月 | zone 感知: %s | %s |\n", zoned.AddDate(0, 6, 0).Format("2006-01-02 15:04 MST"),
		zoned.AddDate(0, 6, 0).Sub(fixed.AddDate(0, 6, 0)))
	type row struct {
		Name string    `json:"name"`
		At   time.Time `json:"at"`
	}
	for _, tc := range []struct {
		name string
		t    time.Time
	}{
		{"上海会议", time.Date(2026, 3, 8, 9, 0, 0, 0, must("Asia/Shanghai"))},
		{"纽约 DST 起点后", time.Date(2026, 3, 8, 3, 30, 0, 0, must("America/New_York"))},
		{"纽约 DST 起点前", time.Date(2026, 3, 8, 1, 30, 0, 0, must("America/New_York"))},
	} {
		b, _ := json.Marshal(row{Name: tc.name, At: tc.t})
		var back row
		json.Unmarshal(b, &back)
		zn, off := back.At.Zone()
		fmt.Printf("| %s | %s | 回读 zone=%q offset=%+d |\n", tc.name, string(b), zn, off)
	}
}

func must(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
