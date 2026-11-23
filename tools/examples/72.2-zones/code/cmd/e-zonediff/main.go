// 实测 E：同一台机器上三份 tzdata（macOS 2026f / Go 内嵌 2026c / alpine 2026b），
// 枚举 2025–2035 全部切换点，diff 出「换 tzdata 版本会改哪些时区」。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func zoneNames() []string {
	root := "/usr/share/zoneinfo"
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	var names []string
	// 从系统目录收集所有 "Area/City" 形式的名字
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !strings.Contains(rel, "/") {
			return nil
		}
		if strings.HasPrefix(rel, "posix/") || strings.HasPrefix(rel, "right/") {
			return nil
		}
		names = append(names, rel)
		return nil
	})
	sort.Strings(names)
	return names
}

func main() {
	src := os.Getenv("ZONEINFO")
	if src == "" {
		src = "平台默认 /usr/share/zoneinfo"
	}
	fmt.Printf("# source=%s\n", src)
	names := zoneNames()
	total, ok := 0, 0
	for _, name := range names {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		ok++
		t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC)
		for {
			_, zb := t.In(loc).ZoneBounds()
			if zb.IsZero() || !zb.Before(end) || !zb.After(t) {
				break // 防御：同一瞬时出现多个切换时 ZoneBounds 可能不前移
			}
			_, off := zb.In(loc).Zone()
			fmt.Printf("%s|%s|%+d\n", name, zb.UTC().Format("2006-01-02T15:04:05Z"), off)
			total++
			t = zb
		}
	}
	fmt.Fprintf(os.Stderr, "zones_loaded=%d transitions=%d\n", ok, total)
}
