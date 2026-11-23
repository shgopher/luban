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

const segSize = 500

type seg struct {
	UID       int
	SegID     int
	StartTS   int64
	EndTS     int64
	Count     int
	MinLng    int
	MaxLng    int
	MinLat    int
	MaxLat    int
	GH6       string
	DistanceM float64
	Blob      []byte
}

func qInt(v float64) int { return int(v * 1e7) }

type qres struct {
	dur    time.Duration
	rows   int // X: 返回行数；Y: 段数
	points int
}

func pct(rs []qres, p float64) time.Duration {
	ds := make([]time.Duration, len(rs))
	for i := range rs {
		ds[i] = rs[i].dur
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	idx := int(float64(len(ds))*p/100) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ds) {
		idx = len(ds) - 1
	}
	return ds[idx]
}

func avgInt(rs []qres, f func(qres) int) float64 {
	s := 0
	for _, r := range rs {
		s += f(r)
	}
	return float64(s) / float64(len(rs))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func expC() {
	db, err := sql.Open("mysql", "root:@tcp(127.0.0.1:16306)/?charset=utf8mb4")
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	must := func(s string) {
		if _, err := db.Exec(s); err != nil {
			panic(fmt.Sprintf("%s: %v", s, err))
		}
	}
	must("DROP DATABASE IF EXISTS ch71_track")
	must("CREATE DATABASE ch71_track")
	must("USE ch71_track")
	must(`CREATE TABLE track_x (
		uid INT NOT NULL, ts BIGINT NOT NULL,
		lng INT NOT NULL, lat INT NOT NULL, gh6 CHAR(6) NOT NULL,
		PRIMARY KEY (uid, ts)) ENGINE=InnoDB`)
	must(`CREATE TABLE track_seg (
		uid INT NOT NULL, seg_id INT NOT NULL,
		start_ts BIGINT NOT NULL, end_ts BIGINT NOT NULL,
		point_count SMALLINT NOT NULL,
		bbox_min_lng INT NOT NULL, bbox_max_lng INT NOT NULL,
		bbox_min_lat INT NOT NULL, bbox_max_lat INT NOT NULL,
		gh6 CHAR(6) NOT NULL, distance_m INT NOT NULL, points_blob BLOB NOT NULL,
		PRIMARY KEY (uid, seg_id), KEY idx_uid_start (uid, start_ts)) ENGINE=InnoDB`)
	must(`CREATE TABLE seg_cells (
		gh6 CHAR(6) NOT NULL, uid INT NOT NULL, seg_id INT NOT NULL,
		start_ts BIGINT NOT NULL, end_ts BIGINT NOT NULL,
		KEY idx_gh (gh6, start_ts)) ENGINE=InnoDB`)

	var bp int
	_ = db.QueryRow("SELECT @@innodb_buffer_pool_size").Scan(&bp)
	fmt.Printf("C env: MySQL 26.7.0 @127.0.0.1:16306, innodb_buffer_pool_size=%d MiB\n", bp/1024/1024)

	const users, perUser = 100, 10000
	r := rand.New(rand.NewSource(2026))
	all := make([][]Pt, users)
	userStart := make([]int64, users)
	for u := 0; u < users; u++ {
		slng := baseLng + (r.Float64()*2-1)*8000/(111320*0.766)
		slat := baseLat + (r.Float64()*2-1)*8000/110540
		all[u] = genTrajectory(perUser, int64(1000+u), slng, slat, 3000, 3)
		userStart[u] = all[u][0].TS
	}
	fmt.Printf("dataset: users=%d points/user=%d total=%d geohash_level=6\n", users, perUser, users*perUser)

	// X：每点一行，批量 500
	t0 := time.Now()
	tx, _ := db.Begin()
	vals := make([]string, 0, segSize)
	args := make([]any, 0, segSize*5)
	flush := func() {
		if len(vals) == 0 {
			return
		}
		q := "INSERT INTO track_x (uid,ts,lng,lat,gh6) VALUES " + strings.Join(vals, ",")
		if _, err := tx.Exec(q, args...); err != nil {
			panic(err)
		}
		vals, args = vals[:0], args[:0]
	}
	for u := 0; u < users; u++ {
		for _, p := range all[u] {
			vals = append(vals, "(?,?,?,?,?)")
			args = append(args, u, p.TS, qInt(p.Lng), qInt(p.Lat), geohashEncode(p.Lng, p.Lat, 6))
			if len(vals) == segSize {
				flush()
			}
		}
	}
	flush()
	if err := tx.Commit(); err != nil {
		panic(err)
	}
	fmt.Printf("X loaded %d rows in %.1f s (batch=500)\n", users*perUser, time.Since(t0).Seconds())

	// Y：500 点/段 → 2000 段；blob=delta+zigzag(1e-6)；seg_cells=bbox 覆盖格倒排
	segs := make([]seg, 0, users*perUser/segSize)
	t1 := time.Now()
	for u := 0; u < users; u++ {
		pts := all[u]
		for sid := 0; sid*segSize < len(pts); sid++ {
			a := sid * segSize
			b := a + segSize
			if b > len(pts) {
				b = len(pts)
			}
			sub := pts[a:b]
			s := seg{UID: u, SegID: sid, StartTS: sub[0].TS, EndTS: sub[len(sub)-1].TS, Count: len(sub)}
			s.MinLng, s.MaxLng = qInt(sub[0].Lng), qInt(sub[0].Lng)
			s.MinLat, s.MaxLat = qInt(sub[0].Lat), qInt(sub[0].Lat)
			for _, p := range sub {
				if qInt(p.Lng) < s.MinLng {
					s.MinLng = qInt(p.Lng)
				}
				if qInt(p.Lng) > s.MaxLng {
					s.MaxLng = qInt(p.Lng)
				}
				if qInt(p.Lat) < s.MinLat {
					s.MinLat = qInt(p.Lat)
				}
				if qInt(p.Lat) > s.MaxLat {
					s.MaxLat = qInt(p.Lat)
				}
			}
			s.GH6 = geohashEncode(sub[0].Lng, sub[0].Lat, 6)
			s.DistanceM = mileage(sub)
			s.Blob = encDelta(sub, 1e7)
			segs = append(segs, s)
		}
	}
	tx2, _ := db.Begin()
	nCells := 0
	for _, s := range segs {
		if _, err := tx2.Exec(`INSERT INTO track_seg VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.UID, s.SegID, s.StartTS, s.EndTS, s.Count, s.MinLng, s.MaxLng, s.MinLat, s.MaxLat, s.GH6, int(s.DistanceM), s.Blob); err != nil {
			panic(err)
		}
		for _, cell := range bboxCells(s) {
			if _, err := tx2.Exec(`INSERT INTO seg_cells VALUES (?,?,?,?,?)`, cell, s.UID, s.SegID, s.StartTS, s.EndTS); err != nil {
				panic(err)
			}
			nCells++
		}
	}
	if err := tx2.Commit(); err != nil {
		panic(err)
	}
	fmt.Printf("Y loaded %d segments (%.1f s) + seg_cells %d rows\n", len(segs), time.Since(t1).Seconds(), nCells)
	var blobSum, blobMin, blobMax int
	blobMin = 1 << 30
	for _, s := range segs {
		blobSum += len(s.Blob)
		if len(s.Blob) < blobMin {
			blobMin = len(s.Blob)
		}
		if len(s.Blob) > blobMax {
			blobMax = len(s.Blob)
		}
	}
	fmt.Printf("Y blob: avg=%.1f B/段 min=%d max=%d -> %.2f B/点\n",
		float64(blobSum)/float64(len(segs)), blobMin, blobMax, float64(blobSum)/float64(users*perUser))

	must("ALTER TABLE track_x ADD INDEX idx_gh_ts (gh6, ts)")
	must("ANALYZE TABLE track_x, track_seg, seg_cells")
	for _, tbl := range []string{"track_x", "track_seg", "seg_cells"} {
		var dataLen, idxLen, rows int64
		_ = db.QueryRow(`SELECT data_length, index_length, table_rows FROM information_schema.tables WHERE table_schema='ch71_track' AND table_name=?`, tbl).Scan(&dataLen, &idxLen, &rows)
		var exact int64
		_ = db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&exact)
		fmt.Printf("size %-9s rows=%-7d data=%.1f MiB index=%.1f MiB\n", tbl, exact, float64(dataLen)/1048576, float64(idxLen)/1048576)
	}

	// 预热：每类查询先各跑 10 次（排除首次冷读）
	for i := 0; i < 10; i++ {
		u := i
		h := int64((u * 7) % 8)
		lo, hi := userStart[u]+h*3600, userStart[u]+h*3600+3600
		rows, _ := db.Query("SELECT ts,lng,lat FROM track_x WHERE uid=? AND ts>=? AND ts<? ORDER BY ts", u, lo, hi)
		for rows.Next() {
			var a int64
			var b, c int
			_ = rows.Scan(&a, &b, &c)
		}
		rows.Close()
		rows, _ = db.Query("SELECT points_blob FROM track_seg WHERE uid=? AND start_ts<? AND end_ts>? ORDER BY start_ts", u, hi, lo)
		for rows.Next() {
			var b []byte
			_ = rows.Scan(&b)
		}
		rows.Close()
		rows, _ = db.Query("SELECT ts,lng,lat FROM track_x WHERE uid=? AND ts<? ORDER BY ts", u, userStart[u]+40000)
		for rows.Next() {
			var a int64
			var b, c int
			_ = rows.Scan(&a, &b, &c)
		}
		rows.Close()
		rows, _ = db.Query("SELECT points_blob FROM track_seg WHERE uid=? AND start_ts<? ORDER BY start_ts", u, userStart[u]+40000)
		for rows.Next() {
			var b []byte
			_ = rows.Scan(&b)
		}
		rows.Close()
	}

	// ---- 查询 1：某用户某 1 小时 ----
	decHour := func(blob []byte, lo, hi int64) []Pt {
		pts := decDelta(blob, 1e7)
		out := pts[:0]
		for _, p := range pts {
			if p.TS >= lo && p.TS < hi {
				out = append(out, p)
			}
		}
		return out
	}
	q1x := make([]qres, 0, 100)
	for u := 0; u < 100; u++ {
		h := int64((u * 7) % 8)
		lo, hi := userStart[u]+h*3600, userStart[u]+h*3600+3600
		t := time.Now()
		rows, err := db.Query("SELECT ts,lng,lat FROM track_x WHERE uid=? AND ts>=? AND ts<? ORDER BY ts", u, lo, hi)
		if err != nil {
			panic(err)
		}
		n := 0
		for rows.Next() {
			var ts int64
			var lng, lat int
			_ = rows.Scan(&ts, &lng, &lat)
			n++
		}
		rows.Close()
		q1x = append(q1x, qres{time.Since(t), n, n})
	}
	q1y := make([]qres, 0, 100)
	for u := 0; u < 100; u++ {
		h := int64((u * 7) % 8)
		lo, hi := userStart[u]+h*3600, userStart[u]+h*3600+3600
		t := time.Now()
		rows, err := db.Query("SELECT points_blob FROM track_seg WHERE uid=? AND start_ts<? AND end_ts>? ORDER BY start_ts", u, hi, lo)
		if err != nil {
			panic(err)
		}
		cand, pts := 0, 0
		for rows.Next() {
			var blob []byte
			_ = rows.Scan(&blob)
			cand++
			pts += len(decHour(blob, lo, hi))
		}
		rows.Close()
		q1y = append(q1y, qres{time.Since(t), cand, pts})
	}
	fmt.Printf("q1 X rows=%.0f p50=%.2fms p99=%.2fms returned=%.0f payload=%.0fB\n",
		avgInt(q1x, func(r qres) int { return r.rows }), ms(pct(q1x, 50)), ms(pct(q1x, 99)), avgInt(q1x, func(r qres) int { return r.points }), avgInt(q1x, func(r qres) int { return r.points })*16)
	fmt.Printf("q1 Y segs=%.1f p50=%.2fms p99=%.2fms returned=%.0f decoded=%.0f payload=%.0fB\n",
		avgInt(q1y, func(r qres) int { return r.rows }), ms(pct(q1y, 50)), ms(pct(q1y, 99)),
		avgInt(q1y, func(r qres) int { return r.points }), avgInt(q1y, func(r qres) int { return r.rows })*500,
		avgInt(q1y, func(r qres) int { return r.points })*16)

	// ---- 查询 2：某 geohash 格 + 某 1 小时 ----
	type cellHour struct {
		cell string
		hi   int64
	}
	cand := map[cellHour]int{}
	for u := 0; u < users; u++ {
		for _, p := range all[u] {
			ch := cellHour{geohashEncode(p.Lng, p.Lat, 6), userStart[u] + ((p.TS-userStart[u])/3600)*3600}
			cand[ch]++
		}
	}
	keys := make([]cellHour, 0, len(cand))
	for k, v := range cand {
		if v >= 120 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].cell != keys[j].cell {
			return keys[i].cell < keys[j].cell
		}
		return keys[i].hi < keys[j].hi
	})
	r2 := rand.New(rand.NewSource(7))
	r2.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	if len(keys) > 100 {
		keys = keys[:100]
	}
	type q2case struct {
		cell   string
		lo, hi int64
	}
	cases := make([]q2case, 0, len(keys))
	for _, k := range keys {
		cases = append(cases, q2case{k.cell, k.hi, k.hi + 3600})
	}
	q2x := make([]qres, 0, len(cases))
	gt := make([]int, 0, len(cases))
	for _, c := range cases {
		t := time.Now()
		rows, err := db.Query("SELECT ts FROM track_x WHERE gh6=? AND ts>=? AND ts<?", c.cell, c.lo, c.hi)
		if err != nil {
			panic(err)
		}
		n := 0
		for rows.Next() {
			var ts int64
			_ = rows.Scan(&ts)
			n++
		}
		rows.Close()
		gt = append(gt, n)
		q2x = append(q2x, qres{time.Since(t), n, n})
	}
	fmt.Printf("q2 X returned=%.0f p50=%.2fms p99=%.2fms index_entries=%.0f (covering index 范围扫描=返回行数)\n",
		avgInt(q2x, func(r qres) int { return r.points }), ms(pct(q2x, 50)), ms(pct(q2x, 99)), avgInt(q2x, func(r qres) int { return r.rows }))

	q2y := make([]qres, 0, len(cases))
	var decodedAll, matchedAll, candSegAll int
	for _, c := range cases {
		t := time.Now()
		rows, err := db.Query(`SELECT s.points_blob FROM seg_cells c JOIN track_seg s ON s.uid=c.uid AND s.seg_id=c.seg_id
			WHERE c.gh6=? AND c.start_ts>=? AND c.start_ts<? AND c.end_ts>?`, c.cell, c.lo-3600, c.hi, c.lo)
		if err != nil {
			panic(err)
		}
		decoded, matched := 0, 0
		candSeg := 0
		for rows.Next() {
			var blob []byte
			_ = rows.Scan(&blob)
			candSeg++
			pts := decDelta(blob, 1e7)
			decoded += len(pts)
			for _, p := range pts {
				if p.TS < c.lo || p.TS >= c.hi {
					continue
				}
				if geohashEncode(p.Lng, p.Lat, 6) == c.cell {
					matched++
				}
			}
		}
		rows.Close()
		decodedAll += decoded
		matchedAll += matched
		candSegAll += candSeg
		q2y = append(q2y, qres{time.Since(t), candSeg, matched})
	}
	var gtTotal int
	for _, n := range gt {
		gtTotal += n
	}
	fmt.Printf("q2 Y cand_segs=%.1f p50=%.2fms p99=%.2fms decoded=%.0f returned=%.0f false_hit=%.1f%% amplification=%.1f recall=%.1f%%\n",
		float64(candSegAll)/float64(len(cases)), ms(pct(q2y, 50)), ms(pct(q2y, 99)),
		float64(decodedAll)/float64(len(cases)), avgInt(q2y, func(r qres) int { return r.points }),
		100*float64(decodedAll-matchedAll)/float64(decodedAll), float64(decodedAll)/float64(matchedAll),
		100*float64(matchedAll)/float64(gtTotal))

	// Y 变体：只有「段起点格」9 格索引，没有 bbox 倒排
	sHit, sGT := 0, 0
	for _, n := range gt {
		sGT += n
	}
	// 用 X 的真值分别求和
	for _, c := range cases {
		cells := geohashNeighbors9(c.cell)
		ph := make([]string, len(cells))
		args := make([]any, 0, len(cells)+2)
		for j, s := range cells {
			ph[j] = "?"
			args = append(args, s)
		}
		args = append(args, c.hi, c.lo)
		rows, err := db.Query("SELECT points_blob FROM track_seg WHERE gh6 IN ("+strings.Join(ph, ",")+") AND start_ts<? AND end_ts>?", args...)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var blob []byte
			_ = rows.Scan(&blob)
			for _, p := range decDelta(blob, 1e7) {
				if p.TS >= c.lo && p.TS < c.hi && geohashEncode(p.Lng, p.Lat, 6) == c.cell {
					sHit++
				}
			}
		}
		rows.Close()
	}
	fmt.Printf("q2 Y-start-cell-only recall=%d/%d=%.1f%%\n", sHit, sGT, 100*float64(sHit)/float64(sGT))

	// ---- 查询 3：回放 1 万点（单用户全天） ----
	q3x := make([]qres, 0, 100)
	for u := 0; u < 100; u++ {
		hi := userStart[u] + 40000
		t := time.Now()
		rows, err := db.Query("SELECT ts,lng,lat FROM track_x WHERE uid=? AND ts<? ORDER BY ts", u, hi)
		if err != nil {
			panic(err)
		}
		n := 0
		for rows.Next() {
			var ts int64
			var lng, lat int
			_ = rows.Scan(&ts, &lng, &lat)
			n++
		}
		rows.Close()
		q3x = append(q3x, qres{time.Since(t), n, n})
	}
	q3y := make([]qres, 0, 100)
	for u := 0; u < 100; u++ {
		hi := userStart[u] + 40000
		t := time.Now()
		rows, err := db.Query("SELECT points_blob FROM track_seg WHERE uid=? AND start_ts<? ORDER BY start_ts", u, hi)
		if err != nil {
			panic(err)
		}
		segsN, n := 0, 0
		for rows.Next() {
			var blob []byte
			_ = rows.Scan(&blob)
			segsN++
			n += len(decDelta(blob, 1e7))
		}
		rows.Close()
		q3y = append(q3y, qres{time.Since(t), segsN, n})
	}
	fmt.Printf("q3 X rows=%.0f p50=%.2fms p99=%.2fms returned=%.0f payload=%.0fB\n",
		avgInt(q3x, func(r qres) int { return r.rows }), ms(pct(q3x, 50)), ms(pct(q3x, 99)), avgInt(q3x, func(r qres) int { return r.points }), avgInt(q3x, func(r qres) int { return r.points })*16)
	blobAvg := float64(blobSum) / float64(len(segs))
	fmt.Printf("q3 Y segs=%.0f p50=%.2fms p99=%.2fms returned=%.0f payload=%.0fB (20 段 × %.0f B/段)\n",
		avgInt(q3y, func(r qres) int { return r.rows }), ms(pct(q3y, 50)), ms(pct(q3y, 99)), avgInt(q3y, func(r qres) int { return r.points }),
		avgInt(q3y, func(r qres) int { return r.rows })*blobAvg, blobAvg)
}

func bboxCells(s seg) []string {
	var out []string
	seen := map[string]bool{}
	stepLat := 0.0025
	stepLng := 0.005
	for lat := float64(s.MinLat)/1e7 - stepLat; lat <= float64(s.MaxLat)/1e7+stepLat; lat += stepLat {
		for lng := float64(s.MinLng)/1e7 - stepLng; lng <= float64(s.MaxLng)/1e7+stepLng; lng += stepLng {
			c := geohashEncode(lng, lat, 6)
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
