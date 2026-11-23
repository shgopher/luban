package main

// 68.4 名单匹配的假阳性与容量账：10 万名单 + 1 万条不在名单里的查询，
// 对比精确 map / 分片 map / 布隆过滤器的假阳性、常驻内存与单次查询延迟。
// 布隆参数公式与实现细节见 41.8。

import (
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"time"
)

const (
	listN  = 100_000 // 名单条数
	queryN = 10_000  // 查询条数（全部不在名单里）
	shardN = 16      // 分片数
	bfP    = 0.01    // 布隆目标假阳性
	qps    = 5_000.0 // 峰值查询 QPS，用来把假阳性换算成回源/误伤
)

type bloom struct {
	bits []uint64
	m, k uint64
}

// newBloom 按 n、p 反推 m 与 k：m = -n·ln(p)/(ln2)²，k = (m/n)·ln2
func newBloom(n int, p float64) *bloom {
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	k := uint64(math.Round(float64(m) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	return &bloom{bits: make([]uint64, (m+63)/64), m: m, k: k}
}

// hash64 双哈希：h1 定起点、h2 定步长，一个字符串派生 k 个位置
func hash64(s string) (uint64, uint64) {
	h := fnv.New64a()
	h.Write([]byte(s))
	h1 := h.Sum64()
	h2 := (h1 >> 33) | (h1 << 31)
	h2 *= 0x9e3779b97f4a7c15
	h2 ^= h2 >> 29
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

func (b *bloom) add(s string) {
	h1, h2 := hash64(s)
	for i := uint64(0); i < b.k; i++ {
		pos := (h1 + i*h2) % b.m
		b.bits[pos/64] |= 1 << (pos % 64)
	}
}

func (b *bloom) has(s string) bool {
	h1, h2 := hash64(s)
	for i := uint64(0); i < b.k; i++ {
		pos := (h1 + i*h2) % b.m
		if b.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false // 一位为 0：一定不在
		}
	}
	return true // 全为 1：可能在，需回源确认
}

type exact struct{ m map[string]struct{} }

func (x *exact) has(s string) bool { _, ok := x.m[s]; return ok }

type sharded struct{ sh [shardN]map[string]struct{} }

func shardOf(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32() % shardN)
}

func (x *sharded) has(s string) bool {
	_, ok := x.sh[shardOf(s)][s]
	return ok
}

var keep any // 持有被测结构，避免被 GC 回收

// measure 先 GC 读基线，再建结构（连 key 字符串一起建），再 GC 读差值
func measure(build func() any) (int64, any) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	v := build()
	keep = v
	runtime.GC()
	runtime.ReadMemStats(&after)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc), v
}

func avgNS(has func(string) bool, keys []string) float64 {
	const rounds = 20
	for _, k := range keys { // 预热
		has(k)
	}
	start := time.Now()
	for r := 0; r < rounds; r++ {
		for _, k := range keys {
			has(k)
		}
	}
	return float64(time.Since(start).Nanoseconds()) / float64(rounds*len(keys))
}

func main() {
	queries := make([]string, queryN)
	for i := range queries {
		queries[i] = fmt.Sprintf("query:%d", i)
	}
	buildExact := func() any {
		x := &exact{m: make(map[string]struct{}, listN)}
		for i := 0; i < listN; i++ {
			x.m[fmt.Sprintf("user:%d", i)] = struct{}{}
		}
		return x
	}
	buildShard := func() any {
		x := &sharded{}
		for i := range x.sh {
			x.sh[i] = make(map[string]struct{}, listN/shardN)
		}
		for i := 0; i < listN; i++ {
			k := fmt.Sprintf("user:%d", i)
			x.sh[shardOf(k)][k] = struct{}{}
		}
		return x
	}
	buildBloom := func() any {
		b := newBloom(listN, bfP)
		for i := 0; i < listN; i++ {
			b.add(fmt.Sprintf("user:%d", i))
		}
		return b
	}

	// 布隆内存 = 位数组字节数，直接按 m 算，不受分配器取整影响；
	// 顺带量一次全量构建耗时：名单热更新要不要全量重建，看的就是这个数。
	t0 := time.Now()
	b := buildBloom().(*bloom)
	bloomBuild := time.Since(t0)
	t1 := time.Now()
	_ = buildExact()
	mapBuild := time.Since(t1)
	bloomBytes := int64(len(b.bits) * 8)
	memMap, v1 := measure(buildExact)
	memShard, v2 := measure(buildShard)
	ex := v1.(*exact)
	sh := v2.(*sharded)

	countFP := func(has func(string) bool) int {
		n := 0
		for _, k := range queries {
			if has(k) {
				n++
			}
		}
		return n
	}
	fpMap, fpShard, fpBloom := countFP(ex.has), countFP(sh.has), countFP(b.has)
	latMap := avgNS(ex.has, queries) / 1000
	latShard := avgNS(sh.has, queries) / 1000
	latBloom := avgNS(b.has, queries) / 1000

	fmt.Printf("名单=%d 条，查询=%d 条（全部不在名单中），分片数=%d，布隆目标 p=%.0f%%\n\n",
		listN, queryN, shardN, bfP*100)
	fmt.Printf("%-14s %-8s %-11s %-13s %s\n", "结构", "假阳性", "内存(KB)", "单次查询(µs)", "备注")
	fmt.Println("-------------------------------------------------------------------")
	fmt.Printf("%-14s %-8d %-11.1f %-13.3f %s\n", "精确 map", fpMap, float64(memMap)/1024, latMap, "零假阳性")
	fmt.Printf("%-14s %-8d %-11.1f %-13.3f %s\n", "分片 map(16)", fpShard, float64(memShard)/1024, latShard, "零假阳性")
	fmt.Printf("%-14s %-8d %-11.1f %-13.3f %s\n", "布隆 p=1%", fpBloom, float64(bloomBytes)/1024, latBloom, "需回源确认")
	fmt.Printf("\n布隆参数：m=%d bit（%.1f bit/key），k=%d，位数组 %.1f KB\n",
		b.m, float64(b.m)/float64(listN), b.k, float64(bloomBytes)/1024)

	perMap := float64(memMap) / listN
	perBloom := float64(bloomBytes) / listN
	fmt.Printf("\n每条约 %.0f B（map，含 key 字符串）/ %.2f B（布隆）\n", perMap, perBloom)
	fmt.Printf("外推：100 万条 → map %.0f MB、布隆 %.1f MB（同参数线性放大）\n",
		perMap*1e6/1024/1024, perBloom*1e6/1024/1024)
	fmt.Printf("全量构建耗时（10 万条，单线程）：map %.1f ms，布隆 %.1f ms\n",
		float64(mapBuild.Microseconds())/1000, float64(bloomBuild.Microseconds())/1000)

	fpRate := float64(fpBloom) / queryN
	fmt.Printf("\n假阳性率 %.2f%%：QPS=%.0f 时每秒回源 %.1f 次；\n", fpRate*100, qps, qps*fpRate)
	fmt.Printf("若把布隆命中直接当拒绝（错误用法），每天误伤 %.0f 人\n", qps*fpRate*86400)
}
