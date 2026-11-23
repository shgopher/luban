package main

// push.go: 实验 A4 —— 在线推送的两种实现口径：逐人推送 vs 按网关批量推送。
// 同样给 online 个在线成员推一条 256B 消息：逐人 = online 条 PUBLISH；批量 = ceil(online/500) 条 PUBLISH。
import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func pushMode(online, m, batch int) {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)
	payload := strings.Repeat("x", 256) + "|00000001"
	recipients := strings.Repeat("u0000001", batch)

	run := func(perMember bool) (cmds int64, wall time.Duration, lats []time.Duration) {
		pe := newPipeExec(c, ctx, 4000)
		t0All := time.Now()
		for i := 1; i <= m; i++ {
			t0 := time.Now()
			if perMember {
				for j := 0; j < online; j++ {
					j := j
					pe.add(func(pl redis.Pipeliner) {
						pl.Publish(ctx, fmt.Sprintf("ch:u%07d", j), payload)
					})
				}
			} else {
				nb := (online + batch - 1) / batch
				for b := 0; b < nb; b++ {
					b := b
					pe.add(func(pl redis.Pipeliner) {
						pl.Publish(ctx, fmt.Sprintf("gw:%04d", b%1024), recipients)
					})
				}
			}
			pe.finish()
			lats = append(lats, time.Since(t0))
		}
		wall = time.Since(t0All)
		if pe.err != nil {
			fmt.Println("ERR:", pe.err)
			os.Exit(1)
		}
		return pe.cmds, wall, lats
	}

	cmds1, wall1, lat1 := run(true)
	cmds2, wall2, lat2 := run(false)
	fmt.Printf("push online=%d m=%d batch=%d\n", online, m, batch)
	fmt.Printf("  per-member : cmds=%d per_msg=%.0f wall=%v p50=%v p99=%v\n",
		cmds1, float64(cmds1)/float64(m), wall1.Round(time.Millisecond), pct(lat1, 0.5), pct(lat1, 0.99))
	fmt.Printf("  batched    : cmds=%d per_msg=%.1f wall=%v p50=%v p99=%v\n",
		cmds2, float64(cmds2)/float64(m), wall2.Round(time.Millisecond), pct(lat2, 0.5), pct(lat2, 0.99))
	fmt.Printf("  cmd_ratio=%.0fx\n", float64(cmds1)/float64(cmds2))
	c.FlushAll(ctx)
}
