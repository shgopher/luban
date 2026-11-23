package main

// calib.go: 标定本机 Redis 单实例的管道化写入吞吐，用于确定实验 A 的消息条数 M。
// 运行: go run . -mode calib

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisAddr = "127.0.0.1:16385"

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: redisAddr, PoolSize: 16})
}

func calib() {
	ctx := context.Background()
	c := newClient()
	c.FlushAll(ctx)
	payload := strings.Repeat("x", 256)

	// ZADD 到不同 key（模拟写扩散：每个成员一个收件箱）
	for _, total := range []int{200000, 1000000} {
		c.FlushAll(ctx)
		const chunk = 2000
		start := time.Now()
		done := 0
		for done < total {
			pipe := c.Pipeline()
			for i := 0; i < chunk && done+i < total; i++ {
				k := done + i
				pipe.ZAdd(ctx, fmt.Sprintf("calib:in:%d", k%50000), redis.Z{Score: float64(k), Member: payload})
			}
			_, err := pipe.Exec(ctx)
			if err != nil {
				fmt.Println("ERR", err)
				return
			}
			done += chunk
		}
		el := time.Since(start)
		fmt.Printf("ZADD(diff-keys) total=%d elapsed=%v ops/s=%.0f\n", total, el, float64(total)/el.Seconds())
	}

	// PUBLISH 广播（模拟在线推送，无订阅者时也计入命令处理成本）
	for _, total := range []int{200000, 1000000} {
		start := time.Now()
		const chunk = 2000
		done := 0
		for done < total {
			pipe := c.Pipeline()
			for i := 0; i < chunk && done+i < total; i++ {
				pipe.Publish(ctx, fmt.Sprintf("calib:ch:%d", (done+i)%50000), payload)
			}
			_, err := pipe.Exec(ctx)
			if err != nil {
				fmt.Println("ERR", err)
				return
			}
			done += chunk
		}
		el := time.Since(start)
		fmt.Printf("PUBLISH(nosub) total=%d elapsed=%v ops/s=%.0f\n", total, el, float64(total)/el.Seconds())
	}

	// 单 key ZADD 深写（群时间线）
	for _, total := range []int{200000} {
		c.Del(ctx, "calib:timeline")
		start := time.Now()
		const chunk = 2000
		done := 0
		for done < total {
			pipe := c.Pipeline()
			for i := 0; i < chunk && done+i < total; i++ {
				pipe.ZAdd(ctx, "calib:timeline", redis.Z{Score: float64(done + i), Member: payload})
			}
			_, err := pipe.Exec(ctx)
			if err != nil {
				fmt.Println("ERR", err)
				return
			}
			done += chunk
		}
		el := time.Since(start)
		fmt.Printf("ZADD(single-key) total=%d elapsed=%v ops/s=%.0f\n", total, el, float64(total)/el.Seconds())
	}

	c.FlushAll(ctx)
	info, _ := c.Info(ctx, "memory").Result()
	for _, ln := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(ln, "used_memory_human:") || strings.HasPrefix(ln, "used_memory:") {
			fmt.Println(ln)
		}
	}
	_ = os.Stdout
}
