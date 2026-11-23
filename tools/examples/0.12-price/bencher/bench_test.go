package bencher

import (
	"testing"

	"github.com/shgopher/luban/tools/examples/0.12-price/factory"
	"github.com/shgopher/luban/tools/examples/0.12-price/plain"
	"github.com/shgopher/luban/tools/examples/0.12-price/strategy"
)

var kinds = []string{"none", "vip", "coupon", "season"}
var sink int

//go:noinline
func callPlain(k string, c int) int { return plain.Price(k, c) }

//go:noinline
func callStrategy(k string, c int) int { return strategy.Price(k, c) }

//go:noinline
func callFactory(k string, c int) int { return factory.Price(k, c) }

func BenchmarkPlain(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = callPlain(kinds[i&3], 20000)
	}
}

func BenchmarkStrategy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = callStrategy(kinds[i&3], 20000)
	}
}

func BenchmarkFactory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = callFactory(kinds[i&3], 20000)
	}
}
