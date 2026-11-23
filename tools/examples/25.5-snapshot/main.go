// Chandy-Lamport 快照：3 进程 + 3 条 FIFO 通道的最小仿真。
// 拓扑：P0 --c01--> P1 --c12--> P2，P0 --c02--> P2；P0 发起快照。
// 状态：每个进程的 balance（消费一条消息就累加），初始 P0=100（快照前的历史状态）。
// 两组参数：FIFO（marker 不超车）与 非FIFO（marker 在 c02 上超车 d3）。
package main

import (
	"fmt"
	"sort"
)

type kind int

const (
	data kind = iota
	marker
)

type msg struct {
	k      kind
	id     string
	amount int
}

type channel struct {
	name string
	from string
	to   string
	q    []msg // FIFO 队列；非FIFO 组会把 marker 直接插到队首
}

type process struct {
	name        string
	balance     int
	in          []string // 入边通道名
	out         []string // 出边通道名
	recorded    bool
	firstEdge   string
	snapLocal   int
	snapApplied []string
	applied     []string
	snapChan    map[string][]string // 入边 -> 快照记录的通道消息 id
	buffered    map[string][]string
	buffering   map[string]bool
	finished    int
	done        bool
	late        []string // 记录状态之后才到、被漏掉的消息（非FIFO 才会出现）
}

type sim struct {
	procs     map[string]*process
	chans     map[string]*channel
	t         float64
	nonFIFO   bool
	markerCnt int
	logs      []string
	sent      map[string]int // 数据消息 id -> 金额（全局已发）
}

func newSim(nonFIFO bool) *sim {
	s := &sim{procs: map[string]*process{}, chans: map[string]*channel{}, nonFIFO: nonFIFO, sent: map[string]int{}}
	s.procs["P0"] = &process{name: "P0", balance: 100, out: []string{"c01", "c02"},
		snapChan: map[string][]string{}, buffered: map[string][]string{}, buffering: map[string]bool{}}
	s.procs["P1"] = &process{name: "P1", in: []string{"c01"}, out: []string{"c12"},
		snapChan: map[string][]string{}, buffered: map[string][]string{}, buffering: map[string]bool{}}
	s.procs["P2"] = &process{name: "P2", in: []string{"c02", "c12"},
		snapChan: map[string][]string{}, buffered: map[string][]string{}, buffering: map[string]bool{}}
	s.chans["c01"] = &channel{name: "c01", from: "P0", to: "P1"}
	s.chans["c02"] = &channel{name: "c02", from: "P0", to: "P2"}
	s.chans["c12"] = &channel{name: "c12", from: "P1", to: "P2"}
	return s
}

func (s *sim) at(t float64) { s.t = t }

func (s *sim) logf(format string, a ...any) {
	s.logs = append(s.logs, fmt.Sprintf("[t=%4.1f] ", s.t)+fmt.Sprintf(format, a...))
}

func (s *sim) send(chName, id string, amount int) {
	ch := s.chans[chName]
	ch.q = append(ch.q, msg{k: data, id: id, amount: amount})
	s.sent[id] = amount
	s.logf("%s 发送 %s(%d) 到 %s，通道队列=%d", ch.from, id, amount, ch.to, len(ch.q))
}

func (s *sim) markerOf(p *process, chName string) {
	ch := s.chans[chName]
	m := msg{k: marker, id: "M(" + chName + ")"}
	if s.nonFIFO && chName == "c02" {
		ch.q = append([]msg{m}, ch.q...) // marker 超车：插到队首
		s.logf("%s 的 marker 在 %s 上超车（非 FIFO）", p.name, chName)
	} else {
		ch.q = append(ch.q, m)
	}
	s.markerCnt++
}

// 收到第一条 marker：记录本地状态，向所有出边转发 marker，其余入边进入“缓冲”状态。
func (s *sim) record(p *process, edge string) {
	p.recorded = true
	p.firstEdge = edge
	p.snapLocal = p.balance
	p.snapApplied = append([]string(nil), p.applied...)
	if edge != "" {
		p.snapChan[edge] = []string{} // 首条 marker 的入边，通道状态按定义记为空
		p.finished = 1
	}
	for _, e := range p.in {
		if e != edge {
			p.buffering[e] = true
			p.snapChan[e] = nil
		}
	}
	if edge == "" {
		p.finished = len(p.in) // 发起者：没有入边，直接视为完成
	}
	s.logf("%s 记录本地状态 balance=%d，此后沿出边转发 marker", p.name, p.balance)
	p.done = p.finished >= len(p.in)
	for _, e := range p.out {
		s.markerOf(p, e)
	}
}

