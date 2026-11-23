package main

import (
	"fmt"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

func main() {
	for _, s := range []string{"en-US", "de-DE", "fr-FR", "en-IN", "ar-EG", "ja-JP"} {
		tag := language.MustParse(s)
		p := message.NewPrinter(tag)
		fmt.Printf("%s | plain=%%v:%v | dec=%%v:%v | dec-verb=%v | f2=%.2f\n",
			s, 1234567.891, number.Decimal(1234567.891), number.Decimal(1234567.891), 1234567.891)
		_ = p
	}
	// 用 printer 看看 number.Decimal 是否用 locale 分组
	for _, s := range []string{"en-US", "de-DE", "fr-FR", "en-IN", "ar-EG", "ja-JP"} {
		tag := language.MustParse(s)
		p := message.NewPrinter(tag)
		fmt.Printf("P %s => %v\n", s, p.Sprintf("%v", number.Decimal(1234567.891)))
	}
}
