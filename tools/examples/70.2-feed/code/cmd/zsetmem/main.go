// 收件箱 ZSET 的内存账（同机实测，Redis 127.0.0.1:16392）
// 场景 A（生产形态）：1000 个 key × 每 key 1000 member = 100 万 member（一个用户一个收件箱）
// 场景 B（旧口径对照）：3000 个 key × 每 key 1 member（key 的固定开销摊到 1 条上）
// 口径：FLUSHALL 后 INFO memory 的 used_memory 增量 ÷ member 总数
// 运行：go run ./cmd/zsetmem
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const addr = "127.0.0.1:16392"

var (
	ctx = context.Background()
	rdb *redis.Client
)

func main() {
	rdb = redis.NewClient(&redis.Options{Addr: addr, PoolSize: 8})
	defer rdb.Close()
	pong, err := rdb.Ping(ctx).Result()
	must(err)
	ver, _ := rdb.Do(ctx, "INFO", "server").Text()
	fmt.Printf("Redis %s @ %s, PING=%s\n", infoField(ver, "redis_version"), addr, pong)
	fmt.Println("实测：收件箱 ZSET 每 member 内存成本（INFO memory used_memory 增量 ÷ member 数）")

	type scenario struct {
		name          string
		keys, members int
		padded        bool
	}
	scenes := []scenario{
		{"A 生产形态：1000 个收件箱 × 每箱 1000 条（member 补零）", 1000, 1000, true},
		{"B 同上，但 member 不补零（feed:%d）", 1000, 1000, false},
		{"C 旧口径：3000 个收件箱 × 每箱 1 条", 3000, 1, true},
	}

	fmt.Println()
	fmt.Println("| 场景 | key 数 | 每 key member | 总 member | used_memory 增量 | 每 member | 每用户(1000 条) |")
	fmt.Println("|---|---|---|---|---|---|---|")
	results := map[string]struct {
		delta, perMember, perUser int64
	}{}
	for _, sc := range scenes {
		must(rdb.FlushAll(ctx).Err())
		base := settleMemory()
		load(sc.keys, sc.members, sc.padded)
		after := settleMemory()
		delta := after - base
		total := int64(sc.keys * sc.members)
		perMember := delta / total
		// 单用户 1000 条的容量按本场景的每 member 成本外推
		perUser := perMember * 1000
		if sc.members == 1000 {
			perUser = delta / int64(sc.keys)
		}
		results[sc.name] = struct {
			delta, perMember, perUser int64
		}{delta, perMember, perUser}
		fmt.Printf("| %s | %d | %d | %d | %s | %d B | %s |\n",
			sc.name, sc.keys, sc.members, total, human(delta), perMember, human(perUser))
	}

	a := results["A 生产形态：1000 个收件箱 × 每箱 1000 条（member 补零）"]
	b := results["B 同上，但 member 不补零（feed:%d）"]
	c := results["C 旧口径：3000 个收件箱 × 每箱 1 条"]
	fmt.Println()
	fmt.Printf("结论：\n")
	fmt.Printf("  1) 生产形态每 member = %d B，单个用户 1000 条收件箱 = %s；100 万活跃用户 ≈ %s（只存 ID 与 score，不含正文）\n",
		a.perMember, human(a.perUser), human(a.perUser*1_000_000))
	fmt.Printf("  2) 旧口径（3000 个「只有 1 条」的 key）= %d B/条：key 的固定开销摊到 1 条上，高估约 %.0f%%；这也是不能拿小 key 的读数外推大 key 的原因\n",
		c.perMember, (float64(c.perMember)/float64(a.perMember)-1)*100)
	fmt.Printf("  3) member 补零的内存代价：补零 %d B/条 vs 不补零 %d B/条，每百万用户多 %s（用内存换「字符串序=数值序」）\n",
		a.perMember, b.perMember, human((a.perUser-b.perUser)*1_000_000))
	fmt.Printf("  4) 100 万 member 的实测总增量 = %s，可反推「活跃用户数 × 每箱长度」的内存预算\n", human(a.delta))
	fmt.Println()

	// 采样单个 1000 条 key 的真实占用（MEMORY USAGE 包含 key 与编码开销）
	must(rdb.FlushAll(ctx).Err())
	time.Sleep(200 * time.Millisecond)
	load(1, 1000, true)
	mu := rdb.Do(ctx, "MEMORY", "USAGE", "inbox:u0", "SAMPLES", "0").Val()
	fmt.Printf("  5) MEMORY USAGE inbox:u0（1000 条，含 key 与分配器开销）= %v 字节\n", mu)
	must(rdb.FlushAll(ctx).Err())
}

func load(keys, members int, padded bool) {
	for k := 0; k < keys; k++ {
		key := "inbox:u" + strconv.Itoa(k)
		pipe := rdb.Pipeline()
		for m := 0; m < members; m++ {
			score := float64(1_700_000_000_000 + m*1000)
			member := fmt.Sprintf("feed:%d", m+1)
			if padded {
				member = fmt.Sprintf("feed:%012d", m+1)
			}
			pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	}
}

// settleMemory：等 FLUSHALL 的惰性释放完成后取稳定读数
func settleMemory() int64 {
	prev := usedMemory()
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		cur := usedMemory()
		if cur == prev {
			return cur
		}
		prev = cur
	}
	return prev
}

func usedMemory() int64 {
	s, err := rdb.Do(ctx, "INFO", "memory").Text()
	must(err)
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "used_memory:") {
			v, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "used_memory:")), 10, 64)
			return v
		}
	}
	return 0
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func infoField(s, key string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(line[len(key)+1:])
		}
	}
	return "?"
}

func must(err error) {
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}
