// 实测 B：Redis ZSET 真实游标翻页（ZREVRANGEBYSCORE + 同分 id 兜底）
//
// 两个数据集（同一个 3000 条时间线，只差 member 编码）：
//
//	inbox:u1001     member = feed:%012d（补零，生产格式；字符串序与数值序一致）
//	inbox:u1001raw  member = feed:%d   （未补零，对照组）
//	每 3 条共用一个 score（模拟同毫秒批量入库）
//
// 运行：go run ./cmd/zsetcursor
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	addr     = "127.0.0.1:16392"
	pageSize = 20
	keyPad   = "inbox:u1001"
	keyRaw   = "inbox:u1001raw"
)

var (
	ctx = context.Background()
	rdb *redis.Client
)

type walkResult struct {
	pages, returned, missing, dup int
}

func main() {
	rdb = redis.NewClient(&redis.Options{Addr: addr, PoolSize: 8})
	defer rdb.Close()
	pong, err := rdb.Ping(ctx).Result()
	must(err)
	ver, _ := rdb.Do(ctx, "INFO", "server").Text()
	fmt.Printf("Redis %s @ %s, PING=%s\n", infoField(ver, "redis_version"), addr, pong)

	truthPad := rdb.ZRevRange(ctx, keyPad, 0, -1).Val()
	truthRaw := rdb.ZRevRange(ctx, keyRaw, 0, -1).Val()
	fmt.Printf("数据集：%s ZCARD=%d（member=feed:%%012d），%s ZCARD=%d（member=feed:%%d），pageSize=%d\n\n",
		keyPad, len(truthPad), keyRaw, len(truthRaw), pageSize)

	// 1. 同分成员的排序真相：补零组与未补零组的对照
	fmt.Println("== 1. 同分成员怎么排：Redis 按 member 字典序降序，不是按数值 ==")
	fmt.Printf("   %s 第 8~10 名（补零）：%v\n", keyPad, rdb.ZRevRange(ctx, keyPad, 7, 9).Val())
	rawGroup := rdb.ZRevRangeByScore(ctx, keyRaw, &redis.ZRangeBy{
		Max: strconv.FormatInt(scoreOf(9), 10), Min: strconv.FormatInt(scoreOf(9), 10)}).Val()
	padGroup := rdb.ZRevRangeByScore(ctx, keyPad, &redis.ZRangeBy{
		Max: strconv.FormatInt(scoreOf(9), 10), Min: strconv.FormatInt(scoreOf(9), 10)}).Val()
	fmt.Printf("   位数边界组（数值 id 9/10/11 同分）未补零：%v —— 数值降序应为 [feed:11 feed:10 feed:9]\n", rawGroup)
	fmt.Printf("   同组补零：%v —— 字典序与数值序一致\n\n", padGroup)

	// 2. 策略 × member 格式对账
	fmt.Println("== 2. 三种策略 × 两种 member 格式全量对账（3000 条，与 ZREVRANGE 全序比）==")
	fmt.Printf("%-40s %14s %14s\n", "策略", "补零(生产)", "未补零(对照)")
	rep := func(name string, pad, raw walkResult) {
		fmt.Printf("%-40s %10d漏/%d重 %10d漏/%d重\n", name, pad.missing, pad.dup, raw.missing, raw.dup)
	}
	rep("A 朴素：只按 score 严格小于翻（整组丢）", walk(truthPad, keyPad, stratNaive), walk(truthRaw, keyRaw, stratNaive))
	rep("B 同分兜底 + 数值 id 比较", walk(truthPad, keyPad, stratNumeric), walk(truthRaw, keyRaw, stratNumeric))
	rep("C 同分兜底 + member 字典序比较", walk(truthPad, keyPad, stratLex), walk(truthRaw, keyRaw, stratLex))
	fmt.Println()

	// 3. 定点复现：游标停在 feed:9 时，下一页本应从 11 开始
	fmt.Println("== 3. 定点复现（同一个 score 组，数值 9/10/11）==")
	rawFirst := rdb.ZRevRangeByScore(ctx, keyRaw, &redis.ZRangeBy{
		Max: strconv.FormatInt(scoreOf(9), 10), Min: strconv.FormatInt(scoreOf(9), 10), Count: 1}).Val()[0]
	padFirst := rdb.ZRevRangeByScore(ctx, keyPad, &redis.ZRangeBy{
		Max: strconv.FormatInt(scoreOf(9), 10), Min: strconv.FormatInt(scoreOf(9), 10), Count: 1}).Val()[0]
	fmt.Printf("   未补零：游标停在 %s（该组存储序第一条），下一页本应返回 %v\n", rawFirst,
		sameGroupTail(ctx, keyRaw, scoreOf(9), func(m string) bool { return m < rawFirst }))
	fmt.Printf("   未补零：数值比较补同组返回 %d 条 → 这 2 条被静默丢掉（全量累计 2 漏 1 重）\n",
		len(sameGroupTail(ctx, keyRaw, scoreOf(9), func(m string) bool { return numID(m) < numID(rawFirst) })))
	fmt.Printf("   补零：游标停在 %s，数值比较补同组返回 %v —— 与字典序一致，0 漏\n", padFirst,
		sameGroupTail(ctx, keyPad, scoreOf(9), func(m string) bool { return numID(m) < numID(padFirst) }))
	fmt.Println()

	// 4. 真实游标翻页耗时 A/B（生产格式，200 轮 × 150 页）
	fmt.Println("== 4. 单页耗时 A/B：3 次 RTT/页 vs 2 次 RTT/页（200 轮 × 150 页）==")
	type bench struct {
		name    string
		page    func(context.Context, string, *cursor, int) ([]string, *cursor)
		pageLat []time.Duration
		walkLat []time.Duration
	}
	runs := []*bench{
		{"慢版：2 次 ZREVRANGEBYSCORE + ZSCORE 回查（3 次 RTT）", pageLex, nil, nil},
		{"快版：2 次 ZREVRANGEBYSCORE ... WITHSCORES（2 次 RTT）", pageLexWS, nil, nil},
	}
	for _, b := range runs {
		for round := 0; round < 200; round++ {
			t0 := time.Now()
			cur := &cursor{score: 1<<62 - 1, member: "\xff"}
			for page := 0; page < 250; page++ {
				p0 := time.Now()
				ms, next := b.page(ctx, keyPad, cur, pageSize)
				b.pageLat = append(b.pageLat, time.Since(p0))
				if len(ms) == 0 {
					break
				}
				cur = next
			}
			b.walkLat = append(b.walkLat, time.Since(t0))
		}
		sort.Slice(b.pageLat, func(i, j int) bool { return b.pageLat[i] < b.pageLat[j] })
		sort.Slice(b.walkLat, func(i, j int) bool { return b.walkLat[i] < b.walkLat[j] })
	}
	fmt.Printf("%-46s %10s %10s %12s %12s\n", "方案", "单页P50", "单页P99", "整轮P50", "整轮P99")
	for _, b := range runs {
		fmt.Printf("%-46s %10v %10v %12v %12v\n", b.name,
			b.pageLat[len(b.pageLat)/2], b.pageLat[len(b.pageLat)*99/100],
			b.walkLat[len(b.walkLat)/2], b.walkLat[len(b.walkLat)*99/100])
	}
	fmt.Printf("   单页样本数=%d，整轮样本数=%d（整轮=150 页翻完 3000 条）\n\n",
		len(runs[0].pageLat), len(runs[0].walkLat))

	// 5. 游标编码 + HMAC
	fmt.Println("== 5. 游标编码 + HMAC 签名防篡改 ==")
	cur := &cursor{score: 1730000001000, member: "feed:000000002987"}
	tok := encodeCursor(cur)
	fmt.Printf("   编码后：%s（%d 字节）\n", tok, len(tok))
	back, err := decodeCursor(tok)
	fmt.Printf("   解码：%+v err=%v\n", back, err)
	bad := []byte(tok)
	if bad[3] == 'A' {
		bad[3] = 'B'
	} else {
		bad[3] = 'A'
	}
	_, err = decodeCursor(string(bad))
	fmt.Printf("   篡改 1 个字符后：err=%v\n", err)
}

