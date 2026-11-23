// 幂等：同一幂等键的第二次请求必须返回第一次的结果，而不是再执行一次。
package main

import "fmt"

type result struct {
	ok  bool
	msg string
}

func main() {
	store := map[string]result{}
	key := "PAY:order-1001"
	handle := func(key string) result {
		if r, ok := store[key]; ok {
			return result{true, "重复请求，返回首次结果: " + r.msg}
		}
		r := result{true, "扣款成功，流水号 T20261031001"}
		store[key] = r
		return r
	}
	fmt.Println("第 1 次:", handle(key).msg)
	fmt.Println("第 2 次:", handle(key).msg)
	fmt.Printf("实际扣款次数=%d\n", len(store))
}
