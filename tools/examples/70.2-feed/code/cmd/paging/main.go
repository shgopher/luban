// 实测 A：同进程 A/B —— offset 分页 vs (score,id) 双游标分页
// 场景：翻页过程中持续发布新内容（头插）、审核下架/删帖（删）、热度重排（改分数）
// 输出：页数、返回槽位、唯一条数、重复条数、漏条数、扫描行数
//
// 运行：go run ./cmd/paging
package main

import (
	"fmt"
	"math/rand"
	"sort"
)

type item struct {
	id    int64
	score int64 // 毫秒时间戳，发布时冻结；Feed 的游标键必须单调且不变
	born  int   // 0 = 翻页开始前就存在；k = 第 k 页时新插入
}

type feed struct {
	items   []item // 始终按 (score desc, id desc) 有序
	clock   int64
	nextID  int64
	maxScor int64
}

func newFeed(n int, seed int64) *feed {
	r := rand.New(rand.NewSource(seed))
	f := &feed{clock: 1_700_000_000_000}
	base := make([]item, 0, n)
	id := int64(1)
	for i := 0; i < n; i++ {
		// 初始内容：最近 2 小时，分数=发布毫秒时间戳
		sc := f.clock - int64(r.Intn(7_200_000))
		base = append(base, item{id: id, score: sc})
		id++
	}
	f.nextID = id
	sortFeed(base)
	// 重新编号，保证「越新 id 越大」，与雪花 ID 趋势一致
	for i := range base {
		base[i].id = int64(len(base) - i)
	}
	f.items = base
	f.maxScor = base[0].score
	f.clock = base[0].score + 1
	return f
}

func sortFeed(s []item) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].score != s[j].score {
			return s[i].score > s[j].score
		}
		return s[i].id > s[j].id
	})
}

// publish：新发布一条，落在头部，第 born 页可见
func (f *feed) publish(born int) {
	f.clock++
	it := item{id: f.nextID, score: f.clock, born: born}
	f.nextID++
	if it.score > f.maxScor {
		f.maxScor = it.score
	}
	f.items = append([]item{it}, f.items...)
}

// removeAt：删除/下架一条
func (f *feed) removeAt(idx int) {
	f.items = append(f.items[:idx], f.items[idx+1:]...)
}

// bump：热度重排，把一条的分数抬到最高（分数变化型的漂移）
func (f *feed) bumpAt(idx int) {
	it := f.items[idx]
	f.maxScor++
	it.score = f.maxScor
	f.removeAt(idx)
	f.items = append([]item{it}, f.items...)
}

// pickOld：随机挑一条老内容的下标（只依赖内容状态，与分页器无关 → 两种分页器看到的变异序列完全一致）
func (f *feed) pickOld(r *rand.Rand) int {
	if len(f.items) == 0 {
		return -1
	}
	for try := 0; try < 32; try++ {
		i := r.Intn(len(f.items))
		if f.items[i].born == 0 {
			return i
		}
	}
	return -1
}

type scenario struct {
	name             string
	inserts, deletes int
	bumps            int
	banPage, banTop  int // 第 banPage 页封号批量下架头部 banTop 条
}

type cursor struct {
	score int64
	id    int64
}

type result struct {
	pages, slots, scanned, cmps      int
	uniq, dupRows, dupUnique, missed int
	deletedInitial, inserted         int
	newReturned                      int
	returned                         map[int64]int
	pagesTrace                       []string
}

// pageOffset：模拟 ORDER BY score DESC, id DESC LIMIT offset, 20
// 扫描行数 = 跳过的 offset 行 + 返回的行（这正是 44.14 里的 N × (offset+n) 在 N=1 时的形态）
func pageOffset(f *feed, offset, pageSize int) ([]item, int) {
	if offset >= len(f.items) {
		return nil, len(f.items)
	}
	end := offset + pageSize
	if end > len(f.items) {
		end = len(f.items)
	}
	page := make([]item, end-offset)
	copy(page, f.items[offset:end])
	return page, end // 跳过 offset 行 + 读 end-offset 行
}

