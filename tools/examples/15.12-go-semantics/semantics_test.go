package main

import (
	"testing"
	"unsafe"
)

func TestSliceAppendAliases(t *testing.T) {
	s1 := []int{1, 2, 3}
	s2 := s1[1:2]
	s3 := append(s2, 10)
	if got := s1[2]; got != 10 {
		t.Fatalf("append 应写进 s1 的第 3 个槽位，s1=%v", s1)
	}
	if len(s3) != 2 || s3[1] != 10 {
		t.Fatalf("s3=%v", s3)
	}
}

func TestCopyToNilSlice(t *testing.T) {
	src := []int{0, 1, 2}
	var dst []int
	if n := copy(dst, src); n != 0 {
		t.Fatalf("copy 到 nil slice 必须返回 0，得到 %d", n)
	}
	if len(dst) != 0 {
		t.Fatalf("dst 不应被扩容，得到 %v", dst)
	}
}

func TestRangeCopiesElement(t *testing.T) {
	accounts := []account{{100}, {200}, {300}}
	for _, a := range accounts {
		a.balance += 1000
	}
	if accounts[0].balance != 100 {
		t.Fatalf("range 变量是副本，原切片不应变化：%v", accounts)
	}
}

func TestLoopVarPerIteration(t *testing.T) {
	items := []customer{{"1", 0}, {"2", 0}, {"3", 0}}
	m := make(map[string]*customer)
	for _, c := range items {
		m[c.ID] = &c
	}
	if m["1"] == m["2"] || m["2"] == m["3"] {
		t.Fatal("go.mod >= 1.22：每轮循环变量地址必须不同")
	}
}

func TestDeferSemantics(t *testing.T) {
	if got := deferPlain(); got != 3 {
		t.Fatalf("匿名返回值：defer 改局部变量不影响返回，得到 %d", got)
	}
	if got := deferNamed(); got != 6 {
		t.Fatalf("命名返回值：defer 改命名返回值生效，得到 %d", got)
	}

	i := 0
	printed := -1
	func() {
		defer func(v int) { printed = v }(i) // 实参在 defer 语句处求值
		i++
	}()
	if printed != 0 || i != 1 {
		t.Fatalf("defer 实参应立即求值：printed=%d i=%d", printed, i)
	}
}

func TestTypedNilInterface(t *testing.T) {
	err := validateBug(10, "a")
	if err == nil {
		t.Fatal("(*MultiError)(nil) 装进 error 后不等于 nil")
	}
	if _, ok := err.(*MultiError); !ok {
		t.Fatalf("动态类型应为 *MultiError，得到 %T", err)
	}
	if validateFixed(10, "a") != nil {
		t.Fatal("显式返回 nil 才是真 nil")
	}
}

func TestStringIsBytes(t *testing.T) {
	s := "hêllo"
	if len(s) != 6 {
		t.Fatalf("len 是字节数，应为 6，得到 %d", len(s))
	}
	if s[1] != 0xc3 {
		t.Fatalf("s[1] 是 UTF-8 首字节 0xc3，得到 %#x", s[1])
	}
	n := 0
	for range s {
		n++
	}
	if n != 5 {
		t.Fatalf("runes 应为 5 个，得到 %d", n)
	}
}

func TestStructPadding(t *testing.T) {
	if got := unsafe.Sizeof(M{}); got != 16 {
		t.Fatalf("M 应为 16 字节，得到 %d", got)
	}
	if got := unsafe.Sizeof(N{}); got != 8 {
		t.Fatalf("N 应为 8 字节，得到 %d", got)
	}
	if got := unsafe.Sizeof(Bad{}); got != 24 {
		t.Fatalf("Bad 应为 24 字节，得到 %d", got)
	}
	if got := unsafe.Sizeof(Good{}); got != 16 {
		t.Fatalf("Good 应为 16 字节，得到 %d", got)
	}
}

func TestUnsafeStringSharesMemory(t *testing.T) {
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b))
	b[0] = 'H'
	if s != "Hello" {
		t.Fatalf("unsafe 转换应共享底层内存，得到 %q", s)
	}
}
