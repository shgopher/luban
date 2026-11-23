// e3_mysql: 1000 并发点赞的幂等性实测（MySQL 127.0.0.1:13390, db=ch70_4）。
//
//	场景 1（正确版）：唯一键 (user_id, object_id, action) + ON DUPLICATE KEY UPDATE；
//	                 affected rows 为 1/2 才 +1 计数（2 = 取消赞后重新激活）。
//	  1a) 同一用户对同一内容重复点赞 100 次 → 状态行 1、计数 1
//	  1b) 1000 个不同用户点赞同一内容     → 状态行 1000、计数 1000
//	场景 2（错误版）：先 SELECT 再 INSERT（无唯一键兜底），计数无条件 +1。
//	  2a) 同一用户重复点赞 100 次 → 重复行、超计数
//	  2b) 1000 个用户各发 5 次并发请求 → 计数膨胀
package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const dsn = "root@tcp(127.0.0.1:13390)/ch70_4?parseTime=true&loc=Local&timeout=5s"

var ctx = context.Background()

// 记录「真正插进去的状态行数」（INSERT affected rows 之和），用于对齐计数
var insertedRows int64

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func setup(db *sql.DB) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS like_state (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT UNSIGNED NOT NULL,
			object_id BIGINT UNSIGNED NOT NULL,
			object_type TINYINT NOT NULL DEFAULT 1,
			action TINYINT NOT NULL DEFAULT 1,
			state TINYINT NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_user_object_action (user_id, object_id, object_type, action),
			KEY idx_object_state (object_id, object_type, state)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS like_counter (
			object_id BIGINT UNSIGNED NOT NULL,
			object_type TINYINT NOT NULL DEFAULT 1,
			cnt BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (object_id, object_type)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS like_state_bad (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT UNSIGNED NOT NULL,
			object_id BIGINT UNSIGNED NOT NULL,
			action TINYINT NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_user_object (user_id, object_id, action)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS like_counter_bad (
			object_id BIGINT UNSIGNED PRIMARY KEY,
			cnt BIGINT NOT NULL DEFAULT 0
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			panic(fmt.Errorf("ddl failed: %w\n%s", err, s))
		}
	}
}

func reset(db *sql.DB) {
	for _, q := range []string{
		`DELETE FROM like_state`, `DELETE FROM like_counter`,
		`DELETE FROM like_state_bad`, `DELETE FROM like_counter_bad`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			panic(err)
		}
	}
}

func seedCounter(db *sql.DB, table string, oid uint64) {
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		`INSERT INTO %s (object_id, cnt) VALUES (?, 0) ON DUPLICATE KEY UPDATE cnt = 0`, table), oid)
	must(err)
}

type result struct {
	requests  int
	success   int64
	inserted  int64
	errs      int64
	elapsed   time.Duration
	rowCount  int64
	distinct  int64
	counter   int64
	duplicates int64
}

// 正确版：唯一键 + ON DUPLICATE KEY UPDATE，按 affected rows 判定状态迁移：
// 1=新插入，2=从 state=0 重新激活（取消赞后再赞），0=本来已赞 → 只有 1/2 才 +1 计数。
func goodLike(db *sql.DB, uid, oid uint64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO like_state (user_id, object_id, object_type, action, state)
		 VALUES (?, ?, 1, 1, 1)
		 ON DUPLICATE KEY UPDATE
		   updated_at = IF(state = 0, CURRENT_TIMESTAMP, updated_at),
		   state = 1`, uid, oid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 1 || n == 2 {
		atomic.AddInt64(&insertedRows, 1)
		if _, err := tx.ExecContext(ctx,
			`UPDATE like_counter SET cnt = cnt + 1 WHERE object_id = ? AND object_type = 1`, oid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// 取消赞：条件更新 state=1→0，只有真的从「赞」变「取消」才扣计数（cnt>0 保护）
func goodUnlike(db *sql.DB, uid, oid uint64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE like_state SET state = 0
		  WHERE user_id=? AND object_id=? AND object_type=1 AND action=1 AND state=1`, uid, oid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE like_counter SET cnt = cnt - 1
			  WHERE object_id=? AND object_type=1 AND cnt > 0`, oid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// 带下标的并发跑批（用于 like/unlike 交替场景）
func runConcurrentI(total int, fn func(i int) error) result {
	res := result{requests: total}
	var errs int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	t0 := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for attempt := 0; attempt < 3; attempt++ {
				err := fn(i)
				if err == nil {
					atomic.AddInt64(&res.success, 1)
					return
				}
				if strings.Contains(err.Error(), "Deadlock") || strings.Contains(err.Error(), "1213") {
					continue
				}
				atomic.AddInt64(&errs, 1)
				return
			}
			atomic.AddInt64(&errs, 1)
		}(i)
	}
	close(start)
	wg.Wait()
	res.elapsed = time.Since(t0)
	res.errs = errs
	return res
}

// 错误版：先 SELECT 再 INSERT，计数无条件 +1
func badLike(db *sql.DB, uid, oid uint64) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM like_state_bad WHERE user_id=? AND object_id=? AND action=1`,
		uid, oid).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		res, err := db.ExecContext(ctx,
			`INSERT INTO like_state_bad (user_id, object_id, action) VALUES (?, ?, 1)`, uid, oid)
		if err != nil {
			return err
		}
		if aff, _ := res.RowsAffected(); aff == 1 {
			atomic.AddInt64(&insertedRows, 1)
		}
	}
	_, err := db.ExecContext(ctx,
		`UPDATE like_counter_bad SET cnt = cnt + 1 WHERE object_id = ?`, oid)
	return err
}

