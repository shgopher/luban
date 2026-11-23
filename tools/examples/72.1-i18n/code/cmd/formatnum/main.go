// 实测 C（Go 侧）：数字的 locale 分组符 / 小数点 / 数字系统，用 x/text/message 的 number.Decimal。
package main

import (
	"fmt"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var locales = []string{"en-US", "de-DE", "fr-FR", "en-IN", "ar-EG", "ja-JP"}
var values = []struct {
	label string
	v     float64
}{
	{"1234567.891", 1234567.891},
	{"0.5", 0.5},
	{"-1234.5", -1234.5},
}

func main() {
	fmt.Println("locale\tvalue\tformatted\tnondigit_codepoints\tfirst_rune")
	for _, l := range locales {
		tag := language.MustParse(l)
		p := message.NewPrinter(tag)
		for _, v := range values {
			// Scale(3)：固定 3 位小数，才能和 Node 的 min/maxFractionDigits=3 逐字节对比。
			s := p.Sprintf("%v", number.Decimal(v.v, number.Scale(3)))
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", l, v.label, s, codepoints(s), firstRune(s))
		}
	}
}

// codepoints 只列非数字字符，用来暴露分组符/小数点的真实码位：
// fr-FR 在 CLDR 新版本用 U+202F（窄不换行空格），老实现常用 U+00A0 或普通空格。
func codepoints(s string) string {
	out := ""
	for _, r := range s {
		if unicode.IsDigit(r) || r == '-' {
			continue
		}
		out += fmt.Sprintf("U+%04X ", r)
	}
	if out == "" {
		return "-"
	}
	return out[:len(out)-1]
}

func firstRune(s string) string {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return fmt.Sprintf("U+%04X", r)
		}
	}
	return "-"
}