// pageCursor：双游标 (score,id)，二分定位「严格小于游标」的第一条
func pageCursor(f *feed, cur *cursor, pageSize int) ([]item, int, int) {
	lo, hi, cmps := 0, len(f.items), 0
	for lo < hi {
		mid := (lo + hi) / 2
		cmps++
		it := f.items[mid]
		// it 是否严格排在游标之后（更旧）
		if it.score < cur.score || (it.score == cur.score && it.id < cur.id) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	end := lo + pageSize
	if end > len(f.items) {
		end = len(f.items)
	}
	if lo >= len(f.items) {
		return nil, 0, cmps
	}
	page := make([]item, end-lo)
	copy(page, f.items[lo:end])
	return page, len(page), cmps
}

func run(sc scenario, seed int64, initN, pageSize, maxPages int) result {
	f := newFeed(initN, seed)
	r := rand.New(rand.NewSource(seed))
	res := result{returned: map[int64]int{}}
	// 记录初始内容与删除
	initial := map[int64]bool{}
	for _, it := range f.items {
		initial[it.id] = true
	}
	deleted := map[int64]bool{}

	mutate := func(page int) {
		for i := 0; i < sc.inserts; i++ {
			f.publish(page)
		}
		if page == sc.banPage {
			for i := 0; i < sc.banTop && len(f.items) > 0; i++ {
				deleted[f.items[0].id] = true
				f.removeAt(0)
			}
		}
		for i := 0; i < sc.deletes; i++ {
			if idx := f.pickOld(r); idx >= 0 {
				deleted[f.items[idx].id] = true
				f.removeAt(idx)
			}
		}
		for i := 0; i < sc.bumps; i++ {
			if idx := f.pickOld(r); idx >= 0 {
				f.bumpAt(idx)
			}
		}
	}

	offset := 0
	cur := &cursor{score: 1<<62 - 1, id: 1<<62 - 1}
	for page := 1; page <= maxPages; page++ {
		var got []item
		if sc.name == "offset" {
			var scan int
			got, scan = pageOffset(f, offset, pageSize)
			res.scanned += scan
			offset += pageSize
			res.pagesTrace = append(res.pagesTrace,
				fmt.Sprintf("page=%2d offset=%4d 返回=%2d 扫描=%3d", page, offset-pageSize, len(got), scan))
		} else {
			var n, c int
			got, n, c = pageCursor(f, cur, pageSize)
			res.scanned += n
			res.cmps += c
			if len(got) > 0 {
				last := got[len(got)-1]
				cur = &cursor{score: last.score, id: last.id}
			}
			res.pagesTrace = append(res.pagesTrace,
				fmt.Sprintf("page=%2d cursor=(%d,%d) 返回=%2d", page, cur.score, cur.id, len(got)))
		}
		res.pages++
		res.slots += len(got)
		for _, it := range got {
			res.returned[it.id]++
		}
		if len(got) == 0 {
			break
		}
		mutate(page)
	}

	// 统计
	dupRows, dupUnique := 0, 0
	newReturned := 0
	for id, c := range res.returned {
		if c > 1 {
			dupRows += c - 1
			dupUnique++
		}
		if !initial[id] {
			newReturned++
		}
	}
	missed := 0
	for id := range initial {
		if deleted[id] {
			continue
		}
		if res.returned[id] == 0 {
			missed++
		}
	}
	res.dupRows, res.dupUnique, res.missed, res.newReturned, res.uniq = dupRows, dupUnique, missed, newReturned, len(res.returned)
	res.deletedInitial, res.inserted = len(deleted), int(f.nextID)-initN-1
	return res
}

func main() {
	const initN, pageSize, maxPages = 500, 20, 60
	scenarios := []struct {
		label           string
		ins, del, bump  int
		banPage, banTop int
	}{
		{"S1 只插入新内容（每页发 8 条）", 8, 0, 0, 0, 0},
		{"S2 插入 + 封号批量下架（第 5 页一次下架头部 120 条）", 8, 0, 0, 5, 120},
		{"S3 插入 + 持续删帖 + 热度重排（每页删 2、改分 2）", 8, 2, 2, 0, 0},
	}
	fmt.Println("实测 A：同进程 A/B，offset 分页 vs (score,id) 双游标分页")
	fmt.Println("环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21；单进程内两种方案各跑一遍，随机种子相同")
	fmt.Printf("参数：初始 %d 条时间线，页大小 %d，最多翻 %d 页；score=发布毫秒时间戳（冻结），同分用 id 兜底\n\n", initN, pageSize, maxPages)
	fmt.Println("| 场景 | 方案 | 页数 | 返回槽位 | 唯一条数 | 重复条数 | 重复的唯一条 | 漏条(仍在线) | 扫描行数 | 游标比较次数 |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	for _, sc := range scenarios {
		var rs [2]result
		for k, name := range []string{"offset", "cursor"} {
			s := scenario{name: name, inserts: sc.ins, deletes: sc.del, bumps: sc.bump,
				banPage: sc.banPage, banTop: sc.banTop}
			rs[k] = run(s, 20261121, initN, pageSize, maxPages)
		}
		for k, name := range []string{"offset 分页", "(score,id) 双游标"} {
			r := rs[k]
			fmt.Printf("| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d |\n",
				sc.label, name, r.pages, r.slots, r.uniq, r.dupRows, r.dupUnique, r.missed, r.scanned, r.cmps)
			if k == 1 {
				fmt.Printf("| ↳ 变异 | 插入 %d 条 / 删除 %d 条（初始 500 条中 %d 条被下架） | | | | | | | | |\n",
					r.inserted, r.deletedInitial, r.deletedInitial)
			}
		}
	}
	fmt.Println()
	fmt.Println("逐页轨迹（前 6 页 + 末页）：")
	for _, sc := range scenarios {
		fmt.Printf("\n-- %s --\n", sc.label)
		for _, mode := range []struct{ algo, name string }{{"offset", "offset 分页"}, {"cursor", "(score,id) 双游标"}} {
			s := scenario{name: mode.algo, inserts: sc.ins, deletes: sc.del, bumps: sc.bump}
			r := run(s, 20261121, initN, pageSize, maxPages)
			fmt.Println(mode.name)
			for i, line := range r.pagesTrace {
				if i < 6 || i >= len(r.pagesTrace)-1 {
					fmt.Println("  " + line)
				} else if i == 6 {
					fmt.Println("  ...")
				}
			}
		}
	}
}
