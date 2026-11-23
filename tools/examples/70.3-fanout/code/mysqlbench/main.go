// 70.3 实测 A：MySQL 20 万行关注关系，两种分片模型 × 两个查询方向。
// 8 个物理分片表模拟 8 个分片；模型 A 按 follower_id 分片，模型 B 按 followee_id 分片。
// 同一进程里按轮次交替跑 4 个查询方向，每个方向取 15 轮中位数，避免并行任务造成的绝对性能漂移。
package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	dsn        = "root:@tcp(127.0.0.1:13390)/"
	dbName     = "ch70_3"
	shards     = 8
	totalEdges = 200000
	rounds     = 15
	pageSize   = 200
	bigV       = 9999  // 被 5 万人关注
	medium     = 8888  // 被 5000 人关注
	heavy      = 77777 // 关注了 1000 人
)

// querier 让同一段查询既能跑在连接池上（计时），也能跑在单连接上（读 Handler 计数）。
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type edge struct{ follower, followee int64 }

type scenario struct {
	name    string
	explain func() string
	run     func(querier) int
	rows    int
}

func main() {
	ctx := context.Background()
	admin, err := sql.Open("mysql", dsn)
	must(err)
	defer admin.Close()
	must(admin.PingContext(ctx))

	fmt.Printf("# env: macOS arm64, Go %s, NumCPU=%d, MySQL ", runtime.Version(), runtime.NumCPU())
	var ver string
	must(admin.QueryRowContext(ctx, "select version()").Scan(&ver))
	fmt.Printf("%s, date=%s\n", ver, time.Now().Format("2006-01-02 15:04:05"))

	must2(admin.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+dbName+" DEFAULT CHARACTER SET utf8mb4"))
	db, err := sql.Open("mysql", dsn+dbName)
	must(err)
	defer db.Close()
	db.SetMaxOpenConns(16)
	must(db.PingContext(ctx))

	for _, m := range []string{"a", "b"} {
		for i := 0; i < shards; i++ {
			must2(db.ExecContext(ctx, "DROP TABLE IF EXISTS follow_"+m+"_"+itoa(i)))
		}
	}
	createTables(ctx, db)

	edges := genEdges()
	fmt.Printf("# data: edges=%d\n", len(edges))
	load(ctx, db, "a", edges, func(e edge) int { return int(e.follower % shards) },
		"INSERT INTO follow_a_%d (follower_id,followee_id,state,created_at,seq) VALUES ")
	load(ctx, db, "b", edges, func(e edge) int { return int(e.followee % shards) },
		"INSERT INTO follow_b_%d (followee_id,follower_id,state,created_at,seq) VALUES ")
	stats(ctx, db, edges)
	for _, t := range []string{"follow_a", "follow_b"} {
		for i := 0; i < shards; i++ {
			must2(db.ExecContext(ctx, "ANALYZE TABLE "+t+"_"+itoa(i)))
		}
	}

	fmt.Println("\n# phase 1: 无本地二级索引（只有主键）")
	phase1 := []*scenario{
		{
			name: "A-routed   关注列表 heavy(77777 关注 1000 人) 第 1 页",
			explain: func() string {
				return explainOne(ctx, db, fmt.Sprintf("SELECT followee_id FROM follow_a_%d WHERE follower_id=%d AND state=0 ORDER BY followee_id LIMIT %d", heavy%shards, heavy, pageSize))
			},
			run: func(q querier) int {
				return countRows(ctx, q, fmt.Sprintf("SELECT followee_id FROM follow_a_%d WHERE follower_id=%d AND state=0 ORDER BY followee_id LIMIT %d", heavy%shards, heavy, pageSize))
			},
		},
		{
			name: "A-scatter  粉丝列表 medium(8888 被 5000 人关注) 第 1 页",
			explain: func() string {
				return explainScatter(ctx, db, "follow_a", "follower_id", "followee_id", medium)
			},
			run: func(q querier) int {
				return scatter(ctx, q, "follow_a", "follower_id", "followee_id", medium)
			},
		},
		{
			name: "B-routed   粉丝列表 medium(8888 被 5000 人关注) 第 1 页",
			explain: func() string {
				return explainOne(ctx, db, fmt.Sprintf("SELECT follower_id FROM follow_b_%d WHERE followee_id=%d AND state=0 ORDER BY follower_id LIMIT %d", medium%shards, medium, pageSize))
			},
			run: func(q querier) int {
				return countRows(ctx, q, fmt.Sprintf("SELECT follower_id FROM follow_b_%d WHERE followee_id=%d AND state=0 ORDER BY follower_id LIMIT %d", medium%shards, medium, pageSize))
			},
		},
		{
			name: "B-scatter  关注列表 heavy(77777 关注 1000 人) 第 1 页",
			explain: func() string {
				return explainScatter(ctx, db, "follow_b", "followee_id", "follower_id", heavy)
			},
			run: func(q querier) int {
				return scatter(ctx, q, "follow_b", "followee_id", "follower_id", heavy)
			},
		},
	}
	runPhase(ctx, db, phase1)

	fmt.Println("\n# phase 2: 给反向查询加本地二级索引（分片键不变，仍然要访问全部 8 个分片）")
	for i := 0; i < shards; i++ {
		must2(db.ExecContext(ctx, "ALTER TABLE follow_a_"+itoa(i)+" ADD INDEX idx_followee (followee_id,follower_id)"))
		must2(db.ExecContext(ctx, "ALTER TABLE follow_b_"+itoa(i)+" ADD INDEX idx_follower (follower_id,followee_id)"))
	}
	for _, t := range []string{"follow_a", "follow_b"} {
		for i := 0; i < shards; i++ {
			must2(db.ExecContext(ctx, "ANALYZE TABLE "+t+"_"+itoa(i)))
		}
	}
	phase2 := []*scenario{
		{
			name: "A-scatter  粉丝列表 medium(8888 被 5000 人关注) 第 1 页 [有 idx_followee]",
			explain: func() string {
				return explainScatter(ctx, db, "follow_a", "follower_id", "followee_id", medium)
			},
			run: func(q querier) int {
				return scatter(ctx, q, "follow_a", "follower_id", "followee_id", medium)
			},
		},
		{
			name: "B-scatter  关注列表 heavy(77777 关注 1000 人) 第 1 页 [有 idx_follower]",
			explain: func() string {
				return explainScatter(ctx, db, "follow_b", "followee_id", "follower_id", heavy)
			},
			run: func(q querier) int {
				return scatter(ctx, q, "follow_b", "followee_id", "follower_id", heavy)
			},
		},
	}
	runPhase(ctx, db, phase2)

	fmt.Println("\n# phase 3: 大 V 粉丝列表（5 万粉丝）第一页")
	phase3 := []*scenario{
		{
			name: "B-routed   粉丝列表 bigV(9999 被 5 万人关注) 第 1 页",
			explain: func() string {
				return explainOne(ctx, db, fmt.Sprintf("SELECT follower_id FROM follow_b_%d WHERE followee_id=%d AND state=0 ORDER BY follower_id LIMIT %d", bigV%shards, bigV, pageSize))
			},
			run: func(q querier) int {
				return countRows(ctx, q, fmt.Sprintf("SELECT follower_id FROM follow_b_%d WHERE followee_id=%d AND state=0 ORDER BY follower_id LIMIT %d", bigV%shards, bigV, pageSize))
			},
		},
		{
			name: "A-scatter  粉丝列表 bigV(9999 被 5 万人关注) 第 1 页 [有 idx_followee]",
			explain: func() string {
				return explainScatter(ctx, db, "follow_a", "follower_id", "followee_id", bigV)
			},
			run: func(q querier) int {
				return scatter(ctx, q, "follow_a", "follower_id", "followee_id", bigV)
			},
		},
	}
	runPhase(ctx, db, phase3)
}

