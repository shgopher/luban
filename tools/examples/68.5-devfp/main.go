// 设备指纹相似度：两设备属性集合的 Jaccard 相似度，高于阈值视为疑似同设备/群控。
package main

import "fmt"

func jaccard(a, b map[string]bool) float64 {
	inter, union := 0, len(a)
	for k := range b {
		if a[k] {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func main() {
	d1 := map[string]bool{"fingerprint": true, "cpu": true, "screen": true, "tz": true, "fonts": true}
	d2 := map[string]bool{"fingerprint": true, "cpu": true, "screen": true, "tz": true, "fonts": true, "ua": true}
	d3 := map[string]bool{"fingerprint": true, "cpu": true}
	fmt.Printf("d1 vs d2 相似度=%.2f\n", jaccard(d1, d2))
	fmt.Printf("d1 vs d3 相似度=%.2f\n", jaccard(d1, d3))
	fmt.Println("判据：>0.9 疑似同设备；0.6–0.9 需结合 IP/行为；<0.6 视为不同设备")
}
