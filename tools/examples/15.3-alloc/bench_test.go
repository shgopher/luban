package alloc

import (
	"fmt"
	"strconv"
	"testing"
)

var sink any

type Point struct {
	X, Y int64
}

func pointValue() Point    { return Point{1, 2} }
func pointPointer() *Point { return &Point{1, 2} }
func BenchmarkPointValue(b *testing.B) {
	b.ReportAllocs()
	var p Point
	for i := 0; i < b.N; i++ {
		p = pointValue()
	}
	sink = p
}

func BenchmarkPointPointer(b *testing.B) {
	b.ReportAllocs()
	var p *Point
	for i := 0; i < b.N; i++ {
		p = pointPointer()
	}
	sink = p
}

func BenchmarkSlicePrealloc(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := make([]int, 0, 1024)
		for j := 0; j < 1024; j++ {
			s = append(s, j)
		}
		sink = s
	}
}

func BenchmarkSliceGrow(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var s []int
		for j := 0; j < 1024; j++ {
			s = append(s, j)
		}
		sink = s
	}
}

func BenchmarkSprintf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink = fmt.Sprintf("%d:%d", i, i*2)
	}
}

func BenchmarkAppendInt(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 0, 32)
	for i := 0; i < b.N; i++ {
		buf = buf[:0]
		buf = strconv.AppendInt(buf, int64(i), 10)
		buf = append(buf, ':')
		buf = strconv.AppendInt(buf, int64(i*2), 10)
		sink = string(buf)
	}
}
