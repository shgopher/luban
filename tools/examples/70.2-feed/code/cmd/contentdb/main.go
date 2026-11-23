// 内容正排回查 A/B：20 条单条主键查询 vs 1 条 IN 批量查询（MySQL 127.0.0.1:13390，库 ch70_2）
// 用途：支撑「Feed 列表只缓存 ID，正文走内容正排批量回查」这一句的落地成本。
// 运行：go run ./cmd/contentdb
package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	dsn    = "root@tcp(127.0.0.1:13390)/ch70_2?parseTime=false"
	total  = 20000
	rounds = 500
)

func main() {
	admin, err := sql.Open("mysql", "root@tcp(127.0.0.1:13390)/")
	must(err)
	_, err = admin.Exec("CREATE DATABASE IF NOT EXISTS ch70_2")
	must(err)
	admin.Close()

	db, err := sql.Open("mysql", dsn)
	must(err)
	defer db.Close()
	db.SetMaxOpenConns(8)

	var ver string
	must(db.QueryRow("SELECT VERSION()").Scan(&ver))
	fmt.Printf("实测 D：内容正排批量回查（MySQL %s @127.0.0.1:13390，库 ch70_2）\n", ver)

	seed(db)

	var cnt int
	must(db.QueryRow("SELECT COUNT(*) FROM feed_content").Scan(&cnt))
	fmt.Printf("表 feed_content 行数 = %d，回查 %d 条 ID，每方案 %d 轮\n\n", cnt, 20, rounds)

	ids := make([]int64, 20)
	for i := range ids {
		ids[i] = int64(1 + (i*937)%total)
	}

	single := make([]time.Duration, 0, rounds)
	batch := make([]time.Duration, 0, rounds)
	var rowsRead int

	for r := 0; r < rounds; r++ {
		t0 := time.Now()
		for _, id := range ids {
			var id2, author, created int64
			var status int
			var body string
			must(db.QueryRow("SELECT id, author_id, status, created_at, body FROM feed_content WHERE id = ?", id).
				Scan(&id2, &author, &status, &created, &body))
			rowsRead++
		}
		single = append(single, time.Since(t0))

		t0 = time.Now()
		q := "SELECT id, author_id, status, created_at, body FROM feed_content WHERE id IN (" +
			strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		rs, err := db.Query(q, args...)
		must(err)
		n := 0
		for rs.Next() {
			var id2, author, created int64
			var status int
			var body string
			must(rs.Scan(&id2, &author, &status, &created, &body))
			n++
		}
		rs.Close()
		if n != len(ids) {
			panic("批量回查条数不符")
		}
		batch = append(batch, time.Since(t0))
	}

	sort.Slice(single, func(i, j int) bool { return single[i] < single[j] })
	sort.Slice(batch, func(i, j int) bool { return batch[i] < batch[j] })
	fmt.Println("| 回查方式 | 每轮查询数 | P50 | P99 | 相对批量 |")
	fmt.Println("|---|---|---|---|---|")
	fmt.Printf("| 20 条单条主键查询（逐个 RTT） | 20 | %v | %v | %.1f× |\n",
		single[rounds/2], single[rounds*99/100], float64(single[rounds/2])/float64(batch[rounds/2]))
	fmt.Printf("| 1 条 IN 批量查询 | 1 | %v | %v | 1× |\n", batch[rounds/2], batch[rounds*99/100])
	fmt.Printf("\n主键命中（EXPLAIN 见 raw/mysql-explain.txt）：access_type=const/range，回查 %d 行\n", rowsRead/rounds*2)
}

func seed(db *sql.DB) {
	var n int
	must(db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='ch70_2' AND table_name='feed_content'").Scan(&n))
	if n == 0 {
		_, err := db.Exec(`CREATE TABLE feed_content (
  id BIGINT PRIMARY KEY,
  author_id BIGINT NOT NULL,
  status TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  body VARCHAR(256) NOT NULL,
  KEY k_author_time (author_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
		must(err)
	}
	must(db.QueryRow("SELECT COUNT(*) FROM feed_content").Scan(&n))
	if n >= total {
		return
	}
	r := rand.New(rand.NewSource(20261121))
	tx, err := db.Begin()
	must(err)
	for i := 0; i < total; i += 500 {
		var sb strings.Builder
		sb.WriteString("INSERT INTO feed_content(id,author_id,status,created_at,body) VALUES ")
		args := make([]any, 0, 2500)
		for j := 0; j < 500 && i+j < total; j++ {
			id := i + j + 1
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?,?,?)")
			args = append(args, id, 1+r.Intn(10000), r.Intn(3),
				1_700_000_000_000+int64(r.Intn(86_400_000)),
				fmt.Sprintf("第 %d 条正文内容", id))
		}
		_, err = tx.Exec(sb.String(), args...)
		must(err)
	}
	must(tx.Commit())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
