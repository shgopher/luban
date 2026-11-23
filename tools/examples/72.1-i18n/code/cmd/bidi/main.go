// 实测 F：Unicode bidi 类别与视觉顺序（逻辑序 -> 视觉序）。
// 只做算法层验证，未做真机渲染 / 字体 shaping 验证。
package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/unicode/bidi"
)

var className = map[bidi.Class]string{
	bidi.L: "L", bidi.R: "R", bidi.EN: "EN", bidi.ES: "ES", bidi.ET: "ET",
	bidi.AN: "AN", bidi.CS: "CS", bidi.B: "B", bidi.S: "S", bidi.WS: "WS",
	bidi.ON: "ON", bidi.BN: "BN", bidi.NSM: "NSM", bidi.AL: "AL",
}

func class(r rune) string {
	p, _ := bidi.LookupRune(r)
	if s, ok := className[p.Class()]; ok {
		return s
	}
	return "ctrl/other"
}

func main() {
	cases := []struct{ name, s string }{
		{"阿拉伯文 + 欧洲数字 + 希伯来文", "مرحبا 123 שלום"},
		{"希伯来文 + 括号 + 拉丁字母", "שלום (abc) עולם"},
		{"RTL 段落里的负数与百分号", "الرصيد -1,234.50 USD"},
		{"纯 LTR 对照", "hello 123 world"},
	}
	for _, c := range cases {
		var p bidi.Paragraph
		// SetString 返回已处理的字节数（遇到段落分隔 B 就截断）
		n, err := p.SetString(c.s)
		if err != nil {
			panic(err)
		}
		o, err := p.Order()
		if err != nil {
			panic(err)
		}
		fmt.Printf("## %s\nlogical: %q (处理 %d/%d 字节)\n", c.name, c.s, n, len(c.s))
		// 逐字符列出 bidi 类别：这是「为什么数字在 RTL 里不反」的证据
		var parts []string
		for _, r := range c.s {
			parts = append(parts, fmt.Sprintf("%q=%s", r, class(r)))
		}
		fmt.Println("classes: " + strings.Join(parts, " "))
		fmt.Printf("paragraph_direction=%d runs=%d\n", o.Direction(), o.NumRuns())
		// Ordering.Run 按「逻辑序」给 run；RTL 段落要逆序摆放 run 才是视觉序。
		// 这里只处理 0/1 两级（纯 RTL 段落足够），多级嵌套未展开。
		// Ordering.Run 按逻辑序给 run。RTL 段落里，run 在屏幕上是「从右往左」摆放的，
		// 所以真正要交给渲染器的信息是 run 的方向 + 摆放顺序，而不是一个拼接后的字符串。
		// 这里不拼接字符串：阿拉伯文的字形连写/括号镜像必须由渲染器做，文本 dump 无法代表。
		var layout []int
		for i := 0; i < o.NumRuns(); i++ {
			layout = append(layout, i)
		}
		if o.Direction() == bidi.RightToLeft {
			for i, j := 0, len(layout)-1; i < j; i, j = i+1, j-1 {
				layout[i], layout[j] = layout[j], layout[i]
			}
		}
		fmt.Printf("run 摆放顺序(屏幕左->右)=%v\n", layout)
		for i := 0; i < o.NumRuns(); i++ {
			run := o.Run(i)
			start, end := run.Pos()
			fmt.Printf("run[%d] dir=%d pos=%d..%d text=%q\n", i, run.Direction(), start, end, run.String())
		}
		fmt.Println()
	}
}
