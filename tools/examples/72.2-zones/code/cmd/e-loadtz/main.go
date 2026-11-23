// 实测 E：tzdata 是运行时依赖——内嵌 timetzdata 的二进制在裸容器里也能 LoadLocation
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

func main() {
	fmt.Printf("go=%s GOOS=%s TZDIR=%q\n", runtime.Version(), runtime.GOOS, os.Getenv("TZDIR"))
	failed := false
	for _, name := range []string{"America/New_York", "Australia/Lord_Howe", "Asia/Almaty"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			failed = true
			fmt.Printf("LoadLocation(%-20s) FAIL: %v\n", name, err)
			continue
		}
		// 2026-01-15：Almaty 应 +5（2024-03 起），Lord_Howe 应 +11（南半球夏令时）
		t := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).In(loc)
		n, off := t.Zone()
		fmt.Printf("LoadLocation(%-20s) OK  local=%s %s(%+.1f) now=%s\n",
			name, t.Format("2006-01-02 15:04"), n, float64(off)/3600,
			time.Now().In(loc).Format("2006-01-02 15:04"))
	}
	if failed {
		os.Exit(1)
	}
}
