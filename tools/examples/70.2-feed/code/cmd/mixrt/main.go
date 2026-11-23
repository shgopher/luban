// 实测 C：4 路召回 1200 候选 → 过滤 → 混排 → 取 20 的 P99
// 同进程 A/B：① 串行召回 vs 并行召回 ② 全量混排 vs 各路先截断再混排 ③ 缓存命中路径
// 只测 CPU 与内存分配，不含网络 RTT（网络部分是 34.1 的 P99 口径）。
// 运行：go run ./cmd/mixrt
package main

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const (
	globalN  = 6000 // 全局内容正排条数
	authors  = 300
	pageMax  = 20
	iters    = 10000
	rrfK     = 60.0
	truncTop = 100 // 截断方案：每路只留前 100 条进混排
	vecLen   = 128 // 推荐路向量维度
)

type cand struct {
	id     int64
	author int64
	score  float64 // 已预乘：路权重 /(k+rank)
}

type channel struct {
	name   string
	weight float64
	take   int
	index  []cand
}

type userCtx struct {
	id      int64
	read    map[int64]bool
	blocked map[int64]bool
	vec     []float32 // 用户兴趣向量，模拟推荐路打分输入（模拟推荐路的打分输入）
}

type itemStatus uint8

const (
	statusOK itemStatus = iota
	statusReview
	statusRemoved
)

var (
	channels []*channel
	statuses []itemStatus
	users    []*userCtx
	itemVec  [][]float32 // 32 维内容向量，下标 = item id
)

type stats struct {
	total    []time.Duration
	recall   []time.Duration
	filterT  []time.Duration
	mixT     []time.Duration
	chanLat  [4][]time.Duration
	filtered [5]int64
	backfill int64
	top      [pageMax]int64
	nTop     int
}