// runPhase 按轮次交替执行所有场景（同一进程、同一时段、同样的缓存状态），取中位数；
// 随后再用单连接跑一次，读 Handler 计数得到「实际扫描行数」。
func runPhase(ctx context.Context, db *sql.DB, list []*scenario) {
	durs := make([][]time.Duration, len(list))
	for i := range list {
		list[i].rows = -1
	}
	for r := 0; r < rounds; r++ {
		for i, s := range list {
			t0 := time.Now()
			s.rows = s.run(db)
			durs[i] = append(durs[i], time.Since(t0))
		}
	}
	for i, s := range list {
		sort.Slice(durs[i], func(a, b int) bool { return durs[i][a] < durs[i][b] })
		med := durs[i][len(durs[i])/2]
		fmt.Println("---", s.name)
		fmt.Println("EXPLAIN:", s.explain())
		fmt.Printf("结果行数=%d  中位耗时=%s  min=%s max=%s\n", s.rows, med, durs[i][0], durs[i][len(durs[i])-1])
	}
	for _, s := range list {
		conn, err := db.Conn(ctx)
		must(err)
		bN, bR, bK := handlerStats(ctx, conn)
		s.run(conn)
		aN, aR, aK := handlerStats(ctx, conn)
		conn.Close()
		fmt.Printf("PROBE %s | 实际扫描行数: index_next=%d rnd_next=%d key=%d\n", s.name, aN-bN, aR-bR, aK-bK)
	}
}

