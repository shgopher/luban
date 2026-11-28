package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
)

// 1) 哈希难题：统计“找到 k 个前导零比特”需要尝试多少次，期望是 2^k。
func leadingZeroBits(b [32]byte) int {
	n := 0
	for _, x := range b {
		if x == 0 {
			n += 8
			continue
		}
		for i := 7; i >= 0; i-- {
			if x&(1<<uint(i)) == 0 {
				n++
			} else {
				return n
			}
		}
	}
	return n
}

func mineOnce(bits int, seed uint64) uint64 {
	var buf [8]byte
	for nonce := uint64(0); ; nonce++ {
		binary.LittleEndian.PutUint64(buf[:], seed^nonce)
		h := sha256.Sum256(buf[:])
		if leadingZeroBits(h) >= bits {
			return nonce + 1
		}
	}
}

// 2) 白皮书模型：诚实链先领先 z 个块；在诚实网络挖出这 z 个块的同一段时间里，
// 攻击者挖到的块数 K ~ Poisson(λ)，λ = z·q/p。
// 若 K > z，攻击者已经领先；否则从落后 z-K 个块开始追，追上（差距到 0）即成功。
func poisson(lambda float64, rng *rand.Rand) int {
	l, k, p := math.Exp(-lambda), 0, 1.0
	for {
		p *= rng.Float64()
		if p <= l {
			return k
		}
		k++
	}
}

func attackSuccess(q float64, z int, trials int, rng *rand.Rand) float64 {
	if q >= 0.5 {
		return 1
	}
	p := 1 - q
	win := 0
	for t := 0; t < trials; t++ {
		k := poisson(float64(z)*q/p, rng)
		if k > z {
			win++
			continue
		}
		gap := z - k
		for step := 0; step < 20000; step++ {
			if gap == 0 {
				win++
				break
			}
			if rng.Float64() < q {
				gap--
			} else {
				gap++
			}
			if gap > z+300 {
				break
			}
		}
	}
	return float64(win) / float64(trials)
}

// 白皮书的闭式解
func catchUpProb(q float64, z int) float64 {
	p := 1 - q
	if q >= p {
		return 1
	}
	lambda := float64(z) * q / p
	sum, term := 0.0, math.Exp(-lambda)
	for k := 0; k <= z; k++ {
		if k > 0 {
			term *= lambda / float64(k)
		}
		sum += term * (1 - math.Pow(q/p, float64(z-k)))
	}
	return 1 - sum
}

func main() {
	fmt.Println("=== 哈希难题：找到 k 个前导零比特要试多少次（每档 300 次试验）===")
	fmt.Printf("%-6s %-12s %-12s %-10s\n", "位数", "期望 2^k", "实测均值", "最大")
	for _, bits := range []int{8, 12, 16, 20} {
		var sum, max uint64
		for i := 0; i < 300; i++ {
			x := mineOnce(bits, uint64(i)*7919+uint64(bits))
			sum += x
			if x > max {
				max = x
			}
		}
		fmt.Printf("%-6d %-12d %-12d %-10d\n", bits, 1<<uint(bits), sum/300, max)
	}

	fmt.Println("\n=== 双花：诚实链领先 z 个块，攻击者算力占比 q ===")
	fmt.Printf("%-6s %-4s %-10s %-10s\n", "q", "z", "模拟", "公式")
	rng := rand.New(rand.NewSource(7))
	for _, q := range []float64{0.1, 0.2, 0.3, 0.4} {
		for _, z := range []int{1, 3, 6, 12} {
			sim := attackSuccess(q, z, 100000, rng)
			fmt.Printf("%-6.2f %-4d %-10.5f %-10.5f\n", q, z, sim, catchUpProb(q, z))
		}
	}

	fmt.Println("\n=== 出块时间与难度：算力翻倍，同样的 target 出块时间减半 ===")
	fmt.Printf("%-12s %-16s %-12s\n", "相对算力", "期望出块(分钟)", "难度倍数")
	for _, h := range []float64{0.5, 1, 2, 4} {
		fmt.Printf("%-12.1f %-16.1f %-12.1f\n", h, 10/h, h)
	}
}