func main() {
	buildData()
	total := channels[0].take + channels[1].take + channels[2].take + channels[3].take
	fmt.Println("实测 C：Feed 出流 CPU 段（多路召回 → 过滤 → 混排 → 取 20）的 P50/P99")
	fmt.Println("环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21；同机 4 个写作任务并行时测得；单进程，不含网络 RTT")
	fmt.Printf("数据：关注 %d + 推荐 %d + 热门 %d + 同城 %d = %d 个候选槽位；全局正排 %d 条；用户上下文 %d 个\n",
		channels[0].take, channels[1].take, channels[2].take, channels[3].take, total, globalN, len(users))
	fmt.Printf("装配：RRF k=%.0f，权重 %.1f/%.1f/%.1f/%.1f；过滤=已读/拉黑/审核态/自己/跨路重复；混排=同作者最多 2 条；取 %d 条；每方案 %d 次\n\n",
		rrfK, channels[0].weight, channels[1].weight, channels[2].weight, channels[3].weight, pageMax, iters)

	serial := bench(false, false, false)
	parallel := bench(true, false, false)
	trunc := bench(true, true, false)
	cache := bench(false, false, true)
	overlapIdentical, overlapAvg := quality()
	light, lightCh := recallAB(false, false)
	lp, _ := recallAB(true, false)
	heavy, heavyCh := recallAB(false, true)
	hp, _ := recallAB(true, true)

	fmt.Println("| 方案 | 召回 P50 | 召回 P99 | 过滤 P99 | 混排 P99 | 总 P50 | 总 P99 | 与全量混排 top20 对比 |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	row := func(name string, s *stats, q string) {
		fmt.Printf("| %s | %v | %v | %v | %v | %v | %v | %s |\n",
			name, pct(s.recall, 50), pct(s.recall, 99), pct(s.filterT, 99), pct(s.mixT, 99),
			pct(s.total, 50), pct(s.total, 99), q)
	}
	row("① 串行召回 + 全量混排（基线）", serial, "基线")
	row("② 并行召回 + 全量混排", parallel, "基线")
	row("③ 并行召回 + 各路截断到 100 条", trunc, fmt.Sprintf("完全一致 %.1f%%，平均重合 %.2f 条", float64(overlapIdentical)*100/iters, overlapAvg))
	row("④ 本地缓存命中（TTL 5 分钟）", cache, "直接返回上次列表")

	fmt.Println()
	fmt.Println("召回段单开 A/B：并行化到底划不划算（只测召回段，1 万次样本；单路取最慢那一通道）")
	fmt.Println("| 召回实现 | 最慢单路 P50 | 串行召回段 P50 | 串行召回段 P99 | 并行召回段 P50 | 并行召回段 P99 | 并行 P99 变化 |")
	fmt.Println("|---|---|---|---|---|---|---|")
	fmt.Printf("| 轻：纯内存搬运 | %v | %v | %v | %v | %v | %+.0f%% |\n",
		slowestP50(lightCh), pct(light, 50), pct(light, 99), pct(lp, 50), pct(lp, 99),
		(float64(pct(lp, 99))-float64(pct(light, 99)))/float64(pct(light, 99))*100)
	fmt.Printf("| 重：每路 %d 维向量打分 | %v | %v | %v | %v | %v | %+.0f%% |\n",
		vecLen, slowestP50(heavyCh), pct(heavy, 50), pct(heavy, 99), pct(hp, 50), pct(hp, 99),
		(float64(pct(hp, 99))-float64(pct(heavy, 99)))/float64(pct(heavy, 99))*100)

	fmt.Println()
	fmt.Println("各路召回耗时（方案②，单路 1 万次样本）：")
	fmt.Println("| 通道 | 候选数 | P50 | P99 |")
	fmt.Println("|---|---|---|---|")
	for i, c := range channels {
		fmt.Printf("| %s | %d | %v | %v |\n", c.name, c.take, pct(parallel.chanLat[i], 50), pct(parallel.chanLat[i], 99))
	}

	fmt.Println()
	fmt.Println("过滤层逐项读数（方案②，1 万次累计）：")
	fmt.Println("| 过滤原因 | 累计剔除条数 | 平均每次剔除 |")
	fmt.Println("|---|---|---|")
	for i, n := range []string{"已读", "拉黑作者", "审核中/已下架", "自己发的", "跨路重复"} {
		fmt.Printf("| %s | %d | %.1f |\n", n, parallel.filtered[i], float64(parallel.filtered[i])/iters)
	}
	fmt.Printf("| 过滤+限额后不足 20 条（触发回填） | %d 次 | %.2f%% |\n", parallel.backfill, float64(parallel.backfill)*100/iters)

	fmt.Println()
	fmt.Println("质量对账：全量混排 vs 各路截断到 100 条，top20 逐位比对（1 万次）")
	fmt.Printf("  top20 完全一致 = %d / %d (%.1f%%)；平均重合 %.2f / 20 条\n",
		overlapIdentical, iters, float64(overlapIdentical)*100/iters, overlapAvg)
}

// ---------- 数据 ----------

func buildData() {
	r := rand.New(rand.NewSource(20261121))
	statuses = make([]itemStatus, globalN+1)
	for i := 1; i <= globalN; i++ {
		switch {
		case r.Intn(100) < 3:
			statuses[i] = statusReview
		case r.Intn(1000) < 3:
			statuses[i] = statusRemoved
		}
	}
	specs := []struct {
		name string
		w    float64
		take int
	}{{"关注", 1.0, 300}, {"推荐", 0.8, 400}, {"热门", 0.6, 250}, {"同城", 0.5, 250}}
	for _, sp := range specs {
		idx := make([]cand, 0, globalN)
		for i := 1; i <= globalN; i++ {
			idx = append(idx, cand{id: int64(i), author: int64(1 + (i-1)%authors),
				score: float64(r.Intn(1_000_000)) / 1000.0})
		}
		sort.Slice(idx, func(a, b int) bool { return idx[a].score > idx[b].score })
		// 把索引分数换成 RRF 累积分：权重 /(k+rank)
		for rank := range idx {
			idx[rank].score = sp.w / (rrfK + float64(rank+1))
		}
		channels = append(channels, &channel{name: sp.name, weight: sp.w, take: sp.take, index: idx})
	}
	for u := 0; u < 100; u++ {
		uc := &userCtx{id: int64(u + 1), read: map[int64]bool{}, blocked: map[int64]bool{}}
		for k := 0; k < 800; k++ {
			uc.read[int64(1+r.Intn(globalN))] = true
		}
		for k := 0; k < 40; k++ {
			uc.blocked[int64(1+r.Intn(authors))] = true
		}
		uc.vec = make([]float32, vecLen)
		for d := range uc.vec {
			uc.vec[d] = float32(r.Intn(2000)-1000) / 1000
		}
		users = append(users, uc)
	}
	itemVec = make([][]float32, globalN+1)
	for i := 1; i <= globalN; i++ {
		v := make([]float32, vecLen)
		for d := range v {
			v[d] = float32(r.Intn(2000)-1000) / 1000
		}
		itemVec[i] = v
	}
}

