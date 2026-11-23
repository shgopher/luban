// 实测 B（Go 侧）：CLDR cardinal 复数类别。x/text/feature/plural 直接查 CLDR 规则表。
package main

import (
	"fmt"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// n 用 CLDR 的 5 个操作数表达：i 整数位、v 可见小数位、w 去尾零小数位、f/t 小数位值。
// 为什么不用 float：CLDR 规则按「可见小数位」区分（1 与 1.0 在部分语言里不同类）。
type operand struct {
	label string
	i, v, w, f, t int
}

var numbers = []operand{
	{"0", 0, 0, 0, 0, 0},
	{"1", 1, 0, 0, 0, 0},
	{"2", 2, 0, 0, 0, 0},
	{"3", 3, 0, 0, 0, 0},
	{"5", 5, 0, 0, 0, 0},
	{"11", 11, 0, 0, 0, 0},
	{"21", 21, 0, 0, 0, 0},
	{"100", 100, 0, 0, 0, 0},
	{"1.5", 1, 1, 1, 5, 5},
}

var langs = []string{"en", "ru", "ar", "pl", "zh", "fr", "de", "ja", "uk", "cs"}

var formName = map[plural.Form]string{
	plural.Other: "other", plural.Zero: "zero", plural.One: "one",
	plural.Two: "two", plural.Few: "few", plural.Many: "many",
}

func main() {
	fmt.Println("lang\tn\ti\tv\tw\tf\tt\tcategory")
	for _, l := range langs {
		tag := language.MustParse(l)
		for _, n := range numbers {
			// Cardinal = 基数词规则；Ordinal（第 1/第 2）是另一张表，不能混用。
			form := plural.Cardinal.MatchPlural(tag, n.i, n.v, n.w, n.f, n.t)
			fmt.Printf("%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n", l, n.label, n.i, n.v, n.w, n.f, n.t, formName[form])
		}
	}
}
