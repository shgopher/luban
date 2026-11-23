// 内容审核状态机 + 异步回调乱序压力测试：同进程 A/B。
//
// 正确版 = 签名校验 + 去重表（audit_id 唯一）+ 条件更新（from 状态集，终态不可回退）
// 错误版 = 签名校验 + 无条件 UPDATE（last-write-wins，没有去重表）
//
// 输入完全一致：800 条内容 / 5000 次回调，每条内容都含「重复投递 + 乱序重试 + 伪造签名」。
// 输出：违规内容驳回后重新上线次数、正常内容被错误下线次数、终态错误条数、吞吐。
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ---------- 1. 状态与可见性分开建模 ----------

type Status string

const (
	StMachineReviewing Status = "machine_reviewing" // 机审中（隔离区）
	StHumanReviewing   Status = "human_reviewing"   // 人审中（隔离区）
	StPublished        Status = "published"         // 已发布
	StRejected         Status = "rejected"          // 已驳回
)

type Visibility string

const (
	VisPrivate Visibility = "private" // 仅自己
	VisPublic  Visibility = "public"  // 公开
	VisHidden  Visibility = "hidden"  // 屏蔽
)

// ---------- 2. 回调类型 → 合法起始状态（条件更新的 WHERE） ----------

type callKind int

const (
	kMachinePass      callKind = iota // 机审通过（先发后审策略：直接上线）
	kMachineUncertain                 // 机审不确定 → 转人审
	kPostReviewReject                 // 举报/巡检复审驳回（从已发布下线）
	kHumanPass                        // 人审通过
	kHumanReject                      // 人审驳回
	kReviewPass                       // 申诉复审通过（从驳回恢复上线）
)

type rule struct {
	from []Status // 只有当前状态在这里面，UPDATE 才允许生效
	to   Status
	vis  Visibility
}

var rules = map[callKind]rule{
	kMachinePass:      {[]Status{StMachineReviewing}, StPublished, VisPublic},
	kMachineUncertain: {[]Status{StMachineReviewing}, StHumanReviewing, VisPrivate},
	kPostReviewReject: {[]Status{StPublished}, StRejected, VisPrivate},
	kHumanPass:        {[]Status{StHumanReviewing}, StPublished, VisPublic},
	kHumanReject:      {[]Status{StHumanReviewing}, StRejected, VisPrivate},
	kReviewPass:       {[]Status{StRejected}, StPublished, VisPublic},
}

// 申诉受理+复审通过合并为一次回调；表里拆成 appealing 两跳。
// self_only → published 走作者操作（内容已审过），不经过机审回调，故不在 rules 里。

// ---------- 3. 存储：条件更新 / 无条件更新 ----------

type row struct {
	status   Status
	vis      Visibility
	rejected bool     // 是否曾被驳回（用于识别「违规复活」）
	trace    []string // 状态轨迹，用于抽样打印
}

type store struct {
	mu    sync.Mutex
	rows  []row
	truth []bool // true = 该内容真值违规
	// 指标
	violationOnline int // 违规内容被驳回后又置为上线（次数）
	normalOffline   int // 正常内容被错误下线（次数）
}

func (s *store) move(r *row, id int, to Status, vis Visibility) {
	from := r.status
	r.status, r.vis = to, vis
	if to == StRejected {
		r.rejected = true
	}
	if to == StPublished && from != StPublished && s.truth[id] && r.rejected {
		s.violationOnline++ // 违规内容被驳回后又被旧回调置为上线
	}
	if !s.truth[id] && from == StPublished && to != StPublished {
		s.normalOffline++ // 正常内容从公开被打回（用户视角=消失）
	}
	r.trace = append(r.trace, fmt.Sprintf("%s/%s", to, vis))
}

// conditional 语义 = UPDATE content SET status=?,visibility=? WHERE id=? AND status IN (...)
// 返回 affected rows 是否为 1；from 不匹配或同值更新都返回 false（affected rows = 0）。
func (s *store) conditional(id int, from []Status, to Status, vis Visibility) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &s.rows[id]
	ok := false
	for _, f := range from {
		if r.status == f {
			ok = true
			break
		}
	}
	if !ok || (r.status == to && r.vis == vis) {
		return false
	}
	s.move(r, id, to, vis)
	return true
}

// unconditional 语义 = UPDATE content SET status=?,visibility=? WHERE id=?（last-write-wins）
func (s *store) unconditional(id int, to Status, vis Visibility) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &s.rows[id]
	if r.status == to && r.vis == vis {
		return false
	}
	s.move(r, id, to, vis)
	return true
}

// ---------- 4. 回调、签名与处理器 ----------

type callback struct {
	auditID   string
	contentID int
	seq       int
	kind      callKind
	sig       string
}

const secret = "mock-machine-audit-secret"

