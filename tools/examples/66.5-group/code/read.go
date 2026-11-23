package main

// read.go: 实验 A2 —— 读侧放大对比。
// 同为"用户上线补齐 50 条未读"：
//   W（写扩散）: 每条消息已物化进"用户维度收件箱"，补齐 = 1 次 ZRANGEBYSCORE。
//   R/H（读扩散）: 消息只在"群时间线"，用户每加了一个有待读消息的群，就要多读一次群时间线。
// 变量 G = 用户所在群数，分别 10 / 50 / 200。
import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const readSample = 200 // 每次测 200 个用户上线

func readSide(groups int) {
	ctx := context.Background()
	c := newClient()
	if err := c.FlushAll(ctx).Err(); err != nil {
		fmt.Println("flushall err:", err)
		os.Exit(1)
	}
	payload := "msg-payload-256B"

	// 造数据：G 个群时间线，每群 1000 条；每用户在这 G 个群里各欠 1 条未读
	pe := newPipeExec(c, ctx, 4000)
	for g := 0; g < groups; g++ {
		key := fmt.Sprintf("tl:g%03d", g)
		for i := 0; i < 1000; i++ {
			pe.add(func(pl redis.Pipeliner) {
				pl.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: payload})
			})
		}
	}
	// 写扩散侧：用户维度收件箱，每个用户只有 50 条未读（跨群合并后）
	for u := 0; u < readSample; u++ {
		key := fmt.Sprintf("in:u%04d", u)
		for i := 0; i < 50; i++ {
			pe.add(func(pl redis.Pipeliner) {
				pl.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: payload})
			})
		}
	}
	pe.finish()

	// 读扩散路径：G 次 ZRANGEBYSCORE（每次取最后 50 条）
	latR := make([]time.Duration, 0, readSample)
	cmdsR := 0
	tR := time.Now()
	for u := 0; u < readSample; u++ {
		t0 := time.Now()
		for g := 0; g < groups; g++ {
			c.ZRangeByScore(ctx, fmt.Sprintf("tl:g%03d", g), &redis.ZRangeBy{
				Min: "950", Max: "+inf", Offset: 0, Count: 50,
			}).Result()
			cmdsR++
		}
		latR = append(latR, time.Since(t0))
	}
	wallR := time.Since(tR)

	// 写扩散路径：1 次 ZRANGEBYSCORE 读自己的合并收件箱
	latW := make([]time.Duration, 0, readSample)
	tW := time.Now()
	for u := 0; u < readSample; u++ {
		t0 := time.Now()
		c.ZRangeByScore(ctx, fmt.Sprintf("in:u%04d", u), &redis.ZRangeBy{
			Min: "-inf", Max: "+inf", Offset: 0, Count: 50,
		}).Result()
		latW = append(latW, time.Since(t0))
	}
	wallW := time.Since(tW)

	fmt.Printf("read_side groups_per_user=%d users=%d\n", groups, readSample)
	fmt.Printf("  R/H path: cmds=%d cmds_per_user=%d wall=%v p50=%v p99=%v\n",
		cmdsR, cmdsR/readSample, wallR.Round(time.Microsecond), pct(latR, 0.5), pct(latR, 0.99))
	fmt.Printf("  W   path: cmds=%d cmds_per_user=1 wall=%v p50=%v p99=%v\n",
		readSample, wallW.Round(time.Microsecond), pct(latW, 0.5), pct(latW, 0.99))
	fmt.Printf("  read_amp_ratio=%.1fx\n", float64(cmdsR)/float64(readSample))
	sort.Slice(latR, func(i, j int) bool { return latR[i] < latR[j] })
}
