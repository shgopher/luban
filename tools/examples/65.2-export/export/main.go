// 按租户导出：keyset 分页 + 每页 sha256 + 清单断点续传 + 重投幂等 + 快照上限。
// 纯标准库，go run 直接跑。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type row struct {
	id     int
	tenant string
	body   string
}

var rows []row // 真实系统里按 id 走主键索引

func init() {
	id := 0
	add := func(tenant string, n int) {
		for i := 0; i < n; i++ {
			id++
			rows = append(rows, row{id, tenant, fmt.Sprintf("payload-%08d", id)})
		}
	}
	add("t-001", 12347)
	add("t-002", 8001)
	add("t-003", 4002)
}

func tenantRows(tenant string) []row {
	var out []row
	for _, x := range rows {
		if x.tenant == tenant {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// 一次导出请求：租户 + keyset 游标 + 页大小 + 导出开始时的快照上限
type pageReq struct {
	tenant      string
	afterID     int
	limit       int
	snapshotMax int // 开始导出时记录的 MAX(id)，之后的写入不进本次导出
}

func exportPage(r pageReq) (csv []byte, lastID int, n int) {
	var b strings.Builder
	b.WriteString("id,body\n")
	lastID = r.afterID
	for _, x := range tenantRows(r.tenant) {
		if x.id <= r.afterID || x.id > r.snapshotMax {
			continue
		}
		fmt.Fprintf(&b, "%d,%s\n", x.id, x.body)
		lastID = x.id
		n++
		if n == r.limit {
			break
		}
	}
	return []byte(b.String()), lastID, n
}

func sha8(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:8])
}

type part struct {
	no     int
	rows   int
	sha8   string
	lastID int
}

func exportAll(tenant string, page, maxID int) []part {
	var out []part
	after, no := 0, 0
	for {
		csv, last, n := exportPage(pageReq{tenant, after, page, maxID})
		if n == 0 {
			break
		}
		no++
		out = append(out, part{no, n, sha8(csv), last})
		after = last
	}
	return out
}

func main() {
	const tenant = "t-001"
	const page = 5000
	snapshotMax := 0
	for _, x := range tenantRows(tenant) {
		if x.id > snapshotMax {
			snapshotMax = x.id
		}
	}
	fmt.Printf("环境：纯标准库；租户 %s 共 %d 行，每页 %d 行，快照上限 maxID=%d\n",
		tenant, len(tenantRows(tenant)), page, snapshotMax)

	// ① 全量导出：逐页生成，记录每页 行数/sha/游标
	full := exportAll(tenant, page, snapshotMax)
	for _, p := range full {
		fmt.Printf("全量 part%d rows=%-5d sha256[:8]=%s lastID=%d\n", p.no, p.rows, p.sha8, p.lastID)
	}

	// ② 模拟中断：只导完前 2 页进程崩了；重跑时从清单 lastID 续，而不是 offset
	resume := full[1].lastID
	csv3, _, n3 := exportPage(pageReq{tenant, resume, page, snapshotMax})
	fmt.Printf("\n中断于 part2 后，从 lastID=%d 续传：rows=%d sha256[:8]=%s 与全量 part3 一致=%v\n",
		resume, n3, sha8(csv3), sha8(csv3) == full[2].sha8)

	// ③ 幂等重投：同一页（同 tenant+afterID+limit）重跑两次，结果逐字节相同
	a, _, _ := exportPage(pageReq{tenant, full[1].lastID, page, snapshotMax})
	b, _, _ := exportPage(pageReq{tenant, full[1].lastID, page, snapshotMax})
	fmt.Printf("同一分页请求重投两次 sha256[:8]=%s / %s 一致=%v（接收端按 (export_id,part) 覆盖写，不会翻倍）\n",
		sha8(a), sha8(b), sha8(a) == sha8(b))

	// ④ 对照实验：导出到 part2 时插入 3 条 id 更小的补录行（回填），再取下一页
	before := len(tenantRows(tenant))
	for i := 1; i <= 3; i++ {
		rows = append(rows, row{-i, tenant, fmt.Sprintf("backfill-%d", i)})
	}
	fmt.Printf("\n导出期间回填 3 行（%d → %d 行）\n", before, len(tenantRows(tenant)))

	// offset 分页：part3 = 跳过前 10000 行取 5000 行
	tr := tenantRows(tenant)
	offsetIDs := map[int]bool{}
	for _, x := range tr[10000:] {
		offsetIDs[x.id] = true
	}
	// 与"已导出的 part1+part2 = id 1..resume"对比：offset 结果里混进了已导出的行
	dup, miss := 0, 0
	for id := range offsetIDs {
		if id <= resume {
			dup++
		}
	}
	for id := resume + 1; id <= snapshotMax; id++ {
		if !offsetIDs[id] {
			miss++
		}
	}
	fmt.Printf("offset 分页重取 part3：重复行=%d、漏行=%d（回填行把游标往后挤）\n", dup, miss)

	// keyset + snapshotMax：重复行与漏行都必须是 0
	keysetIDs := map[int]bool{}
	for _, x := range tr {
		if x.id > resume && x.id <= snapshotMax {
			keysetIDs[x.id] = true
		}
	}
	kdup, kmiss := 0, 0
	for id := range keysetIDs {
		if id <= resume {
			kdup++
		}
	}
	for id := resume + 1; id <= snapshotMax; id++ {
		if !keysetIDs[id] {
			kmiss++
		}
	}
	fmt.Printf("keyset(带 snapshotMax) 重取 part3：重复行=%d、漏行=%d（回填行不在本次快照内）\n", kdup, kmiss)
}
