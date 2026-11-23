// e4_batch: 100 万次点赞事件，同进程 A/B：
//   A) 逐条写：每个事件同步 INCR 一次（1 次网络往返/事件）
//   B) 合批写：内存按 500 事件一个窗口聚合，窗口结束时 pipeline 一次 INCRBY
// 只测「写命令数」与「墙钟耗时」，不重算 44.22 的字节账。
// 20 个内容对象、窗口 500 → 每窗口把 20 个 key 的增量合一次。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	nEvents  = 1_000_000
	nObjects = 20
	window   = 500
)

var ctx = context.Background()

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16394", PoolSize: 8})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}

	// 事件流：20 个对象轮转（最热场景，合批收益最大；热点对象正是点赞的真实分布）
	eventObj := func(i int) int { return i % nObjects }

	// ---------- A: 逐条写 ----------
	var writesA int64
	t0 := time.Now()
	for i := 0; i < nEvents; i++ {
		k := fmt.Sprintf("cnt:obj:%d", eventObj(i))
		if err := rdb.Incr(ctx, k).Err(); err != nil {
			panic(err)
		}
		writesA++
	}
	durA := time.Since(t0)

	// 校验 A 的结果
	sumA := int64(0)
	for k := 0; k < nObjects; k++ {
		v, _ := rdb.Get(ctx, fmt.Sprintf("cnt:obj:%d", k)).Int64()
		sumA += v
	}

	// ---------- B: 合批写 ----------
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}
	var writesB, pipesB int64
	buf := make(map[string]int64, nObjects)
	inWindow := 0
	t0 = time.Now()
	flushWindow := func() {
		if len(buf) == 0 {
			return
		}
		p := rdb.Pipeline()
		for k, v := range buf {
			p.IncrBy(ctx, k, v)
			writesB++
		}
		if _, err := p.Exec(ctx); err != nil {
			panic(err)
		}
		pipesB++
		buf = make(map[string]int64, nObjects)
	}
	for i := 0; i < nEvents; i++ {
		k := fmt.Sprintf("cnt:obj:%d", eventObj(i))
		buf[k] += 1
		inWindow++
		if inWindow == window {
			flushWindow()
			inWindow = 0
		}
	}
	flushWindow()
	durB := time.Since(t0)

	sumB := int64(0)
	for k := 0; k < nObjects; k++ {
		v, _ := rdb.Get(ctx, fmt.Sprintf("cnt:obj:%d", k)).Int64()
		sumB += v
	}

	fmt.Println("== 100 万次点赞事件：逐条 INCR vs 500 事件窗口合批 INCRBY（同进程，真实 Redis 127.0.0.1:16394） ==")
	fmt.Printf("事件数                 : %d\n", nEvents)
	fmt.Printf("内容对象数             : %d  窗口大小: %d 事件\n", nObjects, window)
	fmt.Println()
	fmt.Printf("A 逐条写: 写命令 %-10d 耗时 %-12v 吞吐 %.0f 事件/s\n", writesA, durA.Round(time.Millisecond), float64(nEvents)/durA.Seconds())
	fmt.Printf("B 合批写: 写命令 %-10d 耗时 %-12v 吞吐 %.0f 事件/s (pipeline 执行 %d 次)\n", writesB, durB.Round(time.Millisecond), float64(nEvents)/durB.Seconds(), pipesB)
	fmt.Printf("写命令削减             : %.1f x\n", float64(writesA)/float64(writesB))
	fmt.Printf("耗时下降               : %.1f x\n", float64(durA)/float64(durB))
	fmt.Printf("结果一致 (总和 %d vs %d) : %v\n", sumA, sumB, sumA == sumB && sumA == nEvents)
	fmt.Printf("每窗口平均写 key 数    : %.1f / %d\n", float64(writesB)/float64(pipesB), nObjects)

	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}
}
