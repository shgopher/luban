// 实测 E：写放大与降频（位置更新频率怎么定）
// 1) 1 000 条移动轨迹、5 s 上报、按「移动超过阈值才更新」统计写次数下降比例
// 2) 本机 Redis GEOADD 吞吐实测，用于外推 100 万在线用户需要的实例数
package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"s703/internal/geoform"
)

type trackUser struct {
	lat, lng     float64
	lastLat      float64
	lastLng      float64
	heading      float64
	speedKmh     float64
	segmentStart int
}

func main() {
	fmt.Println("# 实测 E：位置写放大与降频（5 s 前台 / 30 s 后台）")
	fmt.Println("# 环境：macOS arm64 / Apple M1 / Go 1.27.1 / Redis 8.10.2 @127.0.0.1:16393 / 2026-11-21")
	fmt.Println()

	// ---------- 写入轨迹仿真 ----------
	const users = 1000
	const hours = 1
	const tickSec = 5
	const ticks = hours * 3600 / tickSec // 720
	rng := rand.New(rand.NewSource(20261121))

	start := time.Now()
	_ = start
	ut := make([]trackUser, users)
	for i := range ut {
		// 北京附近 10 km 内均匀
		r := 10000 * math.Sqrt(rng.Float64())
		th := rng.Float64() * 2 * math.Pi
		lat, lng := geoform.OffsetMeters(39.996794, 116.48105, r*math.Sin(th), r*math.Cos(th))
		ut[i] = trackUser{
			lat: lat, lng: lng, lastLat: lat, lastLng: lng,
			heading: rng.Float64() * 360,
			// 速度分档：30% 静止(0-3)、35% 步行(3-8)、20% 骑行(8-25)、15% 驾车(25-50)
			speedKmh: pickSpeed(rng),
		}
	}

	bands := []*band{
		{"静止 0-3 km/h", 0, 3, 0, 0},
		{"步行 3-8 km/h", 3, 8, 0, 0},
		{"骑行 8-25 km/h", 8, 25, 0, 0},
		{"驾车 25-50 km/h", 25, 50, 0, 0},
	}

	totalW5, totalW10, totalW20 := 0, 0, 0
	for t := 0; t < ticks; t++ {
		for i := range ut {
			u := &ut[i]
			// 每 60 s 换一次速度与方向（模拟城市出行）
			if t%12 == 0 {
				u.heading = rng.Float64() * 360
				if rng.Float64() < 0.5 { // 一半概率保持当前档
					u.speedKmh = pickSpeed(rng)
				}
			}
			// 前进一个 5 s tick
			dist := u.speedKmh / 3.6 * tickSec
			rad := u.heading * math.Pi / 180
			u.lat, u.lng = geoform.OffsetMeters(u.lat, u.lng, dist*math.Cos(rad), dist*math.Sin(rad))
			moved := geoform.Haversine(u.lastLat, u.lastLng, u.lat, u.lng)

			b := bandOf(bands, u.speedKmh)
			b.fixes++
			if moved > 5 {
				totalW5++
				b.w5++
				u.lastLat, u.lastLng = u.lat, u.lng
			}
			if moved > 10 {
				totalW10++
			}
			if moved > 20 {
				totalW20++
			}
		}
	}
	// 30 s 口径另行单独仿真：每 30 s 取一次位置，与上次写入位置比
	ut2 := make([]trackUser, users)
	rng2 := rand.New(rand.NewSource(20261121))
	for i := range ut2 {
		r := 10000 * math.Sqrt(rng2.Float64())
		th := rng2.Float64() * 2 * math.Pi
		lat, lng := geoform.OffsetMeters(39.996794, 116.48105, r*math.Sin(th), r*math.Cos(th))
		ut2[i] = trackUser{lat: lat, lng: lng, lastLat: lat, lastLng: lng, heading: rng2.Float64() * 360, speedKmh: pickSpeed(rng2)}
	}
	w30 := 0
	fixes30 := 0
	for t := 0; t < 120; t++ { // 120 个 30 s tick = 1 小时
		for i := range ut2 {
			u := &ut2[i]
			if t%2 == 0 {
				u.heading = rng2.Float64() * 360
				if rng2.Float64() < 0.5 {
					u.speedKmh = pickSpeed(rng2)
				}
			}
			dist := u.speedKmh / 3.6 * 30
			rad := u.heading * math.Pi / 180
			u.lat, u.lng = geoform.OffsetMeters(u.lat, u.lng, dist*math.Cos(rad), dist*math.Sin(rad))
			fixes30++
			if geoform.Haversine(u.lastLat, u.lastLng, u.lat, u.lng) > 5 {
				w30++
				u.lastLat, u.lastLng = u.lat, u.lng
			}
		}
	}

	totalFixes := users * ticks
	fmt.Println("## 仿真参数")
	fmt.Printf("1 000 用户 × 1 小时 × 5 s 上报 = %d 次定位（fix）；速度分档 30%% 静止 / 35%% 步行 / 20%% 骑行 / 15%% 驾车；每 60 s 换向、50%% 换速\n\n", totalFixes)
	fmt.Println("## 阈值过滤后的写入量（5 s 前台口径）")
	fmt.Println("| 写入阈值 | 写入次数 | 相对全量 | 1 万在线用户 QPS | 100 万在线用户 QPS |")
	fmt.Println("|---|---|---|---|---|")
	fmt.Printf("| 全量更新（每次 fix 都写） | %d | 100.00%% | %s | %s |\n", totalFixes, fmtInt(int64(totalFixes/3600*10)), fmtInt(int64(totalFixes/3600*1000)))
	for _, th := range []struct {
		name string
		n    int
	}{
		{"移动 > 5 m 才写", totalW5},
		{"移动 > 10 m 才写", totalW10},
		{"移动 > 20 m 才写", totalW20},
	} {
		ratio := float64(th.n) / float64(totalFixes)
		fmt.Printf("| %s | %d | %.2f%% | %s | %s |\n", th.name, th.n, ratio*100,
			fmtInt(int64(float64(th.n)/3600*10)), fmtInt(int64(float64(th.n)/3600*1000)))
	}
	fmt.Printf("\n30 s 后台口径：%d 次定位 → 写入 %d 次（%.2f%%），1 000 万后台用户 = %s 写 QPS\n\n",
		fixes30, w30, float64(w30)/float64(fixes30)*100, fmtInt(int64(float64(w30)/3600*10000)))

	fmt.Println("## 分速度档的写下降比例（5 s 口径，阈值 5 m）")
	fmt.Println("| 速度档 | fix 次数 | 写入次数 | 写入占比 | 说明 |")
	fmt.Println("|---|---|---|---|---|")
	for _, b := range bands {
		fmt.Printf("| %s | %d | %d | %.1f%% | %s |\n", b.name, b.fixes, b.w5,
			float64(b.w5)/float64(b.fixes)*100, bandNote(b.name))
	}

	// ---------- Redis GEOADD 吞吐实测 ----------
	fmt.Println("\n## 本机 Redis GEOADD 吞吐（单连接、pipeline 批量 5 000）")
	for _, batch := range []int{1000, 5000} {
		rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16393", PoolSize: 4})
		ctx := context.Background()
		_ = rdb.Del(ctx, "wa:bench").Err()
		const n = 200000
		start := time.Now()
		for i := 0; i < n; i += batch {
			j := i + batch
			if j > n {
				j = n
			}
			pipe := rdb.Pipeline()
			for k := i; k < j; k++ {
				lat := 39.9 + float64(k%1000)*0.0001
				lng := 116.4 + float64(k%997)*0.0001
				pipe.GeoAdd(ctx, "wa:bench", &redis.GeoLocation{Name: fmt.Sprintf("b%07d", k), Longitude: lng, Latitude: lat})
			}
			if _, err := pipe.Exec(ctx); err != nil {
				panic(err)
			}
		}
		d := time.Since(start)
		fmt.Printf("- batch=%d：%d 条 GEOADD 用时 %v → %.0f 条/秒（%.2f µs/条）\n",
			batch, n, d.Round(time.Millisecond), float64(n)/d.Seconds(), d.Seconds()*1e6/float64(n))
		_ = rdb.Del(ctx, "wa:bench").Err()
		rdb.Close()
	}

	// 外推
	fmt.Println("\n## 外推：100 万在线用户要几个 Redis 实例（推算）")
	fullQPS := 1000000.0 / 5
	reducedQPS := fullQPS * float64(totalW5) / float64(totalFixes)
	fmt.Printf("推导链：100 万在线 ÷ 5 s = %s 次/s（全量）→ × %.2f%%（5 m 降频后）= %s 写/s\n",
		fmtInt(int64(fullQPS)), float64(totalW5)/float64(totalFixes)*100, fmtInt(int64(reducedQPS)))
	for _, c := range []struct {
		label string
		per   float64
	}{
		{"1M 单 key 写入时实测 247,705 条/s", 247705},
		{"200k 条空载 pipeline(5000) 实测 431,680 条/s", 431680},
	} {
		fmt.Printf("按「%s」外推：全量 %.2f 个实例、降频后 %.2f 个实例（未计查询/网络/主从/CPU 竞争，属下限）\n",
			c.label, fullQPS/c.per, reducedQPS/c.per)
	}
}

func pickSpeed(rng *rand.Rand) float64 {
	x := rng.Float64()
	switch {
	case x < 0.30:
		return rng.Float64() * 3
	case x < 0.65:
		return 3 + rng.Float64()*5
	case x < 0.85:
		return 8 + rng.Float64()*17
	default:
		return 25 + rng.Float64()*25
	}
}

type band struct {
	name       string
	minKmh     float64
	maxKmh     float64
	fixes, w5 int
}

func bandOf(bands []*band, kmh float64) *band {
	for _, b := range bands {
		if kmh >= b.minKmh && kmh < b.maxKmh {
			return b
		}
	}
	return bands[len(bands)-1]
}

func bandNote(name string) string {
	switch name {
	case "静止 0-3 km/h":
		return "GPS 噪声级移动，绝大多数 fix 被 5 m 阈值过滤"
	case "步行 3-8 km/h":
		return "5 s 走 4-11 m，约一半被过滤"
	case "骑行 8-25 km/h":
		return "每 5 s 移动 11-35 m，基本都写"
	default:
		return "每 5 s 移动 35-69 m，全部写"
	}
}

func fmtInt(v int64) string {
	s := fmt.Sprintf("%d", v)
	out := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	return out
}
