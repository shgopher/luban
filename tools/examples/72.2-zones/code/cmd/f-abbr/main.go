// 实测 F：时区缩写不是唯一标识——同一缩写对应不同偏移
package main

import (
	"fmt"
	"time"
)

var zones = []string{
	"Asia/Shanghai", "America/Chicago", "America/Havana",   // CST
	"Asia/Kolkata", "Europe/Dublin", "Asia/Jerusalem",      // IST
	"Europe/London", "Asia/Dhaka",                          // BST
	"America/Halifax", "Asia/Riyadh",                       // AST
	"America/Sao_Paulo", "Europe/Lisbon",                   // 同一地区不同缩写
	"America/New_York", "Australia/Sydney",                 // EST/AEST 易混
}

func main() {
	instants := []time.Time{
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	}
	fmt.Println("## F1 同一时刻下各时区的缩写与偏移")
	fmt.Println("| 时区 | 2026-01-15T12:00Z 缩写/偏移 | 2026-07-15T12:00Z 缩写/偏移 |")
	fmt.Println("|---|---|---|")
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			fmt.Printf("| %s | LOAD ERROR %v | |\n", name, err)
			continue
		}
		fmt.Printf("| %s | %s | %s |\n", name, fmtZone(instants[0].In(loc)), fmtZone(instants[1].In(loc)))
	}

	fmt.Println("\n## F2 按缩写反查：同一缩写对应几个时区、几个偏移")
	fmt.Println("| 缩写 | 时区数 | 偏移种类（小时） | 时区清单 |")
	fmt.Println("|---|---|---|---|")
	abbrZones := map[string][]string{}
	abbrOffsets := map[string]map[int]bool{}
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		for _, inst := range instants {
			n, off := inst.In(loc).Zone()
			abbrZones[n] = append(abbrZones[n], name)
			if abbrOffsets[n] == nil {
				abbrOffsets[n] = map[int]bool{}
			}
			abbrOffsets[n][off] = true
		}
	}
	// 观察到的缩写里，挑出「偏移不止一种」的
	for _, ab := range []string{"CST", "IST", "BST", "AST", "EST", "GMT"} {
		zs := abbrZones[ab]
		if len(zs) == 0 {
			continue
		}
		var offs []string
		for o := range abbrOffsets[ab] {
			offs = append(offs, fmt.Sprintf("%+.1f", float64(o)/3600))
		}
		fmt.Printf("| %s | %d | %v | %v |\n", ab, len(uniq(zs)), offs, uniq(zs))
	}
}

func fmtZone(t time.Time) string {
	n, off := t.Zone()
	return fmt.Sprintf("%s / %+.1f", n, float64(off)/3600)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
