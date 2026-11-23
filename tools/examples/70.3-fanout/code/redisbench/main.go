// 70.3 实测 B：Redis pipeline 批量 ZADD 的吞吐，批大小 100 vs 1000 同进程 A/B。
// 场景等价于「给 10 万个粉丝各写一条收件箱」：10 万个不同的 inbox key，每个 ZADD 一条。
// 方法：交替 5 轮、每轮用独立 key 前缀（不 FLUSHALL，避免异步释放内存污染下一轮），取中位数。
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	addr   = "127.0.0.1:16393"
	rows   = 100000
	rounds = 5
	seqN   = 20000 // 单条 ZADD 的对照样本（不逐条跑 10 万次，省时）
)

type result struct {
	batch int
	dur   time.Duration
	rate  float64
	pipe  int
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 4})
	defer rdb.Close()
	info, err := rdb.Info(ctx, "server").Result()
	must(err)
	persist := rdb.ConfigGet(ctx, "save").Val()
	fmt.Printf("# env: macOS arm64, Go %s, Redis %s, save=%v, date=%s\n",
		runtime.Version(), grep(info, "redis_version:"), persist["save"], time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("# scenario: %d 个粉丝各写 1 条收件箱（%d 个不同 inbox key，ZADD 1 条/ key）\n", rows, rows)

	must(rdb.FlushAll(ctx).Err())
	time.Sleep(200 * time.Millisecond)
	baseMem := usedMemory(ctx, rdb)
	fmt.Printf("# baseline used_memory=%d bytes\n", baseMem)

	// 对照：逐条 ZADD（batch=1），只跑 2 万行并换算
	fmt.Println("\n# baseline: 逐条 ZADD（2 万行）")
	t0 := time.Now()
	for i := 0; i < seqN; i++ {
		must(rdb.ZAdd(ctx, fmt.Sprintf("seq:inbox:%d", i), redis.Z{Score: float64(1760000000000 + i), Member: "feed:000000009001"}).Err())
	}
	seqEl := time.Since(t0)
	fmt.Printf("逐条 ZADD: %d 行, 耗时 %s, 吞吐 %.0f 行/秒\n", seqN, seqEl.Round(time.Millisecond), float64(seqN)/seqEl.Seconds())
	must(rdb.FlushAll(ctx).Err())

	// A/B：批 100 vs 批 1000，交替 5 轮
	all := make([][]result, 2) // 0 -> batch 100, 1 -> batch 1000
	for r := 0; r < rounds; r++ {
		order := []int{100, 1000}
		if r%2 == 1 {
			order = []int{1000, 100}
		}
		for _, batch := range order {
			prefix := fmt.Sprintf("r%d:b%d", r, batch)
			dur, pipes := runBatch(ctx, rdb, batch, prefix)
			rate := float64(rows) / dur.Seconds()
			idx := 0
			if batch == 1000 {
				idx = 1
			}
			all[idx] = append(all[idx], result{batch, dur, rate, pipes})
			fmt.Printf("轮 %d: batch=%-4d 耗时=%-10s 流水线次数=%-5d 吞吐=%.0f 行/秒\n",
				r+1, batch, dur.Round(time.Millisecond), pipes, rate)
		}
	}
	var med1000 float64
	for i, label := range []string{"batch=100", "batch=1000"} {
		rs := make([]float64, 0, len(all[i]))
		for _, x := range all[i] {
			rs = append(rs, x.rate)
		}
		sort.Float64s(rs)
		med := rs[len(rs)/2]
		if i == 1 {
			med1000 = med
		}
		fmt.Printf("MEDIAN %-12s 吞吐=%.0f 行/秒（%d 轮，min=%.0f max=%.0f）\n", label, med, len(rs), rs[0], rs[len(rs)-1])
	}
	must(rdb.FlushAll(ctx).Err())
	time.Sleep(200 * time.Millisecond)

	// 内存实测：再填 10 万个不同的 inbox key，读 used_memory 增量与单 key MEMORY USAGE
	fillStart := usedMemory(ctx, rdb)
	runBatch(ctx, rdb, 1000, "mem")
	fillEnd := usedMemory(ctx, rdb)
	keys := rdb.DBSize(ctx).Val()
	perKey, err := rdb.MemoryUsage(ctx, "mem:inbox:12345", 0).Result()
	must(err)
	fmt.Printf("\n# 内存实测: keys=%d used_memory=%d→%d（增量 %d bytes）MEMORY_USAGE(单 key)=%d bytes\n",
		keys, fillStart, fillEnd, fillEnd-fillStart, perKey)
	perRow := float64(fillEnd-fillStart) / float64(keys)
	fmt.Printf("# 每行实测内存 = %d/%d = %.0f bytes/行（含 key 开销）\n", fillEnd-fillStart, keys, perRow)
	must(rdb.FlushAll(ctx).Err())

	fmt.Println("\n# 推算（用 batch=1000 中位吞吐；只覆盖 Redis 命令通道，不含关系分页/序列化/跨机房 RTT/多副本）")
	fans := 120000000.0
	sec1 := fans / med1000
	fmt.Printf("单连接: 120000000 / %.0f = %.1f 秒 (%.1f 分钟)\n", med1000, sec1, sec1/60)
	for _, c := range []float64{16, 32, 64} {
		fmt.Printf("%.0f 连接并行: %.1f 秒\n", c, sec1/c)
	}
	fmt.Printf("内存: 120000000 × %.0f bytes = %.1f GB（推算）\n", perRow, fans*perRow/1e9)
}

func runBatch(ctx context.Context, rdb *redis.Client, batch int, prefix string) (time.Duration, int) {
	start := time.Now()
	pipes := 0
	for i := 0; i < rows; i += batch {
		end := i + batch
		if end > rows {
			end = rows
		}
		pipe := rdb.Pipeline()
		for j := i; j < end; j++ {
			pipe.ZAdd(ctx, fmt.Sprintf("%s:inbox:%d", prefix, j), redis.Z{
				Score:  float64(1760000000000 + j),
				Member: "feed:000000009001",
			})
		}
		_, err := pipe.Exec(ctx)
		must(err)
		pipes++
	}
	return time.Since(start), pipes
}

func usedMemory(ctx context.Context, rdb *redis.Client) int64 {
	info := rdb.Info(ctx, "memory").Val()
	s := grep(info, "used_memory:")
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func grep(s, prefix string) string {
	for i := 0; i+len(prefix) <= len(s); i++ {
		if s[i:i+len(prefix)] == prefix {
			j := i + len(prefix)
			for j < len(s) && s[j] != '\n' && s[j] != '\r' {
				j++
			}
			return s[i+len(prefix) : j]
		}
	}
	return ""
}

func must(err error) {
	if err != nil {
		fmt.Println("FATAL:", err)
		os.Exit(1)
	}
}