func sign(auditID string, contentID, seq int, kind callKind) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%s|%d|%d|%d", auditID, contentID, seq, kind)
	return hex.EncodeToString(m.Sum(nil))
}

func verify(cb callback) bool {
	return hmac.Equal([]byte(cb.sig), []byte(sign(cb.auditID, cb.contentID, cb.seq, cb.kind)))
}

type processor struct {
	st          *store
	useDedup    bool
	conditional bool
	dedup       map[string]struct{}
	mu          sync.Mutex
	forged      int
	dupSkipped  int
	staleBlock  int
	applied     int
}

func (p *processor) handle(cb callback) {
	// 坑 4：伪造 —— 先验签，验签失败一律不落状态
	if !verify(cb) {
		p.forged++
		return
	}
	// 坑 1：重复 —— 去重表按 audit_id 唯一
	if p.useDedup {
		p.mu.Lock()
		_, seen := p.dedup[cb.auditID]
		if !seen {
			p.dedup[cb.auditID] = struct{}{}
		}
		p.mu.Unlock()
		if seen {
			p.dupSkipped++
			return
		}
	}
	r := rules[cb.kind]
	var ok bool
	if p.conditional {
		ok = p.st.conditional(cb.contentID, r.from, r.to, r.vis)
	} else {
		ok = p.st.unconditional(cb.contentID, r.to, r.vis)
	}
	if ok {
		p.applied++
	} else if p.conditional {
		p.staleBlock++
	}
}

// ---------- 5. 测试数据：A 违规 / B 正常（初审误杀后申诉通过）/ C 正常（顺利） ----------

type testCase struct {
	id        int
	violation bool
	expected  Status
	delivery  []callback
}

func signed(id int, auditID string, seq int, kind callKind) callback {
	return callback{auditID, id, seq, kind, sign(auditID, id, seq, kind)}
}

func forged(id int, auditID string, seq int, kind callKind) callback {
	return callback{auditID, id, seq, kind, "deadbeef-forged-signature"}
}

func makeCases() []testCase {
	cases := make([]testCase, 0, 800)
	// A：200 条真值违规。机审放行上线 → 举报复审驳回 → 旧机审回调延迟重投/另一路旧结果迟到。
	for i := 0; i < 200; i++ {
		cid := i
		cbs := []callback{
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachinePass),
			signed(cid, fmt.Sprintf("r#%d", cid), 2, kPostReviewReject),
			signed(cid, fmt.Sprintf("r#%d", cid), 2, kPostReviewReject), // 重复投递
			forged(cid, fmt.Sprintf("f#%d", cid), 2, kHumanPass),        // 伪造签名
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachinePass),      // 同 audit_id 重试，迟到
			signed(cid, fmt.Sprintf("m2#%d", cid), 1, kMachinePass),     // 另一路旧结果，迟到
		}
		cases = append(cases, testCase{cid, true, StRejected, cbs})
	}
	// B：200 条正常内容，初审被误杀 → 申诉复审通过；但初审驳回回调延迟重投。
	for i := 200; i < 400; i++ {
		cid := i
		cbs := []callback{
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachineUncertain),
			signed(cid, fmt.Sprintf("h#%d", cid), 2, kHumanReject),
			signed(cid, fmt.Sprintf("h#%d", cid), 2, kHumanReject), // 重复投递
			signed(cid, fmt.Sprintf("v#%d", cid), 3, kReviewPass),  // 申诉通过
			forged(cid, fmt.Sprintf("f#%d", cid), 2, kHumanReject), // 伪造签名
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachineUncertain), // 迟到重试
			signed(cid, fmt.Sprintf("h2#%d", cid), 2, kHumanReject),     // 另一审的旧驳回，迟到
		}
		cases = append(cases, testCase{cid, false, StPublished, cbs})
	}
	// C：400 条正常内容顺利发布；迟到重试的结果与终态一致（多数情况不会出错）。
	for i := 400; i < 800; i++ {
		cid := i
		cbs := []callback{
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachineUncertain),
			signed(cid, fmt.Sprintf("h#%d", cid), 2, kHumanPass),
			signed(cid, fmt.Sprintf("h#%d", cid), 2, kHumanPass),    // 重复投递
			forged(cid, fmt.Sprintf("f#%d", cid), 2, kHumanReject),  // 伪造签名
			signed(cid, fmt.Sprintf("m#%d", cid), 1, kMachineUncertain), // 迟到重试
			signed(cid, fmt.Sprintf("h2#%d", cid), 2, kHumanPass),       // 另一审的旧通过，迟到
		}
		cases = append(cases, testCase{cid, false, StPublished, cbs})
	}
	return cases
}

// interleave 把所有内容按「波次」交错投递，模拟真实消费端多分区并发。
func interleave(cases []testCase) []callback {
	maxLen := 0
	for _, c := range cases {
		if len(c.delivery) > maxLen {
			maxLen = len(c.delivery)
		}
	}
	out := make([]callback, 0, 5000)
	for step := 0; step < maxLen; step++ {
		for _, c := range cases {
			if step < len(c.delivery) {
				out = append(out, c.delivery[step])
			}
		}
	}
	return out
}

