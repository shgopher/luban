// 实测 E3 的机器可读版本：给 Node Intl.Segmenter 做逐样本对比。
package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

func main() {
	samples := []struct{ name, s string }{
		{"ascii-a", "a"},
		{"e-combining", "e\u0301"},
		{"flag-cn", "🇨🇳"},
		{"zwj-family", "👨‍👩‍👧‍👦"},
		{"skin-tone", "👍🏽"},
		{"zwj-profession", "👩‍💻"},
		{"keycap-1", "1️⃣"},
		{"heart-vs16", "❤️"},
	}
	fmt.Println("case\tbytes\trunes\tgraphemes")
	for _, s := range samples {
		fmt.Printf("%s\t%d\t%d\t%d\n", s.name, len(s.s), utf8.RuneCountInString(s.s), uniseg.GraphemeClusterCount(s.s))
	}
}
