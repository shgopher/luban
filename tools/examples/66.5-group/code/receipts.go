package main

// receipts.go: 实验 B —— 群已读回执：逐人回执 vs 聚合计数。
// 场景：N=5000 人群，一条消息被 1000 人读过。
//   P1 逐人回执（推发送者）: 每人 1 次状态写 + 1 次推送 = 2000 条命令。
//   P2 逐人回执（广播全群）: 每人 1 次状态写 + N 次推送 = 1+N 条命令/人（O(N^2) 的来源）。
//   AGG 聚合计数: Lua 去重 + HINCRBY，每人 1 条命令；发送者只收 1 次"已读 1000/5000"推送。
// 另测三种"已读集合"编码的单条消息内存成本：bitmap / hash / 纯计数器。
import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var dedupScript = redis.NewScript(`
local added = redis.call('HSETNX', KEYS[1], ARGV[1], '1')
if added == 1 then
  return redis.call('HINCRBY', KEYS[2], 'read', 1)
end
return -1
`)

func receipts(n, readers int) {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)
	time.Sleep(100 * time.Millisecond)
	base := memUsed(ctx, c)

	const senderCh = "ch:g1:u0000001" // 发送者的推送频道

	// ---------- P1: 逐人回执，只推给发送者 ----------
	c.FlushAll(ctx)
	t0 := time.Now()
	var cmds int64
	for i := 0; i < readers; i++ {
		uid := fmt.Sprintf("u%05d", i)
		pipe := c.Pipeline()
		pipe.HSet(ctx, "rcpt:msg1", uid, "1")
		pipe.Publish(ctx, senderCh, uid)
		if _, err := pipe.Exec(ctx); err != nil {
			fmt.Println("ERR", err)
			os.Exit(1)
		}
		cmds += 2
	}
	wallP1 := time.Since(t0)
	memP1 := memUsed(ctx, c) - base

	// ---------- P2: 逐人回执，广播给全群 ----------
	c.FlushAll(ctx)
	t0 = time.Now()
	cmds = 0
	const chunk = 500
	otherCh := make([]string, n)
	for i := 0; i < n; i++ {
		otherCh[i] = fmt.Sprintf("ch:g1:u%05d", i)
	}
	for i := 0; i < readers; i++ {
		uid := fmt.Sprintf("u%05d", i)
		sw := c.Pipeline()
		sw.HSet(ctx, "rcpt:msg1", uid, "1")
		sw.Exec(ctx)
		cmds++
		for done := 0; done < n; done += chunk {
			pipe := c.Pipeline()
			for j := done; j < done+chunk && j < n; j++ {
				pipe.Publish(ctx, otherCh[j], uid)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				fmt.Println("ERR", err)
				os.Exit(1)
			}
			cmds += int64(min(chunk, n-done))
		}
	}
	wallP2 := time.Since(t0)
	cmdsP2 := cmds

	// ---------- AGG: 聚合计数（Lua 去重 + HINCRBY），发送者收 1 次推送 ----------
	c.FlushAll(ctx)
	t0 = time.Now()
	cmds = 0
	pushCount := 0
	for i := 0; i < readers; i++ {
		uid := fmt.Sprintf("u%05d", i)
		if _, err := dedupScript.Run(ctx, c, []string{"read:msg1", "cnt:msg1"}, uid).Result(); err != nil {
			fmt.Println("ERR", err)
			os.Exit(1)
		}
		cmds++
		if i%chunk == 0 { // 发送者按 500 人一批收"已读进度"，最终一条
			c.Publish(ctx, senderCh, fmt.Sprintf("read %d/%d", i+1, n))
			cmds++
			pushCount++
		}
	}
	c.Publish(ctx, senderCh, fmt.Sprintf("read %d/%d", readers, n))
	cmds++
	pushCount++
	wallAgg := time.Since(t0)
	cmdsAgg := cmds
	memAgg := memUsed(ctx, c) - base
	got, _ := c.HGet(ctx, "cnt:msg1", "read").Int64()

	fmt.Printf("receipts n=%d readers=%d\n", n, readers)
	fmt.Printf("  P1  per-person->sender   cmds=%d wall=%v p50_per_receipt=%v mem_delta=%d\n",
		readers*2, wallP1.Round(time.Millisecond), (wallP1 / time.Duration(readers)).Round(time.Microsecond), memP1)
	fmt.Printf("  P2  per-person->group    cmds=%d wall=%v per_receipt=%v (O(N^2): 1 state write + N pushes)\n",
		cmdsP2, wallP2.Round(time.Millisecond), (wallP2 / time.Duration(readers)).Round(time.Microsecond))
	fmt.Printf("  AGG aggregate->sender    cmds=%d wall=%v p50_per_receipt=%v mem_delta=%d push_to_sender=%d counter=%d\n",
		cmdsAgg, wallAgg.Round(time.Millisecond), (wallAgg / time.Duration(readers)).Round(time.Microsecond), memAgg, pushCount, got)
	fmt.Printf("  ratio P1/AGG commands=%.2fx  P2/AGG commands=%.0fx\n",
		float64(readers*2)/float64(cmdsAgg), float64(cmdsP2)/float64(cmdsAgg))

	// ---------- 已读集合三种编码的单消息内存 ----------
	enc := []struct {
		name string
		fn   func()
	}{
		{"bitmap SETBIT/人", func() {
			for i := 0; i < n; i++ {
				c.SetBit(ctx, "bits:msg1", int64(i), 1)
			}
		}},
		{"hash HSETNX/人", func() {
			for i := 0; i < n; i++ {
				c.HSetNX(ctx, "set:msg1", fmt.Sprintf("u%05d", i), "1")
			}
		}},
		{"counter HINCRBY/次", func() {
			for i := 0; i < n; i++ {
				c.HIncrBy(ctx, "cntonly:msg1", "read", 1)
			}
		}},
	}
	for _, e := range enc {
		c.FlushAll(ctx)
		time.Sleep(50 * time.Millisecond)
		b := memUsed(ctx, c)
		e.fn()
		a := memUsed(ctx, c)
		fmt.Printf("  mem[%s] bytes_per_message_for_n=%d = %d (%.1f B/member)\n", e.name, n, a-b, float64(a-b)/float64(n))
	}
	c.FlushAll(ctx)
}