// ---------- 查询 ----------

func scatter(ctx context.Context, q querier, model, sel, where string, v int64) int {
	total := 0
	for i := 0; i < shards; i++ {
		total += countRows(ctx, q, fmt.Sprintf("SELECT %s FROM %s_%d WHERE %s=%d AND state=0 ORDER BY %s LIMIT %d",
			sel, model, i, where, v, sel, pageSize))
	}
	return total
}

func countRows(ctx context.Context, q querier, sqlText string) int {
	rows, err := q.QueryContext(ctx, sqlText)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var x int64
		if err := rows.Scan(&x); err != nil {
			panic(err)
		}
		n++
	}
	return n
}

// ---------- EXPLAIN / Handler ----------

func explainOne(ctx context.Context, db *sql.DB, q string) string {
	rows, err := db.QueryContext(ctx, "EXPLAIN FORMAT=TRADITIONAL "+q)
	if err != nil {
		return "explain error: " + err.Error()
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var out []string
	for rows.Next() {
		must(rows.Scan(ptrs...))
		typ, key, rowN := "", "", ""
		for i, c := range cols {
			v := valString(vals[i])
			switch c {
			case "type":
				typ = v
			case "key":
				key = v
			case "rows":
				rowN = v
			}
		}
		out = append(out, fmt.Sprintf("type=%s key=%s rows=%s", typ, key, rowN))
	}
	return strings.Join(out, " | ")
}

func explainScatter(ctx context.Context, db *sql.DB, model, sel, where string, v int64) string {
	var parts []string
	totalRows := 0.0
	for i := 0; i < shards; i++ {
		q := fmt.Sprintf("SELECT %s FROM %s_%d WHERE %s=%d AND state=0 ORDER BY %s LIMIT %d",
			sel, model, i, where, v, sel, pageSize)
		rows, err := db.QueryContext(ctx, "EXPLAIN FORMAT=TRADITIONAL "+q)
		if err != nil {
			return "explain error: " + err.Error()
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for k := range vals {
			ptrs[k] = &vals[k]
		}
		rows.Next()
		must(rows.Scan(ptrs...))
		typ, key, rowN := "", "", ""
		for i, c := range cols {
			vv := valString(vals[i])
			switch c {
			case "type":
				typ = vv
			case "key":
				key = vv
			case "rows":
				rowN = vv
				var f float64
				fmt.Sscanf(vv, "%f", &f)
				totalRows += f
			}
		}
		rows.Close()
		parts = append(parts, fmt.Sprintf("s%d[%s key=%s rows=%s]", i, typ, key, rowN))
	}
	return fmt.Sprintf("%s rows合计=%.0f", strings.Join(parts, " "), totalRows)
}

func handlerStats(ctx context.Context, conn *sql.Conn) (next, rndNext, key int64) {
	rows, err := conn.QueryContext(ctx, "SHOW SESSION STATUS LIKE 'Handler_read%'")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name, val string
		if err := rows.Scan(&name, &val); err != nil {
			return
		}
		var n int64
		fmt.Sscanf(val, "%d", &n)
		switch name {
		case "Handler_read_next":
			next = n
		case "Handler_read_rnd_next":
			rndNext = n
		case "Handler_read_key":
			key = n
		}
	}
	return
}

func valString(v any) string {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// ---------- 建表 / 造数 ----------

func createTables(ctx context.Context, db *sql.DB) {
	for i := 0; i < shards; i++ {
		must2(db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE follow_a_%d (
  follower_id BIGINT NOT NULL,
  followee_id BIGINT NOT NULL,
  state TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  seq BIGINT NOT NULL,
  PRIMARY KEY (follower_id, followee_id)
) ENGINE=InnoDB`, i)))
		must2(db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE follow_b_%d (
  followee_id BIGINT NOT NULL,
  follower_id BIGINT NOT NULL,
  state TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  seq BIGINT NOT NULL,
  PRIMARY KEY (followee_id, follower_id)
) ENGINE=InnoDB`, i)))
	}
}

