// 实验 C：离线收件箱规模与拉取成本（群聊场景：20 个群 × 5000 条消息，用户离线 7 天积压 5000 条）
// 对比「按人存收件箱 + seq 游标分页」与「按会话扫表再过滤」的拉取成本；量分页延迟、内存放大、保留策略裁剪。
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const (
	nGroups   = 20
	msgsPerGrp = 5000 // 每个群的消息总量
	perUser    = 250  // 用户在每个群积压的条数（= 每群最后 250 条）
	pageSize   = 200
	nInbox     = nGroups * perUser // 5000
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16382"})
	defer rdb.Close()
	must(rdb.Ping(ctx).Err())
	keys, _ := rdb.Keys(ctx, "s662:off:*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	// ---------- 造数：20 个群 × 5000 条消息；用户 u1001 在每个群积压最后 250 条 ----------
	tSetup := time.Now()
	for g := 0; g < nGroups; g++ {
		pipe := rdb.Pipeline()
		for s := 1; s <= msgsPerGrp; s++ {
			mid := fmt.Sprintf("g%d:m%d", g, s)
			pipe.HSet(ctx, fmt.Sprintf("s662:off:grp:%d", g), s,
				fmt.Sprintf(`{"body":"群%d第%d条","ts":%d}`, g, s, 1763000000+s))
			pipe.ZAdd(ctx, fmt.Sprintf("s662:off:grpz:%d", g), redis.Z{Score: float64(s), Member: mid})
			if s > msgsPerGrp-perUser { // 该用户未读的那 250 条进他的收件箱
				pipe.ZAdd(ctx, "s662:off:inbox:u1001", redis.Z{Score: float64((g*perUser)+(s-(msgsPerGrp-perUser))), Member: mid})
			}
		}
		_, err := pipe.Exec(ctx)
		must(err)
	}
	setup := time.Since(tSetup)
	zcard, _ := rdb.ZCard(ctx, "s662:off:inbox:u1001").Result()
	if int(zcard) != nInbox {
		panic(fmt.Sprintf("收件箱 %d != %d", zcard, nInbox))
	}
	fmt.Println("== 实验 C：离线收件箱 5000 条（20 个群 × 5000 条消息，用户离线 7 天，每页 200） ==")
	fmt.Printf("造数：%d 个群 × %d 条消息 = %d 条；用户收件箱 %d 条；pipeline 造数耗时=%v\n",
		nGroups, msgsPerGrp, nGroups*msgsPerGrp, zcard, setup.Round(time.Millisecond))

	// ---------- C1：按人收件箱 + seq 游标分页（ZRANGEBYSCORE 一次取一页 + pipeline 取本体） ----------
	type pageStat struct {
		idx int
		ms  time.Duration
		got int
	}
	var pages []pageStat
	cursor := int64(0)
	bodyFetches := 0
	tPull := time.Now()
	idx := 0
	for {
		idx++
		t0 := time.Now()
		members, err := rdb.ZRangeByScore(ctx, "s662:off:inbox:u1001", &redis.ZRangeBy{
			Min: "(" + fmt.Sprint(cursor), Max: "+inf", Count: pageSize,
		}).Result()
		must(err)
		if len(members) > 0 {
			pipe := rdb.Pipeline()
			for _, m := range members {
				var g, s int
				fmt.Sscanf(m, "g%d:m%d", &g, &s)
				pipe.HGet(ctx, fmt.Sprintf("s662:off:grp:%d", g), fmt.Sprint(s))
			}
			_, err = pipe.Exec(ctx)
			must(err)
			bodyFetches += len(members)
			// 游标从最后一条的 score 取（真实实现把 conv_seq 编码进 member，可省掉这次往返）
			scores, _ := rdb.ZMScore(ctx, "s662:off:inbox:u1001", members[len(members)-1]).Result()
			if len(scores) == 1 {
				cursor = int64(scores[0])
			}
		}
		pages = append(pages, pageStat{idx: idx, ms: time.Since(t0), got: len(members)})
		if len(members) < pageSize {
			break
		}
	}
	totalPull := time.Since(tPull)
	var maxPage, minPage time.Duration = 0, 1 << 62
	var sumPage time.Duration
	for _, p := range pages {
		sumPage += p.ms
		if p.ms > maxPage {
			maxPage = p.ms
		}
		if p.ms < minPage {
			minPage = p.ms
		}
	}
	fmt.Printf("C1 按人收件箱：分页数=%d 总耗时=%v 单页均值=%v 最慢页=%v 拉取条次=%d 游标=end\n",
		len(pages), totalPull.Round(time.Microsecond), (sumPage / time.Duration(len(pages))).Round(time.Microsecond),
		maxPage.Round(time.Microsecond), bodyFetches)
	for _, p := range pages {
		if p.idx <= 3 || p.idx == len(pages)/2 || p.idx == len(pages) {
			fmt.Printf("   page %2d: got=%3d 耗时=%v\n", p.idx, p.got, p.ms.Round(time.Microsecond))
		}
	}

	// ---------- C2：按会话扫表：每个群全量扫 5000 条，再按「每群最后 250 条」过滤 ----------
	tScan := time.Now()
	scanned, found := 0, 0
	for g := 0; g < nGroups; g++ {
		all, err := rdb.ZRange(ctx, fmt.Sprintf("s662:off:grpz:%d", g), 0, -1).Result()
		must(err)
		scanned += len(all)
		for _, m := range all {
			var gg, s int
			fmt.Sscanf(m, "g%d:m%d", &gg, &s)
			if s > msgsPerGrp-perUser { // 过滤条件：用户在该群的未读游标之后
				found++
			}
		}
	}
	scanCost := time.Since(tScan)
	fmt.Printf("C2 按会话扫表：扫描群=%d 扫描条目=%d 命中=%d 总耗时=%v（找回同样的 %d 条）\n",
		nGroups, scanned, found, scanCost.Round(time.Millisecond), found)
	fmt.Printf("成本倍数：扫表扫描条目/收件箱条目 = %d/%d = %.0fx；扫表耗时/收件箱拉取耗时 = %.2fx\n",
		scanned, nInbox, float64(scanned)/float64(nInbox), float64(scanCost)/float64(totalPull))

	// ---------- C3：内存与存储放大 ----------
	memInbox, _ := rdb.MemoryUsage(ctx, "s662:off:inbox:u1001").Result()
	memGrp, _ := rdb.MemoryUsage(ctx, "s662:off:grp:0").Result()
	memGrpZ, _ := rdb.MemoryUsage(ctx, "s662:off:grpz:0").Result()
	dbsize, _ := rdb.DBSize(ctx).Result()
	info, _ := rdb.Info(ctx, "memory").Result()
	usedMem := ""
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "used_memory:") {
			usedMem = strings.TrimSpace(strings.TrimPrefix(line, "used_memory:"))
		}
	}
	perEntry := memInbox / int64(nInbox)
	fmt.Printf("C3 内存：used_memory=%s bytes DBSIZE=%d 收件箱ZSET(%d条)=%d bytes → %d bytes/条\n",
		usedMem, dbsize, nInbox, memInbox, perEntry)
	fmt.Printf("   单个群消息hash(%d条)=%d bytes；单个群ZSET(%d条)=%d bytes\n", msgsPerGrp, memGrp, msgsPerGrp, memGrpZ)
	fmt.Printf("   放大估算：1 条消息 × 1 个离线设备 ≈ %d bytes 收件箱；500 人群全员离线 ≈ %d bytes/条\n",
		perEntry, 500*perEntry)

	// ---------- C4：保留策略：时间 + 条数双上限，超限裁剪 ----------
	pipe := rdb.Pipeline()
	for i := 0; i < 1000; i++ {
		pipe.ZAdd(ctx, "s662:off:inbox:u1001", redis.Z{Score: float64(nInbox + i + 1), Member: fmt.Sprintf("g0:extra%d", i)})
	}
	_, err := pipe.Exec(ctx)
	must(err)
	before, _ := rdb.ZCard(ctx, "s662:off:inbox:u1001").Result()
	t0 := time.Now()
	nDel, err := rdb.ZRemRangeByRank(ctx, "s662:off:inbox:u1001", 0, 999).Result() // 只保留最新 5000 条
	must(err)
	trim := time.Since(t0)
	after, _ := rdb.ZCard(ctx, "s662:off:inbox:u1001").Result()
	fmt.Printf("C4 保留策略：裁剪前 %d 条，ZREMRANGEBYRANK 删最老 %d 条，耗时=%v，裁剪后 %d 条\n",
		before, nDel, trim.Round(time.Microsecond), after)

	// ---------- C5：拉完即删（客户端 ack 后清收件箱） ----------
	t0 = time.Now()
	removed, err := rdb.ZRemRangeByScore(ctx, "s662:off:inbox:u1001", "-inf", "+inf").Result()
	must(err)
	clean := time.Since(t0)
	left, _ := rdb.ZCard(ctx, "s662:off:inbox:u1001").Result()
	fmt.Printf("C5 拉完即删：ZREMRANGEBYSCORE 清空 %d 条，耗时=%v，剩余 %d 条\n",
		removed, clean.Round(time.Microsecond), left)
	_ = cursor
}
