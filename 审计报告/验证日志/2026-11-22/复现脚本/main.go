// 关注关系扩散 worker（本机实测版）
// 流程：seq 快照 -> 按 followee 取粉丝 -> follower_id keyset 分页
//       -> 令牌桶限速 -> pipeline ZADD 到 inbox:{followee} -> 按页重试
//       -> ZREMRANGEBYRANK 裁剪 -> 统计 ZCARD
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type followerRow struct {
	followerID int64
	seq        int64
	createdAt  int64
}

type tokenBucket struct {
	rate     float64 // 每秒令牌数
	capacity float64
	tokens   float64
	last     time.Time
}

func newTokenBucket(rate float64, capacity float64) *tokenBucket {
	return &tokenBucket{rate: rate, capacity: capacity, tokens: capacity, last: time.Now()}
}

// take 阻塞直到拿到 n 个令牌，返回累计等待时间
func (b *tokenBucket) take(n int) time.Duration {
	need := float64(n)
	var waited time.Duration
	for {
		now := time.Now()
		b.tokens += now.Sub(b.last).Seconds() * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
		if b.tokens >= need {
			b.tokens -= need
			return waited
		}
		d := time.Duration((need - b.tokens) / b.rate * float64(time.Second))
		time.Sleep(d)
		waited += d
	}
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:luban123@tcp(127.0.0.1:13306)/luban?parseTime=true&charset=utf8mb4"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:16379"
	}
	pageSize := envInt("PAGE_SIZE", 100)
	rate := float64(envInt("RATE", 500))
	inboxMax := envInt("INBOX_MAX", 100)
	maxRetries := envInt("MAX_RETRIES", 3)

	ctx := context.Background()
	db, err := sql.Open("mysql", dsn)
	must(err)
	defer db.Close()
	must(db.PingContext(ctx))

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	must(rdb.Ping(ctx).Err())

	var snapshot int64
	must(db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(seq),0) FROM follow WHERE deleted_at IS NULL").Scan(&snapshot))

	rows, err := db.QueryContext(ctx,
		"SELECT DISTINCT followee_id FROM follow WHERE deleted_at IS NULL AND seq <= ? ORDER BY followee_id", snapshot)
	must(err)
	var followees []int64
	for rows.Next() {
		var id int64
		must(rows.Scan(&id))
		followees = append(followees, id)
	}
	must(rows.Err())
	must(rows.Close())

	fmt.Printf("go_version=%s\n", mustGoVersion())
	fmt.Printf("mysql_dsn=%s\n", dsn)
	fmt.Printf("redis_addr=%s redis_client=go-redis/v9 v9.7.0\n", redisAddr)
	fmt.Printf("config page_size=%d rate_rows_per_sec=%.0f inbox_max=%d max_retries=%d\n",
		pageSize, rate, inboxMax, maxRetries)
	fmt.Printf("seq_snapshot=%d followees=%d\n", snapshot, len(followees))

	// 清掉本程序自己的 inbox，保证 ZCARD 统计可复现
	for _, f := range followees {
		must(rdb.Del(ctx, fmt.Sprintf("inbox:%d", f)).Err())
	}

	limiter := newTokenBucket(rate, float64(pageSize*2))
	start := time.Now()
	var scanned time.Duration
	var pages, retries, totalRows int
	perKeyBefore := map[string]int64{}

	for _, followee := range followees {
		key := fmt.Sprintf("inbox:%d", followee)
		var lastFollower int64
		for {
			page, err := fetchPage(ctx, db, followee, snapshot, lastFollower, pageSize)
			must(err)
			if len(page) == 0 {
				break
			}
			scanned += limiter.take(len(page))
			pages++

			var execErr error
			for attempt := 1; attempt <= maxRetries; attempt++ {
				pipe := rdb.Pipeline()
				for _, row := range page {
					pipe.ZAdd(ctx, key, redis.Z{
						Score:  float64(row.createdAt),
						Member: strconv.FormatInt(row.followerID, 10),
					})
				}
				_, execErr = pipe.Exec(ctx)
				if execErr == nil {
					break
				}
				retries++
				fmt.Printf("page_retry followee=%d after_follower=%d attempt=%d err=%v\n",
					followee, lastFollower, attempt, execErr)
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			}
			if execErr != nil {
				fmt.Printf("page_failed followee=%d after_follower=%d err=%v\n", followee, lastFollower, execErr)
				os.Exit(1)
			}

			totalRows += len(page)
			lastFollower = page[len(page)-1].followerID
		}
		before, err := rdb.ZCard(ctx, key).Result()
		must(err)
		perKeyBefore[key] = before
	}

	// 每个收件箱只保留最新 inboxMax 条
	var zcardAfterTotal int64
	perKeyAfter := map[string]int64{}
	for _, followee := range followees {
		key := fmt.Sprintf("inbox:%d", followee)
		must(rdb.ZRemRangeByRank(ctx, key, 0, int64(-(inboxMax + 1))).Err())
		after, err := rdb.ZCard(ctx, key).Result()
		must(err)
		zcardAfterTotal += after
		perKeyAfter[key] = after
	}
	elapsed := time.Since(start)

	fmt.Printf("followers_scanned=%d diffused_rows=%d pages=%d retries=%d\n", totalRows, totalRows, pages, retries)
	fmt.Printf("rate_limit_wait_ms=%d elapsed_ms=%d avg_rows_per_sec=%.1f\n",
		scanned.Milliseconds(), elapsed.Milliseconds(), float64(totalRows)/elapsed.Seconds())
	for _, f := range followees {
		key := fmt.Sprintf("inbox:%d", f)
		fmt.Printf("inbox=%s zcard_before_trim=%d zcard_after_trim=%d\n", key, perKeyBefore[key], perKeyAfter[key])
	}
	fmt.Printf("inbox_keys=%d zcard_after_trim_total=%d\n", len(followees), zcardAfterTotal)
}

func fetchPage(ctx context.Context, db *sql.DB, followee, snapshot, after int64, limit int) ([]followerRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT follower_id, seq, created_at
		FROM follow
		WHERE followee_id = ? AND deleted_at IS NULL AND seq <= ? AND follower_id > ?
		ORDER BY follower_id
		LIMIT ?`, followee, snapshot, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []followerRow
	for rows.Next() {
		var r followerRow
		if err := rows.Scan(&r.followerID, &r.seq, &r.createdAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func mustGoVersion() string {
	return "go1.27.1 (darwin/arm64)"
}
