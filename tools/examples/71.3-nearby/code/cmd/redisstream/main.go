// 实测 A：Redis GEO 规模实验（1 万 / 10 万 / 100 万 member）
// - GEOSEARCH BYRADIUS 5km ASC COUNT 50 各 500 次：P50 / P99
// - INFO memory 折算单 member 字节；MEMORY USAGE 单 key
// - 单 key 全国 vs 按 20 城市拆 key 的延迟与内存差异
package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type city struct {
	name string
	lat  float64
	lng  float64
}

var cities = []city{
	{"北京", 39.9042, 116.4074}, {"上海", 31.2304, 121.4737}, {"广州", 23.1291, 113.2644},
	{"深圳", 22.5431, 114.0579}, {"成都", 30.5728, 104.0668}, {"杭州", 30.2741, 120.1551},
	{"武汉", 30.5928, 114.3055}, {"西安", 34.3416, 108.9398}, {"南京", 32.0603, 118.7969},
	{"重庆", 29.5630, 106.5516}, {"天津", 39.0842, 117.2010}, {"苏州", 31.2989, 120.5853},
	{"郑州", 34.7466, 113.6254}, {"长沙", 28.2282, 112.9388}, {"青岛", 36.0671, 120.3826},
	{"沈阳", 41.8057, 123.4315}, {"大连", 38.9140, 121.6147}, {"厦门", 24.4798, 118.0894},
	{"昆明", 25.0389, 102.7183}, {"哈尔滨", 45.8038, 126.5349},
}

type pt struct {
	lng, lat float64
	city     int
}

func offset(lat, lng, north, east float64) (float64, float64) {
	dLat := north / 111320.0
	dLng := east / (111320.0 * math.Cos(lat*math.Pi/180))
	return lat + dLat, lng + dLng
}

func diskPoint(rng *rand.Rand, lat, lng, radiusM float64) (float64, float64) {
	r := radiusM * math.Sqrt(rng.Float64())
	theta := rng.Float64() * 2 * math.Pi
	return offset(lat, lng, r*math.Sin(theta), r*math.Cos(theta))
}

func genPoints(n int) []pt {
	rng := rand.New(rand.NewSource(20261121))
	out := make([]pt, n)
	for i := 0; i < n; i++ {
		c := i % len(cities)
		lat, lng := diskPoint(rng, cities[c].lat, cities[c].lng, 20000)
		out[i] = pt{lng, lat, c}
	}
	return out
}

func usedMemory(ctx context.Context, rdb *redis.Client) int64 {
	info, err := rdb.Info(ctx, "memory").Result()
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "used_memory:") {
			v, _ := strconv.ParseInt(strings.TrimPrefix(line, "used_memory:"), 10, 64)
			return v
		}
	}
	return 0
}

func geoAdd(ctx context.Context, rdb *redis.Client, key string, pts []pt) time.Duration {
	start := time.Now()
	const batch = 1000
	for i := 0; i < len(pts); i += batch {
		j := i + batch
		if j > len(pts) {
			j = len(pts)
		}
		pipe := rdb.Pipeline()
		for k := i; k < j; k++ {
			pipe.GeoAdd(ctx, key, &redis.GeoLocation{
				Name:      fmt.Sprintf("u%09d", k),
				Longitude: pts[k].lng,
				Latitude:  pts[k].lat,
			})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			panic(err)
		}
	}
	return time.Since(start)
}

