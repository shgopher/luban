// 对比三种「求和」写法的开销：具体类型 / 泛型 / interface{} 断言。
// 数据规模 1000 个 int64，重复 1e6 次。
package main

import (
	"fmt"
	"testing"
)

var data = make([]int64, 1000)
var anyData = make([]any, 1000)

func init() {
	for i := range data {
		data[i] = int64(i)
		anyData[i] = int64(i)
	}
}

func sumConcrete(xs []int64) int64 {
	var s int64
	for _, v := range xs {
		s += v
	}
	return s
}

func sumGeneric[T int64 | int32 | float64](xs []T) T {
	var s T
	for _, v := range xs {
		s += v
	}
	return s
}

func sumAny(xs []any) int64 {
	var s int64
	for _, v := range xs {
		s += v.(int64) // 每次循环一次类型断言
	}
	return s
}

func main() {
	fmt.Println("具体类型 []int64   ", testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sumConcrete(data)
		}
	}))
	fmt.Println("泛型     []T       ", testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sumGeneric(data)
		}
	}))
	fmt.Println("interface{} []any  ", testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sumAny(anyData)
		}
	}))
}
