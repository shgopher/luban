// 对比 Go 里两种「错误传播」的代价：返回 error vs panic+recover。
// 只测「传播一次」的开销，不含业务逻辑。
package main

import (
	"errors"
	"fmt"
	"testing"
)

var errBoom = errors.New("boom")

//go:noinline
func byError() (int, error) { return 0, errBoom }

//go:noinline
func byPanic() (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered: %v", r)
		}
	}()
	panic("boom")
}

//go:noinline
func byPanicPlain() (n int, err error) {
	defer func() {
		if recover() != nil {
			err = errBoom
		}
	}()
	panic("boom")
}

func BenchmarkPanicPlain(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := byPanicPlain(); err != nil {
		}
	}
}

func BenchmarkError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := byError(); err != nil {
		}
	}
}

func BenchmarkPanicRecover(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := byPanic(); err != nil {
		}
	}
}

func main() {
	fmt.Println("BenchmarkError      ", testing.Benchmark(BenchmarkError))
	fmt.Println("BenchmarkPanicRecover", testing.Benchmark(BenchmarkPanicRecover))
	fmt.Println("BenchmarkPanicPlain  ", testing.Benchmark(BenchmarkPanicPlain))
}
