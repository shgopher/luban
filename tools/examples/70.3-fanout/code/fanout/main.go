// 70.3 实测 C：100 万粉丝的扩散 worker（seq 快照 + keyset 分页 + 批量 ZADD + 令牌桶限速 + 重试）。
//
// 分页按关系表的创建序 seq 做快照：snapshot_seq = 开始时最大 seq，分页 seq > cursor AND seq <= snapshot_seq。
// 为什么不用 max(follower_id)：follower_id 不是创建序——新粉丝是已有用户之后关注作者，
// 其 id 可以任意（多半落在旧上界之内），id 游标越过就漏写、不设 id 上界又会把新关系写进历史帖。
// 粉丝源用内存有序切片模拟；收件箱写真实 Redis（127.0.0.1:16393）。
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	addr           = "127.0.0.1:16393"
	userUniverse   = 2000000 // 用户池 1..200 万
	totalFollowers = 1000000 // 其中 100 万人关注了作者
	newFollowers   = 500     // 扩散期间新增关注
	pageSize       = 1000
	ratePerSec     = 50000
	fetchLatency   = 300 * time.Microsecond
	feedID         = "feed:000000009001" // 与出流侧 member 编码一致：12 位补零
	bucketMs       = 1000 // 峰值速率取样窗口改成 1s，避免 100ms 桶把令牌桶的突发放大
)

type relation struct {
	followerID int64
	seq        int64 // 关系创建序，全局单调（生产用 snowflake/号段）
}

type runStats struct {
	label         string
	dur           time.Duration
	pages         int
	rows          int
	peakRate      float64
	retries       int
	dup           int
	newExcluded   int
	unfollowedHit int
}