type result struct {
	name            string
	forged          int
	dupSkipped      int
	staleBlock      int
	applied         int
	violationOnline int
	normalOffline   int
	finalMismatch   int
	elapsed         time.Duration
	p50, p99        time.Duration
	total           int
}

func run(name string, cases []testCase, cbs []callback, useDedup, conditional bool) result {
	st := &store{rows: make([]row, len(cases)), truth: make([]bool, len(cases))}
	for i, c := range cases {
		st.rows[i] = row{status: StMachineReviewing, vis: VisPrivate}
		st.truth[i] = c.violation
	}
	p := &processor{st: st, useDedup: useDedup, conditional: conditional, dedup: map[string]struct{}{}}
	if useDedup {
		p.dedup = make(map[string]struct{}, len(cbs))
	}
	durs := make([]time.Duration, 0, len(cbs))
	start := time.Now()
	for _, cb := range cbs {
		t0 := time.Now()
		p.handle(cb)
		durs = append(durs, time.Since(t0))
	}
	elapsed := time.Since(start)

	mismatch := 0
	for i, c := range cases {
		if st.rows[i].status != c.expected {
			mismatch++
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	return result{
		name: name, forged: p.forged, dupSkipped: p.dupSkipped, staleBlock: p.staleBlock,
		applied: p.applied, violationOnline: st.violationOnline, normalOffline: st.normalOffline,
		finalMismatch: mismatch, elapsed: elapsed,
		p50: durs[len(durs)/2], p99: durs[len(durs)*99/100],
		total: len(cbs),
	}
}

func (r result) String() string {
	sec := r.elapsed.Seconds()
	return fmt.Sprintf(`[%s]
  签名伪造被拒         %d
  重复回调去重跳过     %d
  旧状态回调被拦截     %d
  真正生效的回调       %d
  违规内容驳回后上线   %d
  正常内容被错误下线   %d
  终态与真值不一致     %d
  耗时 %v，吞吐 %.0f 次回调/秒，单次 P50=%v P99=%v`,
		r.name, r.forged, r.dupSkipped, r.staleBlock, r.applied,
		r.violationOnline, r.normalOffline, r.finalMismatch,
		r.elapsed, float64(r.total)/sec, r.p50, r.p99)
}

func main() {
	cases := makeCases()
	cbs := interleave(cases)
	fmt.Printf("环境: go1.27.1 darwin/arm64 | 内容 %d 条，回调 %d 次（每条 6~7 次，含重复+乱序+伪造签名）\n",
		len(cases), len(cbs))
	fmt.Println("分组: A 违规 200 条 | B 正常-初审误杀后申诉通过 200 条 | C 正常-顺利 400 条")
	fmt.Println()

	// 同进程 A/B：输入完全相同，只换「状态推进机制」
	correct := run("正确版：去重表 + 条件更新", cases, cbs, true, true)
	wrong := run("错误版：无条件 UPDATE（last-write-wins）", cases, cbs, false, false)
	fmt.Println(correct)
	fmt.Println()
	fmt.Println(wrong)
	fmt.Println()

	fmt.Println("抽样轨迹（内容 #0，真值=违规，期望终态 rejected/private）：")
	fmt.Printf("  正确版: %s\n", traceOfTrace(cases, cbs, true, true, 0))
	fmt.Printf("  错误版: %s\n", traceOfTrace(cases, cbs, false, false, 0))
	fmt.Println("抽样轨迹（内容 #700，真值=正常，期望终态 published/public）：")
	fmt.Printf("  正确版: %s\n", traceOfTrace(cases, cbs, true, true, 700))
	fmt.Printf("  错误版: %s\n", traceOfTrace(cases, cbs, false, false, 700))
}

// traceOfTrace 重跑一遍单条内容并打印轨迹，便于对照。
func traceOfTrace(cases []testCase, cbs []callback, useDedup, conditional bool, id int) string {
	st := &store{rows: make([]row, len(cases)), truth: make([]bool, len(cases))}
	for i, c := range cases {
		st.rows[i] = row{status: StMachineReviewing, vis: VisPrivate}
		st.truth[i] = c.violation
	}
	p := &processor{st: st, useDedup: useDedup, conditional: conditional, dedup: map[string]struct{}{}}
	if useDedup {
		p.dedup = make(map[string]struct{}, len(cbs))
	}
	for _, cb := range cbs {
		if cb.contentID == id {
			p.handle(cb)
		}
	}
	trace := st.rows[id].trace
	out := fmt.Sprintf("machine_reviewing/private")
	for _, t := range trace {
		out += " → " + t
	}
	return out
}
