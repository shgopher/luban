// e2_pipeline: 1000 个内容 ID 的「我是否点过赞」批量判定，同进程 A/B：
//   A) 逐个 GETBIT：1000 次独立 RTT
//   B) 一个 pipeline 里 1000 个 GETBIT：1 次 RTT
//   C) 计数快照的 MGET 对照：1000 次 GET vs 一次 MGET（渲染 Feed 时另一批必读数据）
// 三种方式的结果必须一致，否则判定逻辑就是错的。
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	nObjects = 1000
	userID   = 42
	rounds   = 5
)

var ctx = context.Background()

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16394", PoolSize: 8})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	// 每次运行前清空自己的实例，重建 1000 个对象的状态位图与计数 key
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}

	// 建 1000 个内容对象：每对象一个 bitmap，bit 偏移 = user_id；
	// 该用户对其中偶数下标的对象点过赞。另建 1000 个计数 key（点赞数）。
	want := make([]bool, nObjects)
	pipe := rdb.Pipeline()
	for i := 0; i < nObjects; i++ {
		key := fmt.Sprintf("like:obj:%d", i)
		if i%2 == 0 {
			want[i] = true
			pipe.SetBit(ctx, key, int64(userID), 1)
		} else {
			pipe.SetBit(ctx, key, 0, 1) // 只让别的用户点过，确保 key 存在
		}
		pipe.Set(ctx, fmt.Sprintf("cnt:obj:%d", i), i%97, 0)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		panic(err)
	}

	// ---------- A: 逐个 GETBIT（1000 次 RTT） ----------
	runSeq := func() (time.Duration, []bool) {
		t0 := time.Now()
		got := make([]bool, nObjects)
		for i := 0; i < nObjects; i++ {
			v, err := rdb.GetBit(ctx, fmt.Sprintf("like:obj:%d", i), int64(userID)).Result()
			if err != nil {
				panic(err)
			}
			got[i] = v == 1
		}
		return time.Since(t0), got
	}

	// ---------- B: 一个 pipeline 里 1000 个 GETBIT ----------
	runPipe := func() (time.Duration, []bool) {
		t0 := time.Now()
		p := rdb.Pipeline()
		cmds := make([]*redis.IntCmd, nObjects)
		for i := 0; i < nObjects; i++ {
			cmds[i] = p.GetBit(ctx, fmt.Sprintf("like:obj:%d", i), int64(userID))
		}
		if _, err := p.Exec(ctx); err != nil && err != redis.Nil {
			panic(err)
		}
		got := make([]bool, nObjects)
		for i, c := range cmds {
			v, _ := c.Result()
			got[i] = v == 1
		}
		return time.Since(t0), got
	}

	// ---------- C: 计数快照，逐个 GET vs 一次 MGET ----------
	runCountSeq := func() time.Duration {
		t0 := time.Now()
		for i := 0; i < nObjects; i++ {
			if _, err := rdb.Get(ctx, fmt.Sprintf("cnt:obj:%d", i)).Result(); err != nil && err != redis.Nil {
				panic(err)
			}
		}
		return time.Since(t0)
	}
	runCountMGet := func() time.Duration {
		t0 := time.Now()
		keys := make([]string, nObjects)
		for i := 0; i < nObjects; i++ {
			keys[i] = fmt.Sprintf("cnt:obj:%d", i)
		}
		if _, err := rdb.MGet(ctx, keys...).Result(); err != nil {
			panic(err)
		}
		return time.Since(t0)
	}

	seq := make([]time.Duration, 0, rounds)
	pp := make([]time.Duration, 0, rounds)
	cseq := make([]time.Duration, 0, rounds)
	cmget := make([]time.Duration, 0, rounds)
	var gotSeq, gotPipe []bool
	for r := 0; r < rounds; r++ {
		d, g := runSeq()
		if r == 0 {
			gotSeq = g
		}
		seq = append(seq, d)
		d, g = runPipe()
		if r == 0 {
			gotPipe = g
		}
		pp = append(pp, d)
		cseq = append(cseq, runCountSeq())
		cmget = append(cmget, runCountMGet())
	}

	// 一致性检查
	consistent := true
	for i := 0; i < nObjects; i++ {
		if gotSeq[i] != want[i] || gotPipe[i] != want[i] {
			consistent = false
		}
	}

	avg := func(ds []time.Duration) time.Duration {
		var s time.Duration
		for _, d := range ds {
			s += d
		}
		return s / time.Duration(len(ds))
	}
	best := func(ds []time.Duration) time.Duration {
		m := ds[0]
		for _, d := range ds {
			if d < m {
				m = d
			}
		}
		return m
	}

	fmt.Println("== 批量已赞判定：1000 个内容 ID，同一用户 ==")
	fmt.Printf("rounds                    : %d（每轮清空结果重跑，取平均/最小）\n", rounds)
	fmt.Printf("A 逐个 GETBIT (1000 RTT)  : avg %-12v best %v\n", avg(seq).Round(time.Microsecond), best(seq).Round(time.Microsecond))
	fmt.Printf("B pipeline GETBIT (1 RTT) : avg %-12v best %v\n", avg(pp).Round(time.Microsecond), best(pp).Round(time.Microsecond))
	fmt.Printf("A/B 加速倍数              : %.1f x\n", float64(avg(seq))/float64(avg(pp)))
	fmt.Printf("单次 RTT 折算（A/1000）   : %.1f µs\n", float64(avg(seq).Microseconds())/nObjects)
	fmt.Println()
	fmt.Println("== 计数快照：1000 个对象点赞数 ==")
	fmt.Printf("C1 逐个 GET (1000 RTT)    : avg %-12v best %v\n", avg(cseq).Round(time.Microsecond), best(cseq).Round(time.Microsecond))
	fmt.Printf("C2 MGET (1 RTT)           : avg %-12v best %v\n", avg(cmget).Round(time.Microsecond), best(cmget).Round(time.Microsecond))
	fmt.Printf("C1/C2 加速倍数            : %.1f x\n", float64(avg(cseq))/float64(avg(cmget)))
	fmt.Printf("结果一致 (1000/1000)      : %v\n", consistent)

	if err := rdb.FlushDB(ctx).Err(); err != nil {
		panic(err)
	}
}
