// Package rsqrt 演示《左耳听风》第 13 讲那条带 ``// what the fuck?`` 的代码：
// IEEE 754 位表示 + log2(1+m) 的线性近似 + 牛顿迭代，三段拼出"平方根倒数"。
//
// 三段各管一件事：
//  1. 位表示：读出 S/E/M，或者反过来说明"为什么 3.14 存不准"。
//  2. 线性近似：把 log2(1+m) ≈ m + σ 代入 log2(y) = -½log2(x)，得到
//     I_y ≈ R - ½·I_x —— 一次整数减法和右移就给出可用的初值。
//  3. 牛顿迭代：初值越准，迭代次数越少；一次迭代就能到 0.2% 以内。
package rsqrt

import "math"

// MagicQuake 是 Quake III Arena 里那个魔数。
// 它对应 R = (127-σ)·(3/2)·2^23 中的 σ ≈ 0.0450465，魔数本身怎么来的没有定论。
const MagicQuake uint32 = 0x5f3759df

// 另外三个常被引用的候选值：
//   - MagicMinErr：Chris Lomont 按"误差最小"推出的最佳 R
//   - MagicBestAfterNewton：Lomont 按"1–2 次牛顿迭代后最精确"暴力搜索出的 R
//   - MagicDouble：64 位双精度的对应魔数（0x5fe6ec85e7de30da）
const (
	MagicMinErr         uint32 = 0x5f37642f
	MagicBestAfterNwt   uint32 = 0x5f375a86
	MagicQuake64        uint64 = 0x5fe6ec85e7de30da
	MagicDoubleBest64   uint64 = 0x5fe6eb50c7aa19f9
)

// FastInvSqrt 是原文那段代码的 Go 版本，迭代一次。
func FastInvSqrt(x float32, magic uint32) float32 {
	i := math.Float32bits(x)
	i = magic - (i >> 1) // 这里就是 log2(y) = -½log2(x) 的整数版本
	y := math.Float32frombits(i)
	// 牛顿迭代：y = y·(1.5 - 0.5·x·y²)，对应 f(y)=1/y²-x
	y = y * (1.5 - 0.5*x*y*y)
	return y
}

// FastInvSqrt2 迭代两次。原文里第二次被注释掉了，因为图形学不需要那么准。
func FastInvSqrt2(x float32, magic uint32) float32 {
	i := math.Float32bits(x)
	i = magic - (i >> 1)
	y := math.Float32frombits(i)
	y = y * (1.5 - 0.5*x*y*y)
	y = y * (1.5 - 0.5*x*y*y)
	return y
}

// Exact 是"老实写法"：先开方再取倒数。它的成本就是我们要对比的基准。
func Exact(x float32) float32 { return 1 / float32(math.Sqrt(float64(x))) }

// Parts 是一个 float32 的三段拆解结果。
type Parts struct {
	Sign     uint32 // S：1 bit，正数 0
	Exponent uint32 // E：8 bit，原始存储值（真值 = E-127）
	Mantissa uint32 // M：23 bit，[0, 2^23) 的定点小数
}

// Split 按 IEEE 754 单精度拆出 S/E/M。
func Split(x float32) Parts {
	b := math.Float32bits(x)
	return Parts{Sign: b >> 31, Exponent: (b >> 23) & 0xff, Mantissa: b & 0x7fffff}
}

// Compose 用 S/E/M 拼回 float32，用于证明"浮点数是离散的"。
func Compose(p Parts) float32 {
	b := (p.Sign << 31) | (p.Exponent << 23) | p.Mantissa
	return math.Float32frombits(b)
}

// RoundTripError 返回 x 存成 float32 后与十进制字面量的相对误差。
// 3.14 与 0.015 都存不准，这不是 bug，是二进制位宽的必然。
func RoundTripError(dec float64, as float32) float64 {
	return (float64(as) - dec) / dec
}
