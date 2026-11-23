package main

// memmap.go: 内存结构标定 —— 用 MEMORY USAGE 精确量每种结构的单条成本。
// 为什么不用 INFO memory 差值：小结构（如 bitmap 625B）会被 1 MB 级的基线抖动淹没；
// MEMORY USAGE 直接给 key 的实际占用，且能同时看到 OBJECT ENCODING（listpack / skiplist）。
import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

func memUsage(ctx context.Context, c *redis.Client, key string) int64 {
	n, err := c.MemoryUsage(ctx, key).Result()
	if err != nil {
		return -1
	}
	return n
}

func memmap() {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)
	pointer := "m%012d"  // 收件箱指针：13 字节
	body := strings.Repeat("x", 243) // 正文 243B + "|" + 13B msg id = 256B

	fmt.Println("# 收件箱 ZSET（member = 13B msg id 指针）")
	for _, n := range []int{100, 1000, 5000, 20000} {
		key := fmt.Sprintf("inbox:cal:%d", n)
		c.Del(ctx, key)
		pipe := c.Pipeline()
		for i := 0; i < n; i++ {
			pipe.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: fmt.Sprintf(pointer, i)})
		}
		pipe.Exec(ctx)
		enc, _ := c.ObjectEncoding(ctx, key).Result()
		sz := memUsage(ctx, c, key)
		fmt.Printf("  n=%-6d bytes=%-9d per_entry=%6.1f encoding=%s\n", n, sz, float64(sz)/float64(n), enc)
	}

	fmt.Println("# 群时间线 ZSET（member = 256B 正文）")
	for _, n := range []int{100, 1000, 20000} {
		key := fmt.Sprintf("tl:cal:%d", n)
		c.Del(ctx, key)
		pipe := c.Pipeline()
		for i := 0; i < n; i++ {
			pipe.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: body + fmt.Sprintf("|m%012d", i)})
		}
		pipe.Exec(ctx)
		enc, _ := c.ObjectEncoding(ctx, key).Result()
		sz := memUsage(ctx, c, key)
		fmt.Printf("  n=%-6d bytes=%-9d per_entry=%6.1f encoding=%s\n", n, sz, float64(sz)/float64(n), enc)
	}

	fmt.Println("# 已读集合三种编码（N = 5000 成员，一条消息）")
	// bitmap
	c.Del(ctx, "bits:cal")
	for i := 0; i < 5000; i++ {
		c.SetBit(ctx, "bits:cal", int64(i), 1)
	}
	fmt.Printf("  bitmap:  bytes=%-6d per_member=%.2f encoding=%s\n", memUsage(ctx, c, "bits:cal"),
		float64(memUsage(ctx, c, "bits:cal"))/5000, mustEnc(ctx, c, "bits:cal"))
	// hash
	c.Del(ctx, "set:cal")
	pipe := c.Pipeline()
	for i := 0; i < 5000; i++ {
		pipe.HSet(ctx, "set:cal", fmt.Sprintf("u%07d", i), "1")
	}
	pipe.Exec(ctx)
	fmt.Printf("  hash:    bytes=%-6d per_member=%.2f encoding=%s\n", memUsage(ctx, c, "set:cal"),
		float64(memUsage(ctx, c, "set:cal"))/5000, mustEnc(ctx, c, "set:cal"))
	// counter
	c.Del(ctx, "cnt:cal")
	c.HIncrBy(ctx, "cnt:cal", "read", 5000)
	fmt.Printf("  counter: bytes=%-6d per_member=%.2f encoding=%s\n", memUsage(ctx, c, "cnt:cal"),
		float64(memUsage(ctx, c, "cnt:cal"))/5000, mustEnc(ctx, c, "cnt:cal"))

	// 单条消息的已读存储总成本：bitmap vs hash vs counter
	fmt.Println("# 结论读数（一条消息的已读存储）")
	fmt.Printf("  N=5000: bitmap=%.0fB, hash=%.0fB, counter=%.0fB\n",
		float64(memUsage(ctx, c, "bits:cal")), float64(memUsage(ctx, c, "set:cal")),
		float64(memUsage(ctx, c, "cnt:cal")))
	c.FlushAll(ctx)
}

func mustEnc(ctx context.Context, c *redis.Client, key string) string {
	e, err := c.ObjectEncoding(ctx, key).Result()
	if err != nil {
		return "?"
	}
	return e
}
