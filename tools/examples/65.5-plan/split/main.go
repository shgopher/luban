// go run split.go：同一波 10% 流量，按用户哈希切 vs 按 tenant_id 整租户切。
package main

import (
	"fmt"
	"sort"
)

const n, users = 200, 40 // 200 个租户 × 40 个用户

func h(s string) float64 { // 桶号 = FNV-1a(s) % 10000
	x := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		x = (x ^ uint32(s[i])) * 16777619
	}
	return float64(x%10000) / 10000
}

func main() {
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i
	}
	sort.Slice(ids, func(a, b int) bool { return h(fmt.Sprint("tenant:", ids[a])) < h(fmt.Sprint("tenant:", ids[b])) })
	tenantV2 := map[int]bool{}
	for _, id := range ids[:n/10] { // 整租户切：桶号前 10% 的租户走 v2
		tenantV2[id] = true
	}
	for _, mode := range []string{"user", "tenant"} {
		var v2u, mixed int
		for t := 0; t < n; t++ {
			v2n := 0
			for u := 0; u < users; u++ {
				on := tenantV2[t]
				if mode == "user" {
					on = h(fmt.Sprint("user:", t, ":", u)) < .1
				}
				if on {
					v2n++
					v2u++
				}
			}
			if v2n > 0 && v2n < users { // 租户内同时有新老两个版本
				mixed++
			}
		}
		fmt.Printf("%-6s：v2 用户占比 %.1f%%（%d/%d），租户内格式分裂 %d/%d\n",
			mode, float64(v2u)/(n*users)*100, v2u, n*users, mixed, n)
	}
}