func runConcurrent(db *sql.DB, total int, fn func(uid, oid uint64) error, uidFn func(i int) uint64, oid uint64) result {
	res := result{requests: total}
	var inserted, errs int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	var once sync.Once
	t0 := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // 让所有 goroutine 同时起跑
			uid := uidFn(i)
			for attempt := 0; attempt < 3; attempt++ {
				err := fn(uid, oid)
				if err == nil {
					atomic.AddInt64(&res.success, 1)
					return
				}
				if strings.Contains(err.Error(), "Deadlock") || strings.Contains(err.Error(), "1213") {
					continue
				}
				atomic.AddInt64(&errs, 1)
				return
			}
			atomic.AddInt64(&errs, 1)
		}(i)
	}
	once.Do(func() { close(start) })
	wg.Wait()
	res.elapsed = time.Since(t0)
	res.inserted = inserted
	res.errs = errs
	return res
}

func main() {
	db, err := sql.Open("mysql", dsn)
	must(err)
	defer db.Close()
	db.SetMaxOpenConns(64)
	db.SetMaxIdleConns(64)
	if err := db.PingContext(ctx); err != nil {
		panic(err)
	}
	setup(db)
	reset(db)

	// ---- 场景 1a：同一用户重复点赞 100 次 ----
	atomic.StoreInt64(&insertedRows, 0)
	seedCounter(db, "like_counter", 1001)
	t0 := time.Now()
	r1a := runConcurrent(db, 100, func(uid, oid uint64) error { return goodLike(db, uid, oid) }, func(i int) uint64 { return 7 }, 1001)
	r1a.elapsed = time.Since(t0)
	r1a.inserted = atomic.LoadInt64(&insertedRows)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state WHERE object_id=1001`).Scan(&r1a.rowCount)
	db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM like_state WHERE object_id=1001`).Scan(&r1a.distinct)
	db.QueryRowContext(ctx, `SELECT cnt FROM like_counter WHERE object_id=1001`).Scan(&r1a.counter)

	// ---- 场景 1b：1000 个不同用户点赞同一内容 ----
	reset(db)
	atomic.StoreInt64(&insertedRows, 0)
	seedCounter(db, "like_counter", 1002)
	t0 = time.Now()
	r1b := runConcurrent(db, 1000, func(uid, oid uint64) error { return goodLike(db, uid, oid) }, func(i int) uint64 { return uint64(10000 + i) }, 1002)
	r1b.elapsed = time.Since(t0)
	r1b.inserted = atomic.LoadInt64(&insertedRows)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state WHERE object_id=1002`).Scan(&r1b.rowCount)
	db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM like_state WHERE object_id=1002`).Scan(&r1b.distinct)
	db.QueryRowContext(ctx, `SELECT cnt FROM like_counter WHERE object_id=1002`).Scan(&r1b.counter)

	// ---- 场景 2a：错误版，同一用户重复点赞 100 次 ----
	reset(db)
	atomic.StoreInt64(&insertedRows, 0)
	seedCounter(db, "like_counter_bad", 2001)
	bad2a := make([]result, 0, 3)
	for round := 0; round < 3; round++ {
		db.ExecContext(ctx, `DELETE FROM like_state_bad`)
		db.ExecContext(ctx, `UPDATE like_counter_bad SET cnt=0 WHERE object_id=2001`)
		atomic.StoreInt64(&insertedRows, 0)
		t0 = time.Now()
		r := runConcurrent(db, 100, func(uid, oid uint64) error { return badLike(db, uid, oid) }, func(i int) uint64 { return 7 }, 2001)
		r.elapsed = time.Since(t0)
		r.inserted = atomic.LoadInt64(&insertedRows)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state_bad WHERE object_id=2001`).Scan(&r.rowCount)
		db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM like_state_bad WHERE object_id=2001`).Scan(&r.distinct)
		db.QueryRowContext(ctx, `SELECT cnt FROM like_counter_bad WHERE object_id=2001`).Scan(&r.counter)
		r.duplicates = r.rowCount - r.distinct
		bad2a = append(bad2a, r)
	}

	// ---- 场景 2b：错误版，1000 个用户各发 5 次并发请求（真相 = 1000） ----
	db.ExecContext(ctx, `DELETE FROM like_state_bad`)
	seedCounter(db, "like_counter_bad", 2002)
	atomic.StoreInt64(&insertedRows, 0)
	t0 = time.Now()
	r2b := runConcurrent(db, 5000, func(uid, oid uint64) error { return badLike(db, uid, oid) }, func(i int) uint64 { return uint64(30000 + i/5) }, 2002)
	r2b.elapsed = time.Since(t0)
	r2b.inserted = atomic.LoadInt64(&insertedRows)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state_bad WHERE object_id=2002`).Scan(&r2b.rowCount)
	db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM like_state_bad WHERE object_id=2002`).Scan(&r2b.distinct)
	db.QueryRowContext(ctx, `SELECT cnt FROM like_counter_bad WHERE object_id=2002`).Scan(&r2b.counter)

	// ---- 场景 3a：like → unlike → like，同一用户同一对象 ----
	reset(db)
	seedCounter(db, "like_counter", 3001)
	type stepStat struct {
		step   string
		rows   int64
		state  int64
		cnt    int64
	}
	var seq []stepStat
	read := func(step string) {
		var rows, state, cnt int64
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state WHERE object_id=3001`).Scan(&rows)
		db.QueryRowContext(ctx, `SELECT COALESCE(MAX(state),-1) FROM like_state WHERE object_id=3001`).Scan(&state)
		db.QueryRowContext(ctx, `SELECT cnt FROM like_counter WHERE object_id=3001`).Scan(&cnt)
		seq = append(seq, stepStat{step, rows, state, cnt})
	}
	if err := goodLike(db, 7, 3001); err != nil {
		panic(err)
	}
	read("like")
	if err := goodUnlike(db, 7, 3001); err != nil {
		panic(err)
	}
	read("unlike")
	if err := goodLike(db, 7, 3001); err != nil {
		panic(err)
	}
	read("re-like")

	// ---- 场景 3b：100 并发 like/unlike 交替（100 次请求，同一用户同一对象） ----
	reset(db)
	seedCounter(db, "like_counter", 3002)
	t0 = time.Now()
	mix := runConcurrentI(100, func(i int) error {
		if i%2 == 0 {
			return goodLike(db, 7, 3002)
		}
		return goodUnlike(db, 7, 3002)
	})
	mix.elapsed = time.Since(t0)
	var mixRows, mixState, mixCnt int64
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM like_state WHERE object_id=3002`).Scan(&mixRows)
	db.QueryRowContext(ctx, `SELECT COALESCE(MAX(state),-1) FROM like_state WHERE object_id=3002`).Scan(&mixState)
	db.QueryRowContext(ctx, `SELECT cnt FROM like_counter WHERE object_id=3002`).Scan(&mixCnt)

	// ---- 输出 ----
	println("== MySQL 26.7.0 / db=ch70_4 / InnoDB / 连接池 64 / 1000 并发 goroutine ==")
	println()
	println("[正确版] 唯一键 uk(user_id, object_id, action) + ON DUPLICATE KEY UPDATE，aff1=新插入/aff2=重新激活才 UPDATE 计数")
	println()
	fmt.Println("[1a] 同一用户(user=7) 重复点赞 100 次，object=1001")
	fmt.Printf("  requests=%d  success=%d  errors=%d  elapsed=%v\n", r1a.requests, r1a.success, r1a.errs, r1a.elapsed.Round(time.Millisecond))
	fmt.Printf("  INSERT 行数=%d   COUNT(*) 行数=%d   去重用户对数=%d   like_counter.cnt=%d\n", r1a.inserted, r1a.rowCount, r1a.distinct, r1a.counter)
	fmt.Printf("  判定: 状态行=%d (期望 1) %v; 计数=%d (期望 1) %v\n",
		r1a.rowCount, verdict(r1a.rowCount == 1), r1a.counter, verdict(r1a.counter == 1))
	fmt.Println()
	fmt.Println("[1b] 1000 个不同用户各点赞 1 次，object=1002")
	fmt.Printf("  requests=%d  success=%d  errors=%d  elapsed=%v\n", r1b.requests, r1b.success, r1b.errs, r1b.elapsed.Round(time.Millisecond))
	fmt.Printf("  INSERT 行数=%d   COUNT(*) 行数=%d   去重用户对数=%d   like_counter.cnt=%d\n", r1b.inserted, r1b.rowCount, r1b.distinct, r1b.counter)
	fmt.Printf("  判定: 状态行=%d (期望 1000) %v; 计数=%d (期望 1000) %v\n",
		r1b.rowCount, verdict(r1b.rowCount == 1000), r1b.counter, verdict(r1b.counter == 1000))
	fmt.Println()

	println("[错误版] 先 SELECT COUNT(*) 再 INSERT（无唯一键兜底），计数无条件 +1")
	println()
	fmt.Println("[2a] 同一用户(user=7) 重复点赞 100 次，object=2001，跑 3 轮")
	for i, r := range bad2a {
		fmt.Printf("  round%d: requests=%d success=%d errors=%d elapsed=%v INSERT行=%d COUNT(*)行=%d 去重用户对=%d 重复行=%d 计数=%d (期望 1)\n",
			i+1, r.requests, r.success, r.errs, r.elapsed.Round(time.Millisecond), r.inserted, r.rowCount, r.distinct, r.duplicates, r.counter)
	}
	fmt.Println()
	fmt.Println("[2b] 1000 个用户各发 5 次并发请求，object=2002")
	fmt.Printf("  requests=%d success=%d errors=%d elapsed=%v\n", r2b.requests, r2b.success, r2b.errs, r2b.elapsed.Round(time.Millisecond))
	fmt.Printf("  INSERT行=%d  COUNT(*)行=%d  去重用户对=%d (期望 1000)  重复行=%d  计数=%d (期望 1000，超计数 %d)\n",
		r2b.inserted, r2b.rowCount, r2b.distinct, r2b.rowCount-r2b.distinct, r2b.counter, r2b.counter-1000)

	println("[正确版] 取消赞后再点赞（ON DUPLICATE KEY UPDATE 按 affected rows 判定）")
	println()
	fmt.Println("[3a] like → unlike → like，object=3001，user=7")
	for _, st := range seq {
		fmt.Printf("  %-8s 状态行=%d  state=%d  计数=%d\n", st.step, st.rows, st.state, st.cnt)
	}
	fmt.Println()
	fmt.Println("[3b] 100 并发 like/unlike 交替，object=3002，user=7")
	fmt.Printf("  requests=%d success=%d errors=%d elapsed=%v\n", mix.requests, mix.success, mix.errs, mix.elapsed.Round(time.Millisecond))
	fmt.Printf("  状态行=%d  state=%d  计数=%d\n", mixRows, mixState, mixCnt)
	ok3b := mixRows == 1 && mixCnt >= 0 && ((mixState == 1 && mixCnt == 1) || (mixState == 0 && mixCnt == 0))
	fmt.Printf("  判定: 状态行=1 %v; 计数非负 %v; 计数与 state 一致 %v\n",
		verdict(mixRows == 1), verdict(mixCnt >= 0), verdict(ok3b))

	// 残留清理（自己的库自己的表）
	for _, q := range []string{`DELETE FROM like_state`, `DELETE FROM like_counter`,
		`DELETE FROM like_state_bad`, `DELETE FROM like_counter_bad`} {
		db.ExecContext(ctx, q)
	}
}

func verdict(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