// ---------- 单次请求 ----------

type request struct {
	cands []cand
	keep  []cand
}

func newRequest() *request {
	return &request{cands: make([]cand, 0, 1200), keep: make([]cand, 0, 1200)}
}

// recall：按用户偏移从各路索引里搬出 take 条候选
func (rq *request) recall(u *userCtx, parallel bool, truncated, heavy bool, st *stats) {
	rq.cands = rq.cands[:0]
	wg := sync.WaitGroup{}
	out := make([][]cand, len(channels))
	for ci, c := range channels {
		take := c.take
		if truncated && take > truncTop {
			take = truncTop
		}
		off := int(u.id) * 37 % (globalN / 2)
		buf := make([]cand, take)
		out[ci] = buf
		fetch := func(ci int, c *channel, take, off int, buf []cand) {
			for i := 0; i < take; i++ {
				buf[i] = c.index[(off+i)%len(c.index)]
			}
			if heavy {
				// 推荐路的真实成本：对候选做向量打分（1200 候选 × vecLen 次乘加）
				for i := range buf {
					v := itemVec[buf[i].id]
					var dot float32
					for d := range v {
						dot += v[d] * u.vec[d]
					}
					buf[i].score += float64(dot) * 1e-6
				}
			}
		}
		if parallel {
			wg.Add(1)
			go func(ci int, c *channel, take, off int, buf []cand) {
				defer wg.Done()
				t0 := time.Now()
				fetch(ci, c, take, off, buf)
				if st != nil {
					st.chanLat[ci] = append(st.chanLat[ci], time.Since(t0))
				}
			}(ci, c, take, off, buf)
		} else {
			t0 := time.Now()
			fetch(ci, c, take, off, buf)
			if st != nil {
				st.chanLat[ci] = append(st.chanLat[ci], time.Since(t0))
			}
		}
	}
	if parallel {
		wg.Wait()
	}
	for _, buf := range out {
		rq.cands = append(rq.cands, buf...)
	}
}

// filter：已读 / 拉黑 / 审核态 / 自己 / 跨路重复
func (rq *request) filter(u *userCtx, fc *[5]int64) {
	rq.keep = rq.keep[:0]
	seen := make(map[int64]bool, len(rq.cands))
	for _, c := range rq.cands {
		if seen[c.id] {
			if fc != nil {
				fc[4]++
			}
			continue
		}
		seen[c.id] = true
		switch {
		case u.read[c.id]:
			if fc != nil {
				fc[0]++
			}
		case u.blocked[c.author]:
			if fc != nil {
				fc[1]++
			}
		case statuses[c.id] != statusOK:
			if fc != nil {
				fc[2]++
			}
		case c.author == u.id:
			if fc != nil {
				fc[3]++
			}
		default:
			rq.keep = append(rq.keep, c)
		}
	}
}