// genEdges: 20 万条边，故意造出「有人关注 1000 人」「有人被 5 万 / 5 千人关注」的长尾。
func genEdges() []edge {
	seen := make(map[edge]struct{}, totalEdges)
	out := make([]edge, 0, totalEdges)
	add := func(f, e int64) {
		k := edge{f, e}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for f := int64(0); f < 50000; f++ { // bigV 的 5 万粉丝
		add(f, bigV)
	}
	for f := int64(50000); f < 55000; f++ { // medium 的 5 千粉丝
		add(f, medium)
	}
	for e := int64(0); e < 1000; e++ { // 重度用户关注 1000 人
		add(heavy, e)
	}
	r := rand.New(rand.NewSource(20261121))
	for len(out) < totalEdges { // 其余边：8 万用户 × 2 万账号的随机图
		add(int64(20000+r.Intn(80000)), int64(r.Intn(20000)))
	}
	return out
}

func load(ctx context.Context, db *sql.DB, model string, edges []edge, shardOf func(edge) int, prefix string) {
	t0 := time.Now()
	rowsPerShard := make([][]edge, shards)
	for _, e := range edges {
		s := shardOf(e)
		rowsPerShard[s] = append(rowsPerShard[s], e)
	}
	for s := 0; s < shards; s++ {
		rows := rowsPerShard[s]
		for i := 0; i < len(rows); i += 1000 {
			j := i + 1000
			if j > len(rows) {
				j = len(rows)
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf(prefix, s))
			args := make([]any, 0, (j-i)*5)
			for k, e := range rows[i:j] {
				if k > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("(?,?,?,?,?)")
				st := 0 // 2% 取关软删（state=1），模拟真实表里状态列
				if (e.follower+e.followee)%50 == 0 {
					st = 1
				}
				ts := int64(1760000000000 + (e.follower*7+e.followee)%1000000)
				sq := int64(1 + (e.follower*7919+e.followee*104729)%200000)
				if model == "b" {
					args = append(args, e.followee, e.follower, st, ts, sq) // 列顺序 followee_id, follower_id
				} else {
					args = append(args, e.follower, e.followee, st, ts, sq)
				}
			}
			must2(db.ExecContext(ctx, sb.String(), args...))
		}
	}
	fmt.Printf("# load model %s: %d rows, %s\n", model, len(edges), time.Since(t0).Round(time.Millisecond))
}

func stats(ctx context.Context, db *sql.DB, edges []edge) {
	out := make(map[int64]int)
	in := make(map[int64]int)
	for _, e := range edges {
		out[e.follower]++
		in[e.followee]++
	}
	maxOut, maxOutUser, maxIn, maxInUser := 0, int64(0), 0, int64(0)
	for u, n := range out {
		if n > maxOut {
			maxOut, maxOutUser = n, u
		}
	}
	for u, n := range in {
		if n > maxIn {
			maxIn, maxInUser = n, u
		}
	}
	fmt.Printf("# graph: distinct_followers=%d distinct_followees=%d max_out_degree=%d(user=%d) max_in_degree=%d(user=%d) rows_per_shard=",
		len(out), len(in), maxOut, maxOutUser, maxIn, maxInUser)
	for i := 0; i < shards; i++ {
		var n int
		must(db.QueryRowContext(ctx, "SELECT COUNT(*) FROM follow_a_"+itoa(i)).Scan(&n))
		fmt.Printf("%d ", n)
	}
	fmt.Println()
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

func must(err error) {
	if err != nil {
		fmt.Println("FATAL:", err)
		os.Exit(1)
	}
}
func must2(_ sql.Result, err error) { must(err) }
