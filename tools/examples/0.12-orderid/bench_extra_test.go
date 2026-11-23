// 这个文件只放两个「对照」基准：
//   BenchmarkSpinClock —— 序列用尽时自旋等下一毫秒的版本（0.12 里说它把单次拖到 245 ns）
//   BenchmarkMutex      —— 一次加解锁的裸成本（0.12 Q8 用它说明锁只占单次生成的四分之一）
// 两者都不是推荐的实现，只为正文里的数字可复现。

package orderid

import (
	"sync"
	"testing"
	"time"
)

// genSpin 是反例：不借下一毫秒，而是等真实时钟跨过这一毫秒。
type genSpin struct {
	mu   sync.Mutex
	node int64
	last int64
	seq  int64
}

func (g *genSpin) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := time.Now().UnixMilli() - epoch
	if ms == g.last {
		g.seq++
		if g.seq > seqMask { // 自旋等下一毫秒
			for ms <= g.last {
				ms = time.Now().UnixMilli() - epoch
			}
			g.seq = 0
			g.last = ms
		}
	} else {
		g.seq = 0
		g.last = ms
	}
	return ms<<(nodeBits+seqBits) | g.node<<seqBits | g.seq
}

var gspin = &genSpin{node: 1}

func BenchmarkSpinClock(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = gspin.Next()
	}
}

var mu sync.Mutex
var mcount int64

func BenchmarkMutex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		mu.Lock()
		mcount++
		mu.Unlock()
	}
}