// mix：累加 RRF 分 → 排序 → 同作者限 2 条 → 取 20 → 不足回填
func (rq *request) mix(u *userCtx, top []int64, bf *int64) int {
	score := make(map[int64]float64, len(rq.keep))
	author := make(map[int64]int64, len(rq.keep))
	for _, c := range rq.keep {
		score[c.id] += c.score
		author[c.id] = c.author
	}
	type kv struct {
		id int64
		s  float64
	}
	list := make([]kv, 0, len(score))
	for id, s := range score {
		list = append(list, kv{id, s})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].s > list[j].s })
	per := map[int64]int{}
	n := 0
	for _, e := range list {
		if per[author[e.id]] >= 2 {
			continue
		}
		per[author[e.id]]++
		top[n] = e.id
		n++
		if n == pageMax {
			return n
		}
	}
	if n < pageMax && bf != nil {
		(*bf)++
	}
	for _, e := range list {
		dup := false
		for k := 0; k < n; k++ {
			if top[k] == e.id {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		top[n] = e.id
		n++
		if n == pageMax {
			break
		}
	}
	return n
}

// do：一次完整出流
func do(rq *request, u *userCtx, parallel, truncated bool, st *stats) int {
	t0 := time.Now()
	p := time.Now()
	rq.recall(u, parallel, truncated, false, st)
	if st != nil {
		st.recall = append(st.recall, time.Since(p))
	}
	p = time.Now()
	rq.filter(u, &st.filtered)
	st.filterT = append(st.filterT, time.Since(p))
	p = time.Now()
	n := rq.mix(u, st.top[:], &st.backfill)
	st.mixT = append(st.mixT, time.Since(p))
	st.total = append(st.total, time.Since(t0))
	st.nTop = n
	return n
}

func bench(parallel, truncated, cacheHit bool) *stats {
	st := &stats{}
	rq := newRequest()
	cache := map[int64][pageMax]int64{}
	for i := 0; i < iters; i++ {
		u := users[i%len(users)]
		t0 := time.Now()
		if cacheHit {
			v := cache[u.id]
			st.top = v
			st.total = append(st.total, time.Since(t0))
			continue
		}
		do(rq, u, parallel, truncated, st)
		cache[u.id] = st.top
	}
	return st
}

// quality：同一用户同一轮，全量混排 vs 截断混排 的 top20 对比
func quality() (identical int, avgOverlap float64) {
	rqA, rqB := newRequest(), newRequest()
	var a, b [pageMax]int64
	sum := 0
	for i := 0; i < iters; i++ {
		u := users[i%len(users)]
		na := doTop(rqA, u, false, a[:])
		nb := doTop(rqB, u, true, b[:])
		identicalThis := na == nb && na == pageMax
		for k := 0; k < na && k < nb && k < pageMax; k++ {
			if a[k] != b[k] {
				identicalThis = false
				break
			}
		}
		if identicalThis {
			identical++
		}
		set := map[int64]bool{}
		for k := 0; k < na; k++ {
			set[a[k]] = true
		}
		hit := 0
		for k := 0; k < nb; k++ {
			if set[b[k]] {
				hit++
			}
		}
		sum += hit
	}
	return identical, float64(sum) / iters
}

// recallAB：只测召回段的串行/并行差异，同时记录每路耗时
func recallAB(parallel, heavy bool) ([]time.Duration, [4][]time.Duration) {
	rq := newRequest()
	st := &stats{}
	out := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		u := users[i%len(users)]
		t0 := time.Now()
		rq.recall(u, parallel, false, heavy, st)
		out = append(out, time.Since(t0))
	}
	return out, st.chanLat
}

func slowestP50(lat [4][]time.Duration) time.Duration {
	var mx time.Duration
	for i := range lat {
		if d := pct(lat[i], 50); d > mx {
			mx = d
		}
	}
	return mx
}

func doTop(rq *request, u *userCtx, truncated bool, top []int64) int {
	rq.recall(u, false, truncated, false, nil)
	rq.filter(u, nil)
	return rq.mix(u, top, nil)
}

// ---------- 工具 ----------

func pct(s []time.Duration, p int) time.Duration {
	if len(s) == 0 {
		return 0
	}
	c := append([]time.Duration(nil), s...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	idx := len(c) * p / 100
	if idx >= len(c) {
		idx = len(c) - 1
	}
	return c[idx]
}
