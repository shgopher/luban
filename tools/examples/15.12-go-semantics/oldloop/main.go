// 旧语义对照：同一个循环，go.mod 写 1.21 时循环变量只有一份。
package main

import "fmt"

type customer struct {
	ID      string
	Balance float64
}

func main() {
	test := []customer{{"1", 10}, {"2", -10}, {"3", 0}}
	m := map[string]*customer{}
	for _, c := range test {
		m[c.ID] = &c
	}
	same := m["1"] == m["2"] && m["2"] == m["3"]
	fmt.Printf("go.mod = go 1.21：3 个指针地址完全相同 = %v\n", same)
	fmt.Printf("m[1].ID=%s m[2].ID=%s m[3].ID=%s\n", m["1"].ID, m["2"].ID, m["3"].ID)
}
