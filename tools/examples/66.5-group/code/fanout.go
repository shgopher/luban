package main

// fanout.go: 实验 A —— 三策略扇出基准（纯写扩散 W / 纯读扩散 R / 混合 H）。
//
// 成本口径（每条消息，全部走 Redis 命令计数）：
//   公共底噪: 1 次消息主存储写（群时间线 ZSET，存 256B 正文）。
//   W: +N 次收件箱写（每个成员一条指针，16B msg id），+ 在线推送（按网关批量，500 收件人/命令）。
//   R: 不推送（客户端轮询/按需拉取；读侧成本在 read 实验单独量）。
//   H: + 在线推送（批量），离线成员不写任何东西。
// 逐条消息统计"该消息全部命令收到 Redis 回复"的耗时 = 扇出完成耗时；
// 真实"最后一个在线成员收到"的端到端值在 e2e 实验里用 1000 个真实订阅者单独量。
import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type pipeExec struct {
	c     *redis.Client
	ctx   context.Context
	pipe  redis.Pipeliner
	cnt   int
	limit int
	cmds  int64
	err   error
}

func newPipeExec(c *redis.Client, ctx context.Context, limit int) *pipeExec {
	return &pipeExec{c: c, ctx: ctx, limit: limit}
}

func (pe *pipeExec) add(f func(p redis.Pipeliner)) {
	if pe.err != nil {
		return
	}
	if pe.cnt == 0 {
		pe.pipe = pe.c.Pipeline()
	}
	f(pe.pipe)
	pe.cnt++
	if pe.cnt >= pe.limit {
		pe.flush()
	}
}

func (pe *pipeExec) flush() {
	if pe.cnt == 0 || pe.err != nil {
		return
	}
	_, err := pe.pipe.Exec(pe.ctx)
	if err != nil {
		pe.err = err
	}
	pe.cmds += int64(pe.cnt)
	pe.cnt = 0
	pe.pipe = nil
}

func (pe *pipeExec) finish() { pe.flush() }

func memUsed(ctx context.Context, c *redis.Client) int64 {
	info, err := c.Info(ctx, "memory").Result()
	if err != nil {
		return -1
	}
	for _, ln := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(ln, "used_memory:") {
			var v int64
			fmt.Sscanf(strings.TrimPrefix(ln, "used_memory:"), "%d", &v)
			return v
		}
	}
	return -1
}

func pct(ds []time.Duration, q float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[int(float64(len(s)-1)*q)]
}

const pushBatch = 500 // 一个网关批量推送命令携带 500 个收件人（生产中一次 RPC 带一批 uid）

func fanout(strategy string, n int, p float64, m, devices int) {
	ctx := context.Background()
	c := newClient()
	if err := c.FlushAll(ctx).Err(); err != nil {
		fmt.Println("flushall err:", err)
		os.Exit(1)
	}
	time.Sleep(300 * time.Millisecond)
	base := memUsed(ctx, c)

	content := strings.Repeat("x", 243) // 消息正文 256B（尾部拼 msg id 保证 ZSET member 唯一），只写一次（主存储）
	online := int(float64(n)*p + 0.5)
	members := make([]string, n)
	for i := range members {
		members[i] = fmt.Sprintf("u%07d", i)
	}
	const gid = "g1"
	timeline := "tl:" + gid
	pushTargets := online * devices
	pushBatches := (pushTargets + pushBatch - 1) / pushBatch
	recipients := strings.Repeat("u0000001", 60) // 批量推送负载：~540B 收件人列表

	lat := make([]time.Duration, 0, m)
	pe := newPipeExec(c, ctx, 4000)
	t0All := time.Now()
	for i := 1; i <= m; i++ {
		t0 := time.Now()
		msgID := fmt.Sprintf("m%012d", i)
		body := content + "|" + msgID // 256B，含唯一 msg id，ZSET 不会去重
		// 公共底噪：消息主存储写一次（正文 256B）
		pe.add(func(pl redis.Pipeliner) {
			pl.ZAdd(ctx, timeline, redis.Z{Score: float64(i), Member: body})
		})
		// 写扩散：每个成员收件箱各写一条指针
		if strategy == "w" {
			for _, u := range members {
				u := u
				pe.add(func(pl redis.Pipeliner) {
					pl.ZAdd(ctx, "in:"+u, redis.Z{Score: float64(i), Member: msgID})
				})
			}
		}
		// 在线推送（W/H 相同）：按网关批量
		if strategy != "r" {
			for b := 0; b < pushBatches; b++ {
				b := b
				pe.add(func(pl redis.Pipeliner) {
					pl.Publish(ctx, fmt.Sprintf("gw:%04d", b%1024), recipients+msgID)
				})
			}
		}
		pe.finish()
		lat = append(lat, time.Since(t0))
	}
	wall := time.Since(t0All)
	if pe.err != nil {
		fmt.Println("ERR:", pe.err)
		os.Exit(1)
	}
	after := memUsed(ctx, c)
	var entries int
	if strategy == "w" {
		entries = n*m + m
	} else {
		entries = m
	}
	fmt.Printf("strategy=%s n=%d p=%.2f m=%d devices=%d online=%d\n", strategy, n, p, m, devices, online)
	fmt.Printf("  redis_cmds=%d (per_msg=%.1f) wall=%v throughput_msgs_per_s=%.1f\n",
		pe.cmds, float64(pe.cmds)/float64(m), wall.Round(time.Millisecond), float64(m)/wall.Seconds())
	fmt.Printf("  latency_p50=%v latency_p99=%v latency_max=%v\n",
		pct(lat, 0.50), pct(lat, 0.99), maxDur(lat))
	fmt.Printf("  mem_base=%d mem_after=%d mem_delta=%d entries=%d bytes_per_entry=%.1f\n",
		base, after, after-base, entries, float64(after-base)/float64(entries))
	_ = pushTargets
}