// ---------- 策略 ----------

type cursor struct {
	score  int64
	member string
}

type strat int

const (
	stratNaive strat = iota
	stratNumeric
	stratLex
)

func walk(truth []string, key string, st strat) walkResult {
	res := walkResult{}
	seen := map[string]bool{}
	cur := &cursor{score: 1<<62 - 1, member: "\xff"}
	for page := 0; page < 400; page++ {
		var got []string
		var next *cursor
		switch st {
		case stratNaive:
			got, next = pageNaive(ctx, key, cur, pageSize)
		case stratNumeric:
			got, next = pageWithTailWS(ctx, key, cur, pageSize, func(m string) bool { return numID(m) < numID(cur.member) })
		case stratLex:
			got, next = pageWithTailWS(ctx, key, cur, pageSize, func(m string) bool { return m < cur.member })
		}
		if len(got) == 0 {
			break
		}
		res.pages++
		res.returned += len(got)
		for _, m := range got {
			if seen[m] {
				res.dup++
			}
			seen[m] = true
		}
		cur = next
	}
	for _, m := range truth {
		if !seen[m] {
			res.missing++
		}
	}
	return res
}

func pageNaive(ctx context.Context, key string, cur *cursor, n int) ([]string, *cursor) {
	max := "+inf"
	if cur.score < 1<<62-1 {
		max = "(" + strconv.FormatInt(cur.score, 10)
	}
	ms := rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{Max: max, Min: "-inf", Count: int64(n)}).Val()
	if len(ms) == 0 {
		return nil, cur
	}
	last := ms[len(ms)-1]
	s := rdb.ZScore(ctx, key, last).Val()
	return ms, &cursor{score: int64(s), member: last}
}

