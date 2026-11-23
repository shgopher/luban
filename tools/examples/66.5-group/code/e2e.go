package main

// e2e.go: 实验 A3 —— 真实订阅者口径的端到端延迟：
// 把 1000 个在线成员做成真实 Redis 订阅者（4 条连接，每条 250 个频道），
// 逐条消息统计"发出 → 最后一个在线成员收到"的耗时，验证 A 的扇出完成耗时口径是否失真。
import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func e2e(n, m int) {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)

	payload := func(seq int) string {
		// 256 字节，尾部带序号，订阅者靠尾部序号归因
		s := fmt.Sprintf("%0255d|%08d", seq, seq)
		return s
	}
	chans := make([]string, n)
	for i := 0; i < n; i++ {
		chans[i] = fmt.Sprintf("ch:g1:u%07d", i)
	}

	// 4 条订阅连接，每条订阅 250 个频道
	const conns = 4
	recvCh := make(chan int, n*m)
	var wg sync.WaitGroup
	for k := 0; k < conns; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			sub := redis.NewClient(&redis.Options{Addr: redisAddr})
			lo, hi := k*n/conns, (k+1)*n/conns
			ps := sub.Subscribe(ctx, chans[lo:hi]...)
			defer ps.Close()
			for msg := range ps.Channel(redis.WithChannelSize(4096)) {
				var seq int
				p := msg.Payload
				if idx := strings.LastIndex(p, "|"); idx >= 0 {
					fmt.Sscanf(p[idx+1:], "%08d", &seq)
				}
				recvCh <- seq
			}
		}(k)
	}
	time.Sleep(300 * time.Millisecond) // 等订阅就绪

	lat := make([]time.Duration, 0, m)
	for seq := 1; seq <= m; seq++ {
		t0 := time.Now()
		// 管道化 publish 到 n 个成员频道
		const chunk = 2000
		for done := 0; done < n; done += chunk {
			pipe := c.Pipeline()
			for i := done; i < done+chunk && i < n; i++ {
				pipe.Publish(ctx, chans[i], payload(seq))
			}
			if _, err := pipe.Exec(ctx); err != nil {
				fmt.Println("publish err:", err)
				os.Exit(1)
			}
		}
		// 等最后一个成员收到（超时 3s 兜底）
		got := 0
		deadline := time.After(3 * time.Second)
	wait:
		for got < n {
			select {
			case s := <-recvCh:
				if s == seq {
					got++
				}
			case <-deadline:
				break wait
			}
		}
		lat = append(lat, time.Since(t0))
		if got < n {
			fmt.Printf("  seq=%d only %d/%d received (timeout)\n", seq, got, n)
		}
	}
	all := append([]time.Duration(nil), lat...)
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	fmt.Printf("e2e n=%d m=%d subscribers=%d conns=%d\n", n, m, n, conns)
	fmt.Printf("  send_to_last_member p50=%v p99=%v max=%v\n",
		pct(lat, 0.50), pct(lat, 0.99), all[len(all)-1])
}
