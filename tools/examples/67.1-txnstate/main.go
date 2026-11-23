// 交易状态机校验：只允许白名单里的迁移，非法迁移必须被拦住。
package main

import "fmt"

var allowed = map[string][]string{
	"INIT":      {"PAYING", "CLOSED"},
	"PAYING":    {"PAID", "PAY_FAILED"},
	"PAID":      {"REFUNDING", "SETTLED"},
	"REFUNDING": {"REFUNDED", "REFUND_FAILED"},
}

func can(from, to string) bool {
	for _, t := range allowed[from] {
		if t == to {
			return true
		}
	}
	return false
}

func main() {
	cases := [][2]string{
		{"INIT", "PAYING"}, {"PAYING", "PAID"}, {"PAID", "REFUNDING"},
		{"PAID", "PAYING"}, {"CLOSED", "PAID"}, {"REFUNDED", "PAID"},
	}
	for _, c := range cases {
		if can(c[0], c[1]) {
			fmt.Printf("✓ %s -> %s\n", c[0], c[1])
		} else {
			fmt.Printf("✗ 非法迁移 %s -> %s\n", c[0], c[1])
		}
	}
}
