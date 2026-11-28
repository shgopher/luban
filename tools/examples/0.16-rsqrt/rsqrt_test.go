package rsqrt

import (
	"fmt"
	"math"
	"testing"
)

// TestSplit 复现原文的两个算例：3.14 与 0.015。
// 重点不是"算出来是几"，而是"存回去已经不是 3.14 了"。
func TestSplit(t *testing.T) {
	cases := []struct {
		dec    float64
		want   Parts
		wantIn string
	}{
		{3.14, Parts{Sign: 0, Exponent: 128, Mantissa: 4781507}, "0x4048F5C3"},
		{0.015, Parts{Sign: 0, Exponent: 120, Mantissa: 7717519}, "0x3C75C28F"},
	}
	for _, c := range cases {
		f := float32(c.dec)
		got := Split(f)
		if got != c.want {
			t.Fatalf("%v: Split=%+v want %+v", c.dec, got, c.want)
		}
		if h := fmt.Sprintf("0x%08X", math.Float32bits(f)); h != c.wantIn {
			t.Fatalf("%v: bits=%s want %s", c.dec, h, c.wantIn)
		}
		if Compose(got) != f {
			t.Fatalf("%v: Compose 往返失败", c.dec)
		}
		err := RoundTripError(c.dec, Compose(got))
		t.Logf("%-6v E=%-3d M=%-8d 回算=%.20g 相对误差=%.3e", c.dec, got.Exponent, got.Mantissa, float64(Compose(got)), err)
		if err == 0 {
			t.Fatalf("%v 居然精确表示，和原文结论不符", c.dec)
		}
	}
}

// TestZeroOnePlusZeroTwo 是"为什么 0.1+0.2 != 0.3"的最小证据：
// 三个十进制字面量在二进制里都存不准，误差不会自动抵消。
func TestZeroOnePlusZeroTwo(t *testing.T) {
	// 必须用变量：Go 的无类型常量是任意精度，编译期会把 0.1+0.2 算成精确值，
	// 只有落到 float64 变量上才会露出二进制误差。
	a, b := 0.1, 0.2
	sum := a + b
	if sum == 0.3 {
		t.Fatal("0.1+0.2 竟然等于 0.3")
	}
	t.Logf("0.1+0.2 = %.20g, 0.3 = %.20g, 差 = %.3e", sum, 0.3, sum-0.3)
}

// TestAccuracy 统计相对误差：一次迭代已经够图形学用，两次迭代接近精确值。
func TestAccuracy(t *testing.T) {
	const n = 200000
	magics := []struct {
		name string
		v    uint32
	}{
		{"0x5f3759df(Quake)", MagicQuake},
		{"0x5f37642f(最小误差)", MagicMinErr},
		{"0x5f375a86(迭代后最优)", MagicBestAfterNwt},
	}
	// 在 [0.01, 1000] 上按对数均匀取点，避免大数主导统计
	var xs []float32
	for i := 0; i < n; i++ {
		e := -2 + 5*float64(i)/float64(n-1) // 10^-2 → 10^3
		xs = append(xs, float32(math.Pow(10, e)))
	}
	for _, m := range magics {
		var max1, max2, sum1, sum2 float64
		for _, x := range xs {
			exact := 1 / math.Sqrt(float64(x))
			e1 := math.Abs(float64(FastInvSqrt(x, m.v))-exact) / exact
			e2 := math.Abs(float64(FastInvSqrt2(x, m.v))-exact) / exact
			if e1 > max1 {
				max1 = e1
			}
			if e2 > max2 {
				max2 = e2
			}
			sum1 += e1
			sum2 += e2
		}
		t.Logf("%-22s 1 次迭代 最大相对误差=%.3e 平均=%.3e | 2 次迭代 最大=%.3e 平均=%.3e",
			m.name, max1, sum1/float64(len(xs)), max2, sum2/float64(len(xs)))
	}
}

func BenchmarkFast1(b *testing.B) {
	x := float32(3.14)
	for i := 0; i < b.N; i++ {
		_ = FastInvSqrt(x, MagicQuake)
	}
}

func BenchmarkFast2(b *testing.B) {
	x := float32(3.14)
	for i := 0; i < b.N; i++ {
		_ = FastInvSqrt2(x, MagicQuake)
	}
}

func BenchmarkExact(b *testing.B) {
	x := float32(3.14)
	for i := 0; i < b.N; i++ {
		_ = Exact(x)
	}
}

// 用 1024 个不同输入跑，避免编译器把"同一常数反复算"优化掉。
var benchInputs = func() []float32 {
	s := make([]float32, 1024)
	for i := range s {
		s[i] = float32(0.5 + float64(i)*0.01)
	}
	return s
}()

func BenchmarkFast1Slice(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = FastInvSqrt(benchInputs[i&1023], MagicQuake)
	}
}

func BenchmarkExactSlice(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Exact(benchInputs[i&1023])
	}
}
