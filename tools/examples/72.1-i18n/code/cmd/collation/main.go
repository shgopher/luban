// 实测 D（Go 侧）：同一组名字在 en/de/sv 三种 collation 下的顺序。
// 关键：这不是「按 Unicode 码点排」，也不是「大小写不敏感」这么简单——
// collation 决定 a 与 å 的主级/次级权重，瑞典语把 å ä ö 排在 z 之后。
package main

import (
	"fmt"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

var words = []string{"Zebra", "apple", "Öl", "Åsa", "Ärger", "a", "z", "éclair", "Ödla", "B", "Arger", "Asa"}

func main() {
	for _, l := range []string{"en", "de", "sv"} {
		tag := language.MustParse(l)
		c := collate.New(tag) // CLDR root collation 在该语言下的定制
		sorted := append([]string(nil), words...)
		c.SortStrings(sorted)
		fmt.Printf("%s\t%v\n", l, sorted)
	}
	// 直接两两比较，回答「谁在前」这种排序测试用例
	fmt.Println("\n## 两两比较（负=前者在前，0=主级相同）")
	pairs := [][2]string{{"apple", "Åsa"}, {"Åsa", "Zebra"}, {"Ärger", "Zebra"}, {"Öl", "Zebra"}, {"apple", "Ärger"}}
	for _, l := range []string{"en", "de", "sv"} {
		c := collate.New(language.MustParse(l))
		for _, p := range pairs {
			fmt.Printf("%s\t%s vs %s\t%d\n", l, p[0], p[1], c.CompareString(p[0], p[1]))
		}
	}
}
