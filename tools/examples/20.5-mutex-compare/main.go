// 20.5 按消息量算账：集中式 / Lamport / Ricart-Agrawala / 令牌环的闭环仿真
// 场景：N 个节点、K 个客户端各自「用完立刻再抢」（闭环，始终有 K 个请求在争用），
//
//	临界区 C=2ms，单程消息延迟 L=1ms，仿真 10s。
//
// 说明：消息数按协议定义记账（集中式 3 条；Lamport 3(N−1)；RA 2(N−1)）；
//
//	排队与令牌传递用离散事件推进，令牌环的每条进入消息数由「令牌要走过几个节点」真实产生。
//
// 运行：go run main.go（go1.27.1 / darwin/arm64 / Apple M1）
package main

import (
	"fmt"
	"strings"
)

const (
	nodes               = 10
	csUS                = 2000 // 临界区 2ms
	latUS               = 1000 // 单程延迟 1ms
	simUS               = int64(10_000_000)
	msgsPerEntryCentral = 3
)

func width(s string) int {
	w := 0
	for _, r := range s {
		if r > 0x2E7F {
			w += 2
		} else {
			w++
		}
	}
	return w
}
func pad(s string, n int) string {
	if d := n - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
func row(cells []string, widths []int) {
	line := ""
	for i, c := range cells {
		if i > 0 {
			line += "  "
		}
		line += pad(c, widths[i])
	}
	fmt.Println(line)
}

// ---------- 队列类算法：单临界区串行，按协议的关键路径推进 ----------
// 集中式：释放(L)→协调者→授权(L) → 每条进入的关键路径 C+2L
// Lamport/RA：前一个持有者退出时把延迟的回复发出(L)，下一个才能进 → C+L
func queueAlgo(k int, entryCostUS int64, perEntry int64) (entries int64, msgs int64, throughput float64) {
	var now int64
	for now < simUS {
		now += entryCostUS
		entries++
	}
	msgs = entries * perEntry
	return entries, msgs, float64(entries) / (float64(simUS) / 1e6)
}

type ringResult struct{ entries, hops int64 }

// ---------- 令牌环：真实令牌逐跳推进 ----------
// 令牌在每个节点停：有等待请求 → 进临界区 C；没有 → 立即传给下一个节点。
// 每传一跳计 1 条消息；闭环下 K 个客户端一直待命，令牌每绕一圈服务 K 次。
func ring(k int) ringResult {
	pending := make([]bool, nodes)
	for i := 0; i < k; i++ {
		pending[i] = true
	}
	var now int64
	var entries, hops int64
	pos := 0
	for now < simUS {
		if pending[pos] {
			now += csUS // 进入临界区
			entries++
			pending[pos] = false
			// 客户端用完立刻再发起下一次请求（闭环）
			// 注意：请求从「用完」时刻起等待令牌再次到达
		}
		// 传令牌给下一个节点
		now += latUS
		hops++
		next := (pos + 1) % nodes
		// 刚服务的客户端重新排队（闭环：立刻再请求）
		if pos < k {
			pending[pos] = true
		}
		pos = next
	}
	return ringResult{entries, hops}
}

func main() {
	fmt.Printf("参数：N=%d 节点，临界区 C=2ms，单程延迟 L=1ms，闭环仿真 10s（K 个客户端一直有请求在等）\n\n", nodes)
	fmt.Println("表 A：每条进入的消息数（与并发无关，只随 N 变）")
	row([]string{"算法", "N=5", "N=10", "N=50", "备注"},
		[]int{16, 8, 8, 8, 34})
	ns := []int{5, 10, 50}
	cells := []string{"集中式"}
	for range ns {
		cells = append(cells, "3")
	}
	cells = append(cells, "请求+授权+释放；外加协调者节点")
	row(cells, []int{16, 8, 8, 8, 34})
	cells = []string{"Lamport"}
	for _, n := range ns {
		cells = append(cells, fmt.Sprintf("%d", 3*(n-1)))
	}
	cells = append(cells, "请求+回复+释放 = 3(N−1)")
	row(cells, []int{16, 8, 8, 8, 34})
	cells = []string{"Ricart-Agrawala"}
	for _, n := range ns {
		cells = append(cells, fmt.Sprintf("%d", 2*(n-1)))
	}
	cells = append(cells, "请求+回复 = 2(N−1)，延迟回复当释放")
	row(cells, []int{16, 8, 8, 8, 34})
	row([]string{"令牌环（K=N 满载）", "1", "1", "1", "令牌每绕一圈服务 N 次，摊薄到 1 跳/次"},
		[]int{16, 8, 8, 8, 34})

	fmt.Println("\n表 B：每秒消息量（N=10，不同并发 K；各算法按自己的吞吐跑满 10s）")
	row([]string{"并发 K", "集中式 条/秒", "Lamport 条/秒", "RA 条/秒", "环 进入/秒", "环 条/秒", "环 条/进入"},
		[]int{8, 14, 16, 12, 12, 12, 12})
	cEntries, cMsgs, cTPS := queueAlgo(0, csUS+2*latUS, msgsPerEntryCentral)
	lEntries, lMsgs, _ := queueAlgo(0, csUS+latUS, int64(3*(nodes-1)))
	rEntries, rMsgs, _ := queueAlgo(0, csUS+latUS, int64(2*(nodes-1)))
	for _, k := range []int{1, 2, 5, 10} {
		rr := ring(k)
		row([]string{fmt.Sprintf("%d", k),
			fmt.Sprintf("%.0f", float64(cMsgs)/10),
			fmt.Sprintf("%.0f", float64(lMsgs)/10),
			fmt.Sprintf("%.0f", float64(rMsgs)/10),
			fmt.Sprintf("%.0f", float64(rr.entries)/10),
			fmt.Sprintf("%.0f", float64(rr.hops)/10),
			fmt.Sprintf("%.1f", float64(rr.hops)/float64(rr.entries))},
			[]int{8, 14, 16, 12, 12, 12, 12})
	}
	fmt.Printf("\n校验：集中式 %d 次进入 × 3 = %d 条；Lamport %d 次 × %d = %d 条；RA %d 次 × %d = %d 条（10s 累计）\n",
		cEntries, cMsgs, lEntries, 3*(nodes-1), lMsgs, rEntries, 2*(nodes-1), rMsgs)
	fmt.Printf("      集中式吞吐 = %.0f 次/秒（关键路径 C+2L=4ms）；Lamport/RA = %.0f 次/秒（C+L=3ms）\n",
		cTPS, 1/((csUS+latUS)/float64(1e6)))
	// 用断言锁住两条恒等式：集中式 3 条/次；环在 K=N 时 1 跳/次
	if cMsgs != cEntries*3 {
		panic("集中式消息账不平")
	}
	rr := ring(10)
	if rr.hops != rr.entries {
		panic(fmt.Sprintf("环满载应 1 跳/次，实际 %d/%d", rr.hops, rr.entries))
	}
	fmt.Println("断言通过：集中式 3 条/次、令牌环满载 1 跳/次")
}
