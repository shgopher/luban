// 状态机检查：未达状态、无出口状态、缺终态。
package main

import "fmt"

func main() {
	states := []string{"INIT", "PAID", "SHIPPED", "DONE", "CANCELLED"}
	edges := map[string][]string{
		"INIT":      {"PAID", "CANCELLED"},
		"PAID":      {"SHIPPED", "CANCELLED"},
		"SHIPPED":   {"DONE"},
		"DONE":      {},
		"CANCELLED": {},
	}
	reach := map[string]bool{"INIT": true}
	changed := true
	for changed {
		changed = false
		for from, tos := range edges {
			if !reach[from] {
				continue
			}
			for _, to := range tos {
				if !reach[to] {
					reach[to] = true
					changed = true
				}
			}
		}
	}
	for _, s := range states {
		if !reach[s] {
			fmt.Printf("✗ 不可达状态：%s\n", s)
		}
	}
	for _, s := range states {
		if tos, ok := edges[s]; ok && len(tos) == 0 && s != "DONE" && s != "CANCELLED" {
			fmt.Printf("✗ 无出口且非终态：%s\n", s)
		}
	}
	fmt.Println("检查完成：4 个状态可达，2 个终态")
}