func main() {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 8})
	defer rdb.Close()
	info, _ := rdb.Info(ctx, "server").Result()
	fmt.Printf("# env: macOS arm64, Go %s, Redis %s, date=%s\n",
		runtime.Version(), grep(info, "redis_version:"), time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("# scenario: 粉丝=%d page=%d 限速=%.0f 行/秒 每页拉取=%s 注入失败=0.3%%/批（首次失败后重试一次）\n",
		totalFollowers, pageSize, float64(ratePerSec), fetchLatency)

	rnd := rand.New(rand.NewSource(20261121))
	perm := rnd.Perm(userUniverse) // 用户池里随机选 100 万人作为粉丝，id 与关注顺序无关
	initialIDs := make([]int64, totalFollowers)
	for i := range initialIDs {
		initialIDs[i] = int64(perm[i] + 1)
	}
	rels := make([]relation, totalFollowers) // 按 seq 升序：seq = 关注发生顺序
	for i, id := range initialIDs {
		rels[i] = relation{followerID: id, seq: int64(i + 1)}
	}
	snapshotSeq := int64(totalFollowers)

	// 扩散期间新增 500 个粉丝：400 个是池中已有用户（id 大多 < 旧 max id），100 个是新注册（id 更大）
	inInitial := make(map[int64]bool, totalFollowers)
	for _, id := range initialIDs {
		inInitial[id] = true
	}
	newRels := make([]relation, 0, newFollowers)
	existingCandidates := make([]int64, 0, userUniverse-totalFollowers)
	for u := 1; u <= userUniverse; u++ {
		id := int64(u)
		if !inInitial[id] {
			existingCandidates = append(existingCandidates, id)
		}
	}
	for i := 0; i < 400; i++ {
		id := existingCandidates[rnd.Intn(len(existingCandidates))]
		newRels = append(newRels, relation{followerID: id, seq: snapshotSeq + int64(len(newRels)+1)})
	}
	for i := 0; i < 100; i++ {
		id := int64(userUniverse + 1 + rnd.Intn(100000)) // 新注册用户，id 超出原用户池
		newRels = append(newRels, relation{followerID: id, seq: snapshotSeq + int64(len(newRels)+1)})
	}

	// 扩散中被取关的 1000 人（从初始粉丝里抽）
	unfollowed := make(map[int64]bool, 1000)
	for i := 0; i < 1000; i++ {
		unfollowed[initialIDs[rnd.Intn(totalFollowers)]] = true
	}

	// 反事实：若按 follower_id 排序 + id 游标做快照，插入点（第 500 页之后）越过多少新关系
	sortedIDs := append([]int64(nil), initialIDs...)
	sort.Slice(sortedIDs, func(i, j int) bool { return sortedIDs[i] < sortedIDs[j] })
	cursorIDAtInsert := sortedIDs[500*pageSize-1]
	wrongMissed, wrongExtra := 0, 0
	for _, r := range newRels {
		if r.followerID <= cursorIDAtInsert {
			wrongMissed++
		} else {
			wrongExtra++
		}
	}
	lowID, highID := 0, 0
	for _, r := range newRels {
		if r.followerID <= sortedIDs[len(sortedIDs)-1] {
			lowID++
		} else {
			highID++
		}
	}

	// 第一轮：限速 5 万行/秒
	must(rdb.FlushAll(ctx).Err())
	time.Sleep(100 * time.Millisecond)
	limited := run(ctx, rdb, rels, snapshotSeq, newRels, unfollowed, true, "LIMITED 5万行/秒")

	// 第二轮：不限速（同进程对照）
	must(rdb.FlushAll(ctx).Err())
	time.Sleep(100 * time.Millisecond)
	unlimited := run(ctx, rdb, rels, snapshotSeq, newRels, unfollowed, false, "UNLIMITED 不限速")

	// 幂等验证：对同一个粉丝重复写同一 feed，ZSET 长度不变
	reWrite(ctx, rdb, initialIDs[0])
	zcard := rdb.ZCard(ctx, fmt.Sprintf("inbox:%d", initialIDs[0])).Val()
	fmt.Printf("\n# 幂等: 对同一 inbox 重复 ZADD 同一 member 3 次后 ZCARD=%d（应为 1）\n", zcard)

	mem := rdb.Info(ctx, "memory").Val()
	fmt.Printf("# Redis: DBSIZE=%d used_memory=%s\n", rdb.DBSize(ctx).Val(), grep(mem, "used_memory_human:"))

	fmt.Println("\n# 快照口径与反事实")
	fmt.Printf("snapshot_seq=%d（创建序上界）；500 条新关系 seq=%d..%d，follower_id 有 %d 条 <= 初始粉丝最大 id、%d 条更大\n",
		snapshotSeq, snapshotSeq+1, snapshotSeq+newFollowers, lowID, highID)
	fmt.Printf("反事实：若按 max(follower_id) 做上界并用 id 游标，第 500 页时 id 游标=%d，500 条新关系里 %d 条会被漏写、%d 条若不加边界会被写进历史帖\n",
		cursorIDAtInsert, wrongMissed, wrongExtra)

	fmt.Println("\n# 页面成本（推算）：真实分片 MySQL 每页要打 8 个分片，各按 (followee_id, seq) 取 1000 行再归并；")
	fmt.Println("# 本程序用 300µs/页模拟单分片索引查询，8 分片顺序归并约 1.6–4 ms/页（单片 0.2–0.5 ms 量级）")
	fans := 120000000.0
	fmt.Println("\n# 推算（用不限速实测吞吐算单连接理论上限，用限速实测算 1.2 亿粉丝的「可控」扇出耗时）")
	uRate := float64(unlimited.rows) / unlimited.dur.Seconds()
	lRate := float64(limited.rows) / limited.dur.Seconds()
	fmt.Printf("不限速单连接: %.0f 行/秒 → 120000000 / %.0f = %.1f 秒\n", uRate, uRate, fans/uRate)
	fmt.Printf("限速 5 万/秒: %.0f 行/秒 → 120000000 / %.0f = %.1f 秒（%.1f 分钟），16 个 worker = %.1f 秒\n",
		lRate, lRate, fans/lRate, fans/lRate/60, fans/lRate/16)

	_ = limited
	_ = unlimited
}

