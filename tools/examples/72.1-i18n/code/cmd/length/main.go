// 实测 E：文本膨胀、rune/字节/字素/显示宽度四种「长度」，以及 emoji 的计数差异。
package main

import (
	"fmt"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"golang.org/x/text/width"
)

// pseudo 做两件伪本地化最常见的事：加 [!! !!] 包裹（暴露截断）+ 重音替换（暴露编码/字体）。
func pseudo(s string) string {
	rep := map[rune]rune{'a': 'á', 'e': 'é', 'i': 'í', 'o': 'ó', 'u': 'ú', 'c': 'ç', 'n': 'ñ', 's': 'š', 'y': 'ý'}
	out := make([]rune, 0, len(s)+8)
	for _, r := range s {
		if x, ok := rep[r]; ok {
			out = append(out, x)
			continue
		}
		out = append(out, r)
	}
	return "[!! " + string(out) + " !!]"
}

// cells 用 East Asian Width 估算终端/固定宽容器里的显示格数：
// Wide/Fullwidth=2，组合符/零宽=0，其余=1（Ambiguous 按 1，浏览器里通常也是 1）。
func cells(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		case width.EastAsianNarrow, width.EastAsianHalfwidth, width.Neutral:
			n++
		default:
			// Combining / 变体选择符等不占格
		}
	}
	return n
}

func counts(s string) (bytes, runes, graphemes, cells int, uniWidth int) {
	return len(s), utf8.RuneCountInString(s), uniseg.GraphemeClusterCount(s), 0, 0
}

func main() {
	base := "Saved 3 files to your library. Open the folder to review them."
	p := pseudo(base)
	br, pr := utf8.RuneCountInString(base), utf8.RuneCountInString(p)
	fmt.Println("## E1 pseudo-localization 膨胀")
	fmt.Printf("base   runes=%d bytes=%d  %q\n", br, len(base), base)
	fmt.Printf("pseudo runes=%d bytes=%d  %q\n", pr, len(p), p)
	fmt.Printf("rune 膨胀=+%.2f%%  byte 膨胀=+%.2f%%\n", float64(pr-br)*100/float64(br), float64(len(p)-len(base))*100/float64(len(base)))

	fmt.Println("\n## E2 同一文案 en/zh/ja：字节/rune/字素/显示格")
	copies := []struct{ lang, text string }{
		{"en", "Save changes to your library"},
		{"zh", "将更改保存到你的资料库"},
		{"ja", "変更をライブラリに保存"},
		{"de", "Änderungen in Ihrer Bibliothek speichern"}, // 对照：同一文案的德语长度（作者翻译，仅作量级参考）
	}
	fmt.Println("lang\tbytes\trunes\tgraphemes\tcells(EastAsianWidth)\tcells(uniseg)")
	for _, c := range copies {
		g := uniseg.GraphemeClusterCount(c.text)
		fmt.Printf("%s\t%d\t%d\t%d\t%d\t%d\t%s\n", c.lang, len(c.text), utf8.RuneCountInString(c.text), g, cells(c.text), uniseg.StringWidth(c.text), c.text)
	}

	fmt.Println("\n## E3 emoji / 组合字符的三列计数")
	samples := []struct{ name, s string }{
		{"ASCII 'a'", "a"},
		{"e + 组合重音 U+0301", "e\u0301"},
		{"旗帜 CN (2 个 regional indicator)", "🇨🇳"},
		{"ZWJ 家庭 👨‍👩‍👧‍👦", "👨‍👩‍👧‍👦"},
		{"肤色修饰 👍🏽", "👍🏽"},
		{"ZWJ 职业 👩‍💻", "👩‍💻"},
		{"键帽 1️⃣", "1️⃣"},
		{"心 + VS16 ❤️", "❤️"},
	}
	fmt.Println("case\tbytes\trunes\tgraphemes\tcells(uniseg)")
	for _, s := range samples {
		fmt.Printf("%s\t%d\t%d\t%d\t%d\n", s.name, len(s.s), utf8.RuneCountInString(s.s), uniseg.GraphemeClusterCount(s.s), uniseg.StringWidth(s.s))
	}

	fmt.Println("\n## E4 泰语无空格：按空白切 vs 按字素")
	thai := "ภาษาไทยไม่มีช่องว่างระหว่างคำ"
	fmt.Printf("thai=%q bytes=%d runes=%d graphemes=%d whitespace_chunks=%d\n",
		thai, len(thai), utf8.RuneCountInString(thai), uniseg.GraphemeClusterCount(thai), len(splitWS(thai)))
}

func splitWS(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
