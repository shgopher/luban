// 实测 E：同一批瞬时，三份 tzdata 版本（macOS 2026f / Go 内嵌 2026c / alpine 2026b）
// 各自算出什么偏移。只比"某一瞬时落在哪个偏移"，不枚举切换点，结果稳定可比。
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
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.Contains(rel, "/") && !strings.HasPrefix(rel, "posix/") && !strings.HasPrefix(rel, "right/") {
			names = append(names, rel)
		}
		return nil
	})
	sort.Strings(names)
	return names
}

func main() {
	v := "unknown"
	if b, err := os.ReadFile("/usr/share/zoneinfo/+VERSION"); err == nil {
		v = strings.TrimSpace(string(b))
	}
	fmt.Printf("# tzdata_version=%s\n", v)
	instants := []time.Time{
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2030, 7, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2040, 7, 15, 12, 0, 0, 0, time.UTC),
	}
	n := 0
	for _, name := range zoneNames() {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		n++
		fmt.Printf("%s", name)
		for _, inst := range instants {
			_, off := inst.In(loc).Zone()
			fmt.Printf("|%+d", off)
		}
		fmt.Println()
	}
	fmt.Fprintf(os.Stderr, "zones=%d\n", n)
}
