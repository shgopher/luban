// 实测 E：tzdata 是"会变的数据"——同一段代码，换 tzdata 版本就换出不同历史/未来
package main

import (
	"fmt"
	"os"
	"time"
)

var zones = []string{
	"Europe/Istanbul",     // 2016-09 起永久 UTC+3
	"America/Mexico_City", // 2022-10 起取消 DST
	"Asia/Almaty",         // 2024-03 起全国 UTC+5
	"Australia/Lord_Howe", // 30 分钟 DST
	"America/Santiago",    // 南半球 DST
}

func main() {
	fmt.Printf("TZDIR=%q（空=用平台默认 zoneinfo）\n", os.Getenv("TZDIR"))
	fmt.Println("| 时区 | UTC 切换时刻 | 切换前本地/偏移 | 切换后本地/偏移 |")
	fmt.Println("|---|---|---|---|")
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			fmt.Printf("| %s | LOAD FAIL: %v | | |\n", name, err)
			continue
		}
		t := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for {
			_, zb := t.In(loc).ZoneBounds()
			if zb.IsZero() || !zb.Before(end) {
				break
			}
			bn, bo := zb.Add(-time.Second).In(loc).Zone()
			an, ao := zb.In(loc).Zone()
			fmt.Printf("| %s | %s | %s %s(%+.1f) | %s %s(%+.1f) |\n",
				name, zb.UTC().Format("2006-01-02T15:04Z"),
				zb.Add(-time.Second).In(loc).Format("2006-01-02 15:04"), bn, float64(bo)/3600,
				zb.In(loc).Format("2006-01-02 15:04"), an, float64(ao)/3600)
			t = zb
		}
	}
}