func (s *sim) deliver(chName string) {
	ch := s.chans[chName]
	m := ch.q[0]
	ch.q = ch.q[1:]
	p := s.procs[ch.to]
	if m.k == data {
		if p.recorded {
			if chName == p.firstEdge {
				// 这条入边已随首条 marker 记为“空”；非 FIFO 下这里会漏消息。
				p.balance += m.amount
				p.applied = append(p.applied, m.id)
				p.late = append(p.late, m.id)
				s.logf("%s 收到 %s(%d)：晚于 marker，更新实时余额但不进快照", p.name, m.id, m.amount)
				return
			}
			if p.buffering[chName] {
				p.buffered[chName] = append(p.buffered[chName], m.id)
				s.logf("%s 收到 %s(%d)：记入 %s 的通道状态", p.name, m.id, m.amount, chName)
				return
			}
		}
		p.balance += m.amount
		p.applied = append(p.applied, m.id)
		s.logf("%s 收到 %s(%d)：balance→%d", p.name, m.id, m.amount, p.balance)
		return
	}

	// 收到 marker
	if !p.recorded {
		s.logf("%s 收到 %s 的第一条 marker", p.name, chName)
		s.record(p, chName)
		return
	}
	if chName == p.firstEdge {
		s.logf("%s 收到 %s 的重复 marker（忽略）", p.name, chName)
		return
	}
	if p.buffering[chName] {
		p.snapChan[chName] = append([]string(nil), p.buffered[chName]...)
		delete(p.buffering, chName)
		p.finished++
		s.logf("%s 收到 %s 的 marker：通道状态=%v", p.name, chName, p.snapChan[chName])
		if p.finished >= len(p.in) {
			p.done = true
		}
	}
}

func (s *sim) run() {
	// 快照前的在途消息：P0→P1 两笔、P0→P2 一笔；P1 已发一笔给 P2。
	s.at(1)
	s.send("c01", "d1", 10)
	s.at(2)
	s.send("c01", "d2", 20)
	s.at(3)
	s.send("c02", "d3", 5)
	s.at(4)
	s.deliver("c01") // P1 消费 d1
	s.at(4.5)
	s.send("c12", "d4", 7)
	s.at(5)
	s.logf("P0 发起快照：记录本地状态 balance=100，向 c01/c02 发 marker")
	s.record(s.procs["P0"], "") // 发起者记录状态并发 marker
	s.at(5.5)
	s.send("c12", "d5", 8) // P1 尚未收到 marker，仍可发消息
	s.at(6)
	s.deliver("c01") // P1 消费 d2
	s.at(6.5)
	s.deliver("c02") // P2 消费 d3
	s.at(7)
	s.deliver("c01") // P1 收到 marker：记录状态并转发
	s.at(7.5)
	s.deliver("c02") // P2 收到 P0 的 marker：记录状态，c12 进入缓冲
	s.at(8)
	s.deliver("c12") // d4 → 通道状态
	s.at(9)
	s.deliver("c12") // d5 → 通道状态
	s.at(10)
	s.deliver("c12") // P1 的 marker → 定稿 c12 通道状态
	for _, name := range []string{"c01", "c02", "c12"} {
		if len(s.chans[name].q) > 0 {
			s.at(11)
			s.deliver(name) // 清空残留，不影响快照
		}
	}
}

func (s *sim) report() {
	fmt.Println("事件时间线（节选）")
	for i, l := range s.logs {
		if i < 26 || i >= len(s.logs)-2 {
			fmt.Println(" ", l)
		}
		if i == 25 && len(s.logs) > 28 {
			fmt.Println("  ...")
		}
	}

	fmt.Println("\n快照内容（每个进程记录本地状态与各入边通道状态）")
	total := 0
	for _, name := range []string{"P0", "P1", "P2"} {
		p := s.procs[name]
		fmt.Printf("  %s 本地状态=%d  已应用消息=%v\n", name, p.snapLocal, p.snapApplied)
		total += p.snapLocal
		for _, e := range p.in {
			ids := p.snapChan[e]
			amt := 0
			for _, id := range ids {
				amt += s.sent[id]
			}
			fmt.Printf("     通道 %s 状态=%v（在途 %d 条 / %d 元）\n", e, ids, len(ids), amt)
			total += amt
		}
	}

	// 一致性核对：每条已发消息恰好出现在「某个本地状态」或「某条通道状态」一次。
	seen := map[string]int{}
	for _, p := range s.procs {
		for _, id := range p.snapApplied {
			seen[id]++
		}
		for _, ids := range p.snapChan {
			for _, id := range ids {
				seen[id]++
			}
		}
	}
	ids := make([]string, 0, len(s.sent))
	for id := range s.sent {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var missing, dup []string
	for _, id := range ids {
		switch seen[id] {
		case 0:
			missing = append(missing, id)
		case 1:
		default:
			dup = append(dup, id)
		}
	}
	expected := 100
	for _, id := range ids {
		expected += s.sent[id]
	}
	consistent := total == expected && len(missing) == 0 && len(dup) == 0
	fmt.Printf("\n核对：快照总额=%d；应等于 初始状态100 + 已发%d = %d\n", total, expected-100, expected)
	fmt.Printf("      消息覆盖：已发 %d 条，缺失=%v，重复=%v\n", len(ids), missing, dup)
	fmt.Printf("      marker 数=%d（= 边数 3）；先于 marker 被漏掉的消息=%v\n", s.markerCnt, s.procs["P2"].late)
	if consistent {
		fmt.Println("      判定：一致（每条消息恰好出现一次，总额对得上）")
	} else {
		fmt.Printf("      判定：不一致（漏了 %v，快照缺 %d 元）\n", missing, expected-total)
	}
}

func main() {
	fmt.Println("=== 运行 A：FIFO 通道（算法前提成立） ===")
	a := newSim(false)
	a.run()
	a.report()

	fmt.Println("\n=== 运行 B：非 FIFO 通道（marker 在 c02 上超过 d3） ===")
	b := newSim(true)
	b.run()
	b.report()
}