func run(ctx context.Context, rdb *redis.Client, rels []relation, snapshotSeq int64,
	newRels []relation, unfollowed map[int64]bool, limited bool, label string) runStats {
	st := runStats{label: label}
	lim := &limiter{rate: ratePerSec, cap: pageSize, last: time.Now(), tokens: pageSize}
	buckets := make(map[int64]int)
	start := time.Now()
	cursor := int64(0)
	seen := make(map[int64]bool, totalFollowers)

	for {
		page, ok := fetchPage(rels, cursor, snapshotSeq)
		if !ok {
			break
		}
		if limited {
			lim.wait(len(page))
		}
		time.Sleep(fetchLatency) // 模拟 keyset 分页查询（MySQL 侧）耗时
		if st.pages == 500 {     // 扩散中途新增关注（seq > snapshot，必须排除）
			rels = append(rels, newRels...)
		}
		if err := writePage(ctx, rdb, page, &st); err != nil {
			fmt.Println("FATAL write:", err)
			os.Exit(1)
		}
		for _, r := range page {
			if seen[r.followerID] {
				st.dup++
			}
			seen[r.followerID] = true
			b := int64(time.Since(start) / (bucketMs * time.Millisecond))
			buckets[b]++
		}
		st.pages++
		st.rows += len(page)
		cursor = page[len(page)-1].seq
	}
	st.dur = time.Since(start)

	maxBucket := 0
	for _, n := range buckets {
		if n > maxBucket {
			maxBucket = n
		}
	}
	st.peakRate = float64(maxBucket) * (1000.0 / bucketMs)

	st.newExcluded = len(newRels)
	excludedOK := true
	for _, r := range newRels {
		if rdb.Exists(ctx, fmt.Sprintf("inbox:%d", r.followerID)).Val() != 0 {
			excludedOK = false
		}
	}
	pipe := rdb.Pipeline()
	cmds := make([]*redis.IntCmd, 0, len(unfollowed))
	for id := range unfollowed {
		cmds = append(cmds, pipe.Exists(ctx, fmt.Sprintf("inbox:%d", id)))
	}
	pipe.Exec(ctx)
	for _, c := range cmds {
		if c.Val() == 1 {
			st.unfollowedHit++
		}
	}

	fmt.Printf("\n--- %s\n", label)
	fmt.Printf("总耗时=%s 页数=%d 写入行数=%d 平均速率=%.0f 行/秒\n",
		st.dur.Round(time.Millisecond), st.pages, st.rows, float64(st.rows)/st.dur.Seconds())
	fmt.Printf("峰值写速率=%.0f 行/秒（%dms 窗口最大值）重试次数=%d 分页重复=%d\n",
		st.peakRate, bucketMs, st.retries, st.dup)
	fmt.Printf("扩散中新增关注=%d 全部排除=%v（新粉丝不补历史帖，靠下次刷新走拉路径）\n",
		st.newExcluded, excludedOK)
	fmt.Printf("扩散中被取关=%d 其中收件箱仍有行=%d（不回删，出流读时按关系状态过滤）\n",
		len(unfollowed), st.unfollowedHit)
	return st
}

// fetchPage：按创建序 keyset 分页——从 cursor(seq) 之后取一页，且 seq 不超过扩散开始时的快照上界。
func fetchPage(rels []relation, cursorSeq, snapshotSeq int64) ([]relation, bool) {
	i := sort.Search(len(rels), func(k int) bool { return rels[k].seq > cursorSeq })
	out := make([]relation, 0, pageSize)
	for _, r := range rels[i:] {
		if r.seq > snapshotSeq || len(out) >= pageSize {
			break
		}
		out = append(out, r)
	}
	return out, len(out) > 0
}

// writePage：批量 ZADD + 注入 0.3% 瞬时失败，失败后整批重试（ZADD 幂等，重试安全）。
func writePage(ctx context.Context, rdb *redis.Client, page []relation, st *runStats) error {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt == 0 && rand.Float64() < 0.003 {
			st.retries++
			time.Sleep(2 * time.Millisecond) // 模拟一次超时
			continue
		}
		pipe := rdb.Pipeline()
		for _, r := range page {
			pipe.ZAdd(ctx, fmt.Sprintf("inbox:%d", r.followerID), redis.Z{
				Score:  float64(1760000000000 + r.seq),
				Member: feedID,
			})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			if attempt >= 2 {
				return err
			}
			st.retries++
			continue
		}
		return nil
	}
	return fmt.Errorf("page failed after 3 attempts")
}

func reWrite(ctx context.Context, rdb *redis.Client, id int64) {
	for i := 0; i < 3; i++ {
		must(rdb.ZAdd(ctx, fmt.Sprintf("inbox:%d", id), redis.Z{Score: 1760000000000 + float64(id), Member: feedID}).Err())
	}
}

// limiter：令牌桶。每 len(page) 行消耗 token，桶容量 = 一页，补充速率 = ratePerSec。
type limiter struct {
	rate   float64
	cap    float64
	tokens float64
	last   time.Time
}

func (l *limiter) wait(n int) {
	for {
		now := time.Now()
		l.tokens += now.Sub(l.last).Seconds() * l.rate
		if l.tokens > l.cap {
			l.tokens = l.cap
		}
		l.last = now
		if l.tokens >= float64(n) {
			l.tokens -= float64(n)
			return
		}
		need := (float64(n) - l.tokens) / l.rate
		time.Sleep(time.Duration(need * float64(time.Second)))
	}
}

func grep(s, prefix string) string {
	for i := 0; i+len(prefix) <= len(s); i++ {
		if s[i:i+len(prefix)] == prefix {
			j := i + len(prefix)
			for j < len(s) && s[j] != '\n' && s[j] != '\r' {
				j++
			}
			return s[i+len(prefix) : j]
		}
	}
	return ""
}

func must(err error) {
	if err != nil {
		fmt.Println("FATAL:", err)
		os.Exit(1)
	}
}
