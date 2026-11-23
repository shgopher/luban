package main

// 跨地域复制实验：writer 在 A 本地写，reader 在 B 本地轮询可见性。
// 编译：CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bench .
import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	mode := flag.String("mode", "writer", "writer|reader|latency")
	n := flag.Int("n", 200, "iterations")
	gap := flag.Duration("gap", 10*time.Millisecond, "sleep between writes")
	dsn := flag.String("dsn", "host=/run/postgresql user=postgres dbname=postgres sslmode=disable", "dsn")
	table := flag.String("table", "t2", "table")
	flag.Parse()

	db, err := sql.Open("postgres", *dsn)
	must(err)
	defer db.Close()
	must(db.Ping())

	switch *mode {
	case "writer":
		for i := 1; i <= *n; i++ {
			var ts time.Time
			// 一次事务写两行：t2 是被读的数据，w 记录写入侧时间戳，两者同事务提交
			err := db.QueryRow(`WITH a AS (INSERT INTO `+*table+` VALUES ($1) RETURNING id)
				INSERT INTO w VALUES ($1, clock_timestamp()) RETURNING ins_at`, i).Scan(&ts)
			must(err)
			fmt.Printf("%d,%d\n", i, ts.UnixNano())
			time.Sleep(*gap)
		}
	case "reader":
		for i := 1; i <= *n; i++ {
			for {
				var c int
				must(db.QueryRow(`SELECT count(*) FROM `+*table+` WHERE id=$1`, i).Scan(&c))
				if c > 0 {
					break
				}
				time.Sleep(2 * time.Millisecond)
			}
			var ts time.Time
			must(db.QueryRow(`SELECT clock_timestamp()`).Scan(&ts))
			fmt.Printf("%d,%d\n", i, ts.UnixNano())
		}
	case "latency":
		// 单行事务的提交延迟：本地连接，测的是 commit 是否等远端刷盘/回放
		for i := 1; i <= *n; i++ {
			t0 := time.Now()
			_, err := db.Exec(`INSERT INTO t3 VALUES ($1)`, 1000000+i)
			must(err)
			fmt.Printf("%d,%d\n", i, time.Since(t0).Nanoseconds())
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