type result struct {
	mode        string
	members     int
	keys        int
	memBytes    int64 // INFO used_memory 增量
	keyBytes    int64 // MEMORY USAGE 合计
	p50, p99    float64
	avgReturned float64
	minR, maxR  int
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func benchmark(ctx context.Context, rdb *redis.Client, keyOf func(cityIdx int) string, rng *rand.Rand) (float64, float64) {
	const rounds = 500
	us := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		c := rng.Intn(len(cities))
		qlat, qlng := diskPoint(rng, cities[c].lat, cities[c].lng, 3000)
		key := keyOf(c)
		start := time.Now()
		_, err := rdb.GeoSearch(ctx, key, &redis.GeoSearchQuery{
			Longitude: qlng, Latitude: qlat,
			Radius: 5, RadiusUnit: "km", Sort: "ASC", Count: 50,
		}).Result()
		if err != nil {
			panic(err)
		}
		d := float64(time.Since(start).Microseconds())
		if i >= 20 { // 前 20 次预热不计入
			us = append(us, d)
		}
	}
	sorted := append([]float64(nil), us...)
	sort.Float64s(sorted)
	return percentile(sorted, 50), percentile(sorted, 99)
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16393", PoolSize: 8})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	fmt.Println("# 实测 A：Redis GEO 规模实验")
	fmt.Println("# 环境：macOS arm64 / Apple M1 / Go 1.27.1 / Redis 8.10.2 @127.0.0.1:16393 / 2026-11-21")
	fmt.Println("# 数据：20 个城市中心各生成半径 20 km 圆盘均匀点；查询点=城市中心 3 km 内随机")
	fmt.Println("# 查询：GEOSEARCH key FROMLONLAT <lng> <lat> BYRADIUS 5 km ASC COUNT 50；每个规模 500 次（前 20 次预热不计）")
	fmt.Println()

	var rows []result
	for _, n := range []int{10000, 100000, 1000000} {
		rng := rand.New(rand.NewSource(int64(n)))
		pts := genPoints(n)

		// 模式 1：单 key 全国
		_ = rdb.FlushAll(ctx).Err()
		base := usedMemory(ctx, rdb)
		insertDur := geoAdd(ctx, rdb, "geo:all", pts)
		after := usedMemory(ctx, rdb)
		keyBytes, _ := rdb.MemoryUsage(ctx, "geo:all", 0).Result()
		p50, p99 := benchmark(ctx, rdb, func(int) string { return "geo:all" }, rng)
		// 额外统计返回条数
		avg, minR, maxR := returnedStats(ctx, rdb, func(int) string { return "geo:all" }, rand.New(rand.NewSource(7)))
		rows = append(rows, result{"单 key 全国", n, 1, after - base, keyBytes, p50, p99, avg, minR, maxR})
		fmt.Printf("## 规模 %d：单 key 全国\n", n)
		fmt.Printf("写入耗时 %v（%s 条/秒）；INFO used_memory %d -> %d（增量 %.2f MiB，%.1f B/member）；MEMORY USAGE=%.2f MiB；返回条数 min/avg/max=%d/%.1f/%d\n",
			insertDur.Round(time.Millisecond), fmtInt(int64(float64(n)/insertDur.Seconds())),
			base, after, float64(after-base)/(1024*1024), float64(after-base)/float64(n),
			float64(keyBytes)/(1024*1024), minR, avg, maxR)

		// 模式 2：按 20 城市拆 key
		_ = rdb.FlushAll(ctx).Err()
		base = usedMemory(ctx, rdb)
		start := time.Now()
		var totalKeyBytes int64
		for c := 0; c < len(cities); c++ {
			sub := make([]pt, 0, n/len(cities)+1)
			for i := c; i < n; i += len(cities) {
				sub = append(sub, pts[i])
			}
			key := fmt.Sprintf("geo:city:%02d", c)
			_ = geoAdd(ctx, rdb, key, sub)
			kb, _ := rdb.MemoryUsage(ctx, key, 0).Result()
			totalKeyBytes += kb
		}
		insertDur = time.Since(start)
		after = usedMemory(ctx, rdb)
		p50, p99 = benchmark(ctx, rdb, func(c int) string { return fmt.Sprintf("geo:city:%02d", c) }, rng)
		avg, minR, maxR = returnedStats(ctx, rdb, func(c int) string { return fmt.Sprintf("geo:city:%02d", c) }, rand.New(rand.NewSource(7)))
		rows = append(rows, result{"20 城市拆 key", n, 20, after - base, totalKeyBytes, p50, p99, avg, minR, maxR})
		fmt.Printf("## 规模 %d：20 城市拆 key\n", n)
		fmt.Printf("写入耗时 %v；INFO used_memory 增量 %.2f MiB（%.1f B/member）；MEMORY USAGE 合计 %.2f MiB；返回条数 min/avg/max=%d/%.1f/%d\n\n",
			insertDur.Round(time.Millisecond), float64(after-base)/(1024*1024), float64(after-base)/float64(n),
			float64(totalKeyBytes)/(1024*1024), minR, avg, maxR)
	}

	fmt.Println("## 汇总表")
	fmt.Println("| 模式 | member | key 数 | INFO 内存 | B/member | MEMORY USAGE 合计 | 返回条数均值 | P50(µs) | P99(µs) |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	for _, r := range rows {
		fmt.Printf("| %s | %d | %d | %.2f MiB | %.1f | %.2f MiB | %.1f | %.1f | %.1f |\n",
			r.mode, r.members, r.keys, float64(r.memBytes)/(1024*1024), float64(r.memBytes)/float64(r.members),
			float64(r.keyBytes)/(1024*1024), r.avgReturned, r.p50, r.p99)
	}

	// 单 key 体积 vs 1MB 红线
	fmt.Println("\n## 单 key 体积与「1 MB 红线」")
	for _, r := range rows {
		if r.keys == 1 {
			fmt.Printf("- %d member 单 key：MEMORY USAGE %.2f MiB = 红线的 %.0f 倍\n",
				r.members, float64(r.keyBytes)/(1024*1024), float64(r.keyBytes)/(1024*1024))
		}
	}
	_ = rdb.FlushAll(ctx).Err()
}

func returnedStats(ctx context.Context, rdb *redis.Client, keyOf func(int) string, rng *rand.Rand) (float64, int, int) {
	minR, maxR, sum, cnt := 1<<30, 0, 0, 0
	for i := 0; i < 100; i++ {
		c := rng.Intn(len(cities))
		qlat, qlng := diskPoint(rng, cities[c].lat, cities[c].lng, 3000)
		res, err := rdb.GeoSearch(ctx, keyOf(c), &redis.GeoSearchQuery{
			Longitude: qlng, Latitude: qlat, Radius: 5, RadiusUnit: "km", Sort: "ASC", Count: 50,
		}).Result()
		if err != nil {
			panic(err)
		}
		if len(res) < minR {
			minR = len(res)
		}
		if len(res) > maxR {
			maxR = len(res)
		}
		sum += len(res)
		cnt++
	}
	return float64(sum) / float64(cnt), minR, maxR
}

func fmtInt(v int64) string {
	s := strconv.FormatInt(v, 10)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
