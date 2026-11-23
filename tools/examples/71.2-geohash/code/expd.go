package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 实测 D：索引维护写放大（真连本机 Redis 8.x @16394）
// 模型：index cell → set(member=uid)，p=6（约 1 km 格）
//   策略 A 原地更新（先删后插）：每个点 2 条命令（ZREM 旧格 + ZADD 新格）
//   策略 B 单 key 分数覆盖（Redis GEO 式）：每个点 1 条命令（ZADD 覆盖 score）
//   策略 C 双写影子索引 + 切换：离线全量重建 1M ZADD → RENAME 原子切换；窗口期双写

const redisAddr = "127.0.0.1:16394"

func cellKey(cell uint64) string {
	return "c:" + strconv.FormatUint(cell, 36)
}

func expD() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, PoolSize: 16})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Println("redis ping failed:", err)
		return
	}
	defer rdb.FlushDB(ctx)
	srv, _ := rdb.Info(ctx, "server").Result()
	ver := ""
	for _, line := range splitLines(srv) {
		if len(line) > 10 && line[:10] == "redis_vers" {
			ver = line
		}
	}
	fmt.Printf("# 实测 D：索引维护写放大（Redis @%s，%s）\n", redisAddr, ver)
	fmt.Printf("环境: macOS arm64 / Go %s / Redis 8.x @16394 / 2026-11-21\n", goVer())

	const n = 1000000
	const p = 6
	pts := GenPoints(n, 50000, 42)
	// 预计算：旧格、新格（随机走 ~2 km，确保多数点换格）
	oldCell := make([]uint64, n)
	newCell := make([]uint64, n)
	shifted := 0
	for i := 0; i < n; i++ {
		li, ai := quantCell(pts[i].Lat, pts[i].Lng, p)
		oldCell[i] = uint64(ai)<<32 | uint64(li)
		// 位移 0.02° 经度（约 1.7–2.2 km）
		li2, ai2 := quantCell(pts[i].Lat, pts[i].Lng+0.02, p)
		newCell[i] = uint64(ai2)<<32 | uint64(li2)
		if oldCell[i] != newCell[i] {
			shifted++
		}
	}
	fmt.Printf("点集 n=%d，p=%d 层级；一次全量更新中 %d 个点（%.1f%%）换格\n", n, p, shifted, float64(shifted)*100/float64(n))

	// ---- 阶段 0：建初始 per-cell 索引（1M ZADD）----
	t0 := time.Now()
	for start := 0; start < n; start += 1000 {
		end := min(start+1000, n)
		_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i := start; i < end; i++ {
				pipe.ZAdd(ctx, cellKey(oldCell[i]), redis.Z{Score: 0, Member: i})
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	dInit := time.Since(t0)
	fmt.Printf("\n[0] 建全量 per-cell 索引 1M ZADD: %v（%.0f ops/s，%.3f s/百万次）\n",
		dInit.Round(time.Millisecond), float64(n)/dInit.Seconds(), dInit.Seconds())
	fmt.Printf("    keys=%d, DBSIZE=%d\n", countKeys(ctx, rdb), rdb.DBSize(ctx).Val())

	// ---- 阶段 1：原地更新（先删后插）1M 点 ----
	t0 = time.Now()
	ops := 0
	for start := 0; start < n; start += 1000 {
		end := min(start+1000, n)
		_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i := start; i < end; i++ {
				pipe.ZRem(ctx, cellKey(oldCell[i]), i)
				pipe.ZAdd(ctx, cellKey(newCell[i]), redis.Z{Score: 0, Member: i})
				ops += 2
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	dInPlace := time.Since(t0)
	fmt.Printf("[A] 原地更新（ZREM旧格 + ZADD新格）1M 点 = %d 条命令: %v\n", ops, dInPlace.Round(time.Millisecond))
	fmt.Printf("    → %.0f ops/s；%.3f s/百万次更新；单点代价 2 ops\n",
		float64(ops)/dInPlace.Seconds(), dInPlace.Seconds())
	fmt.Printf("    → 10 万点 @5s 更新 = 2 万更新/s = 4 万 ops/s；100 万点 @5s = 20 万更新/s = 40 万 ops/s\n")

	// ---- 阶段 2：单 key 分数覆盖（Redis GEO 式，1 op/更新）----
	score := make([]float64, n)
	for i := 0; i < n; i++ {
		// 52 位 geohash 整数当 score（double 尾数 53 位可无损）
		li2, ai2 := quantCell(pts[i].Lat, pts[i].Lng+0.02, 7)
		score[i] = float64(encode52(pts[i].Lat, pts[i].Lng, li2, ai2))
	}
	t0 = time.Now()
	ops = 0
	for start := 0; start < n; start += 1000 {
		end := min(start+1000, n)
		_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i := start; i < end; i++ {
				pipe.ZAdd(ctx, "geo:live", redis.Z{Score: score[i], Member: i})
				ops++
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	dScore := time.Since(t0)
	fmt.Printf("[B] 单 key 分数覆盖（ZADD 覆盖 score）1M 点 = %d 条命令: %v\n", ops, dScore.Round(time.Millisecond))
	fmt.Printf("    → %.0f ops/s；%.3f s/百万次更新；单点代价 1 op\n",
		float64(ops)/dScore.Seconds(), dScore.Seconds())
	memLive, _ := rdb.MemoryUsage(ctx, "geo:live", 0).Result()
	fmt.Printf("    → MEMORY USAGE geo:live = %.2f MB（%.1f B/member）\n", float64(memLive)/1024/1024, float64(memLive)/float64(n))

	// ---- 阶段 3：影子全量重建 + RENAME 切换 ----
	t0 = time.Now()
	for start := 0; start < n; start += 1000 {
		end := min(start+1000, n)
		_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for i := start; i < end; i++ {
				pipe.ZAdd(ctx, "geo:shadow", redis.Z{Score: score[i], Member: i})
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	}
	dShadow := time.Since(t0)
	t1 := time.Now()
	rdb.Rename(ctx, "geo:shadow", "geo:live2")
	dRename := time.Since(t1)
	fmt.Printf("[C] 影子索引全量重建 1M ZADD: %v（%.0f ops/s，%.3f s/百万次）\n",
		dShadow.Round(time.Millisecond), float64(n)/dShadow.Seconds(), dShadow.Seconds())
	fmt.Printf("    原子切换 RENAME geo:shadow → geo:live2: %v（O(1)）\n", dRename)
	// 窗口期双写成本：重建期间的所有更新必须写两份
	fmt.Printf("    → 若 10 分钟窗口内 20 万更新/s：双写 = 2×120M = 240M ops；重建一次 %.3f s\n", dShadow.Seconds())

	// ---- 阶段 4：pipeline 深度对吞吐的影响（ZADD，20 万条）----
	fmt.Printf("\n[管道深度扫描] ZADD 20 万条，不同 pipeline 批大小：\n")
	fmt.Printf("%8s %12s %12s %14s\n", "batch", "wall", "ops/s", "每秒可写点")
	for _, bs := range []int{100, 500, 1000, 5000} {
		rdb.Del(ctx, "bench:z")
		total := 200000
		t0 = time.Now()
		for start := 0; start < total; start += bs {
			end := min(start+bs, total)
			_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
				for i := start; i < end; i++ {
					pipe.ZAdd(ctx, "bench:z", redis.Z{Score: float64(i), Member: i})
				}
				return nil
			})
			if err != nil {
				panic(err)
			}
		}
		d := time.Since(t0)
		fmt.Printf("%8d %12v %12.0f %14.0f\n", bs, d.Round(time.Millisecond), float64(total)/d.Seconds(), float64(total)/d.Seconds())
	}
	rdb.Del(ctx, "bench:z")

	// ---- 阶段 5：模型对账 ----
	fmt.Printf("\n[对账] 1M 点每 5 s 全量更新 = 20 万更新/s：\n")
	inPlaceCap := float64(ops) / dInPlace.Seconds() // 上一次 ops/s 是单 key 阶段，重算：
	_ = inPlaceCap
	inPlaceOps := float64(2*n) / dInPlace.Seconds()
	scoreOps := float64(n) / dScore.Seconds()
	fmt.Printf("  原地更新实测 %.0f ops/s → 20 万更新/s 需 40 万 ops/s → 需要 %.1f 个 Redis 实例\n", inPlaceOps, 400000/inPlaceOps)
	fmt.Printf("  单 key 覆盖实测 %.0f ops/s → 20 万更新/s 需 20 万 ops/s → 需要 %.1f 个 Redis 实例\n", scoreOps, 200000/scoreOps)
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
		} else if c != '\r' {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func countKeys(ctx context.Context, rdb *redis.Client) int64 {
	var cursor uint64
	var total int64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "c:*", 1000).Result()
		if err != nil {
			return total
		}
		total += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return total
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// quantCell 按 geohash p 层级量化成 (latIdx, lonIdx)，与 GeoHashIndex 同口径。
func quantCell(lat, lng float64, p int) (li, ai uint32) {
	bits := uint(5 * p)
	lonBits := (bits + 1) / 2
	latBits := bits / 2
	li = uint32(float64(uint64(1)<<lonBits) * (lng + 180.0) / 360.0)
	ai = uint32(float64(uint64(1)<<latBits) * (lat + 90.0) / 180.0)
	nlon := uint32(uint64(1) << lonBits)
	nlat := uint32(uint64(1) << latBits)
	if li >= nlon {
		li = nlon - 1
	}
	if ai >= nlat {
		ai = nlat - 1
	}
	return
}

// encode52 生成与 geohash 位交错等价的 52 位整数（只用于演示 score 存储）。
func encode52(lat, lng float64, li, ai uint32) uint64 {
	var v uint64
	for i := 0; i < 52; i++ {
		var bit uint64
		if i%2 == 0 { // 经度
			bit = uint64(li>>(25-uint(i/2))) & 1
		} else {
			bit = uint64(ai>>(25-uint(i/2))) & 1
		}
		v = v<<1 | bit
	}
	return v & 0xFFFFFFFFFFFFF
}