func pageWithTailWS(ctx context.Context, key string, cur *cursor, n int, less func(string) bool) ([]string, *cursor) {
	out := make([]string, 0, n)
	lastScore := int64(-1)
	if cur.score < 1<<62-1 {
		group := rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Max: strconv.FormatInt(cur.score, 10), Min: strconv.FormatInt(cur.score, 10), Count: 500}).Val()
		for _, z := range group {
			m := z.Member.(string)
			if less(m) {
				out = append(out, m)
				lastScore = int64(z.Score)
				if len(out) == n {
					break
				}
			}
		}
	}
	if len(out) < n {
		max := "+inf"
		if cur.score < 1<<62-1 {
			max = "(" + strconv.FormatInt(cur.score, 10)
		}
		rest := rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{Max: max, Min: "-inf", Count: int64(n - len(out))}).Val()
		for _, z := range rest {
			out = append(out, z.Member.(string))
			lastScore = int64(z.Score)
		}
	}
	if len(out) == 0 {
		return nil, cur
	}
	return out, &cursor{score: lastScore, member: out[len(out)-1]}
}

// pageLex / pageLexWS：耗时测试用，每页 3 次 / 2 次 RTT
func pageLex(ctx context.Context, key string, cur *cursor, n int) ([]string, *cursor) {
	return pageWithTailSlow(ctx, key, cur, n, func(m string) bool { return m < cur.member })
}

func pageLexWS(ctx context.Context, key string, cur *cursor, n int) ([]string, *cursor) {
	return pageWithTailWS(ctx, key, cur, n, func(m string) bool { return m < cur.member })
}

func pageWithTailSlow(ctx context.Context, key string, cur *cursor, n int, less func(string) bool) ([]string, *cursor) {
	out := make([]string, 0, n)
	if cur.score < 1<<62-1 {
		group := rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
			Max: strconv.FormatInt(cur.score, 10), Min: strconv.FormatInt(cur.score, 10), Count: 500}).Val()
		for _, m := range group {
			if less(m) {
				out = append(out, m)
				if len(out) == n {
					break
				}
			}
		}
	}
	if len(out) < n {
		max := "+inf"
		if cur.score < 1<<62-1 {
			max = "(" + strconv.FormatInt(cur.score, 10)
		}
		out = append(out, rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{Max: max, Min: "-inf", Count: int64(n - len(out))}).Val()...)
	}
	if len(out) == 0 {
		return nil, cur
	}
	last := out[len(out)-1]
	return out, &cursor{score: int64(rdb.ZScore(ctx, key, last).Val()), member: last}
}

func sameGroupTail(ctx context.Context, key string, score int64, keep func(string) bool) []string {
	group := rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Max: strconv.FormatInt(score, 10), Min: strconv.FormatInt(score, 10), Count: 500}).Val()
	var out []string
	for _, m := range group {
		if keep(m) {
			out = append(out, m)
		}
	}
	return out
}

// ---------- 游标编码 ----------

var secret = []byte("ch70-2-feed-cursor")

func encodeCursor(c *cursor) string {
	payload := make([]byte, 8+len(c.member))
	binary.BigEndian.PutUint64(payload, uint64(c.score))
	copy(payload[8:], c.member)
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)[:8]...))
}

func decodeCursor(tok string) (*cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) < 9 {
		return nil, fmt.Errorf("非法游标")
	}
	payload, sig := raw[:len(raw)-8], raw[len(raw)-8:]
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)[:8]) {
		return nil, fmt.Errorf("游标签名校验失败（被篡改或跨环境复用）")
	}
	return &cursor{score: int64(binary.BigEndian.Uint64(payload[:8])), member: string(payload[8:])}, nil
}

// ---------- 工具 ----------

func scoreOf(n int64) int64 { return 1_700_000_000_000 + (n/3)*1000 }

func numID(m string) int64 {
	if strings.HasPrefix(m, "feed:") {
		m = m[5:]
	}
	n, _ := strconv.ParseInt(m, 10, 64)
	return n
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
