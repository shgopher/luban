// 实验 B：三种多端同步协议在同一个负载下的对比
//
//	① 只推：每条消息把消息本体推给在线端；断线丢失靠重连全量同步
//	② 只拉：每 1s 轮询 cursor 拉增量
//	③ 推叫醒 + 拉补全：推送只带 change 水位（叫醒），数据一律按 cursor 拉取
//
// 同一负载：1 用户 3 设备、300 秒、600 条消息；d_pad 在 120–150s 断线 30s。
// 统计：总流量、数据重复率、丢失、P50/P99 看到新消息延迟、断线 30s 补齐耗时/字节。
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

const (
	durationMs   = 300_000
	msgTotal     = 600
	publishEvery = 500 // 每 500ms 一条 => 2 条/秒
	pushLoss     = 0.10
	dupRate      = 0.10
	pollMs       = 1_000 // 只拉协议：1s 轮询
	backstopMs   = 3_000 // 推叫醒协议：3s 兜底轮询
	offlineStart = 120_000
	offlineEnd   = 150_000
)

var deviceNames = []string{"d_phone", "d_pad", "d_pc"}
var offlineDev = 1 // d_pad 断线 30s

type Msg struct {
	MsgID  string `json:"msg_id"`
	ConvID string `json:"conv_id"`
	Seq    int64  `json:"seq"`
	Sender string `json:"sender"`
	Body   string `json:"body"`
	Ts     int64  `json:"ts_ms"`
}

func bodyText(seq int64) string {
	// 固定 48 字符正文，便于比较各协议的真实传输字节
	return fmt.Sprintf("payload-%04d-%s", seq, strings.Repeat("x", 32))
}

type delivery struct {
	at      int64
	kind    int // 0=消息本体 1=水位
	msg     Msg
	payload []byte
}

type device struct {
	id     string
	online bool

	cursor  int64 // 已应用的连续 conv_seq
	known   map[int64]bool
	applyAt map[int64]int64
	applied int

	inflight []delivery
	pullAt   int64

	reqBytes, respBytes int64
	deliveries          int // 收到的消息投递次数（含重复）
	dupDeliveries       int
	dupPush, dupSync    int
	outOfOrder          int
	pushRecv, pushDup   int
	pulls, fullSyncs    int

	recoverDurMs  int64
	recoverBytes  int64
	recoverTarget int64 // 断线重连时需要补齐到的 seq
}

func newDevice(i int) *device {
	return &device{
		id:      deviceNames[i],
		online:  true,
		known:   map[int64]bool{},
		applyAt: map[int64]int64{},
	}
}

func (d *device) apply(m Msg, now int64, src int) {
	d.deliveries++
	if d.known[m.Seq] {
		d.dupDeliveries++ // 重复投递：按 msg_id/seq 去重
		if src == 0 {
			d.dupPush++
		} else {
			d.dupSync++ // 全量重放造成的重复
		}
		return
	}
	if m.Seq != d.cursor+1 {
		d.outOfOrder++ // 乱序到达：客户端必须按 seq 排序后再展示
	}
	d.known[m.Seq] = true
	d.applyAt[m.Seq] = now
	for d.known[d.cursor+1] {
		d.cursor++
	}
	d.applied = len(d.known)
}

type protocol string

const (
	protoPush     protocol = "只推"
	protoPull     protocol = "只拉(1s轮询)"
	protoPushPull protocol = "推叫醒+拉补全"
)

type result struct {
	name             protocol
	totalMsgs        int64
	known            int
	missing          int
	bytes            int64
	reqBytes         int64
	deliveries, dups int
	pulls, pushes    int
	fullSyncs        int
	outOfOrder       int
	dupPush, dupSync int
	lat              []int64
	recoverDurMs     []int64
	recoverBytes     int64
	recoverOK        int
}

func run(p protocol, seed int64) result {
	rng := rand.New(rand.NewSource(seed))
	devs := []*device{newDevice(0), newDevice(1), newDevice(2)}
	for _, d := range devs {
		d.pullAt = 1000
	}
	var serverMsgs []Msg
	var seq int64
	res := result{name: p}

	syncReq, _ := json.Marshal(map[string]int64{"conv_cursor": 0})

	// pull 取增量
	doPull := func(d *device, now int64) {
		if !d.online {
			return
		}
		var msgs []Msg
		for _, m := range serverMsgs {
			if m.Seq > d.cursor {
				msgs = append(msgs, m)
			}
		}
		if len(msgs) > 100 {
			msgs = msgs[:100]
		}
		req := map[string]int64{"conv_cursor": d.cursor}
		reqBytes, _ := json.Marshal(req)
		resp := map[string]any{"conv_cursor": d.cursor + int64(len(msgs)), "messages": msgs, "has_more": false}
		respBytes, _ := json.Marshal(resp)
		d.reqBytes += int64(len(reqBytes))
		d.respBytes += int64(len(respBytes))
		d.pulls++
		for _, m := range msgs {
			d.apply(m, now, 2)
		}
	}
	// 全量快照（只推协议重连时使用：发全部历史，没有 cursor 概念）
	doFullSync := func(d *device, now int64) {
		if !d.online {
			return
		}
		resp := map[string]any{
			"snapshot_seq":  len(serverMsgs),
			"conversations": []map[string]any{{"conv_id": "c1", "last_seq": len(serverMsgs)}},
			"messages":      serverMsgs,
		}
		b, _ := json.Marshal(resp)
		d.respBytes += int64(len(b))
		d.fullSyncs++
		for _, m := range serverMsgs {
			d.apply(m, now, 1)
		}
	}

	for t := int64(0); t <= durationMs+2000; t++ {
		// 1) 发布消息
		if t%publishEvery == 0 && seq < msgTotal {
			seq++
			m := Msg{MsgID: fmt.Sprintf("m1_%04d", seq), ConvID: "c1", Seq: seq, Sender: "u1", Body: bodyText(seq), Ts: t}
			serverMsgs = append(serverMsgs, m)
			msgBytes, _ := json.Marshal(m)
			wmBytes, _ := json.Marshal(map[string]int64{"change_seq": seq}) // 水位：没有正文
			for _, d := range devs {
				if !d.online {
					continue
				}
				switch p {
				case protoPush:
					if rng.Float64() >= pushLoss {
						lat := int64(5 + rng.Intn(20))
						d.inflight = append(d.inflight, delivery{at: t + lat, kind: 0, msg: m, payload: msgBytes})
						res.pushes++
						if rng.Float64() < dupRate {
							d.inflight = append(d.inflight, delivery{at: t + lat + int64(rng.Intn(5)), kind: 0, msg: m, payload: msgBytes})
							res.pushes++
						}
					}
				case protoPushPull:
					if rng.Float64() >= pushLoss {
						lat := int64(5 + rng.Intn(20))
						d.inflight = append(d.inflight, delivery{at: t + lat, kind: 1, payload: wmBytes})
						res.pushes++
						if rng.Float64() < dupRate {
							d.inflight = append(d.inflight, delivery{at: t + lat + int64(rng.Intn(5)), kind: 1, payload: wmBytes})
							res.pushes++
						}
					}
				}
			}
		}

		// 2) 到货
		for _, d := range devs {
			if len(d.inflight) == 0 {
				continue
			}
			keep := d.inflight[:0]
			for _, f := range d.inflight {
				if f.at > t {
					keep = append(keep, f)
					continue
				}
				switch f.kind {
				case 0: // 消息本体
					d.respBytes += int64(len(f.payload))
					d.apply(f.msg, t, 0)
				case 1: // 水位：只叫醒，拉取在下面统一做
					d.pushRecv++
					if rng.Float64() < dupRate {
						d.pushDup++
					}
					// 收到叫醒后 5~20ms 内安排一次拉取
					at := t + int64(5+rng.Intn(15))
					if at < d.pullAt {
						d.pullAt = at
					}
				}
			}
			if len(keep) < len(d.inflight) {
				d.outOfOrder += 0 // 排序信息见 apply 统计
			}
			d.inflight = keep
		}

		// 3) 拉取
		for _, d := range devs {
			if !d.online {
				continue
			}
			switch p {
			case protoPull:
				if t >= d.pullAt {
					doPull(d, t)
					d.pullAt = t + pollMs
				}
			case protoPushPull:
				if t >= d.pullAt {
					doPull(d, t)
					d.pullAt = t + backstopMs // 兜底：即使推送全丢，最迟 3s 后也会拉到
				}
			case protoPush:
				// 只推协议没有任何拉取
			}
		}

		// 4) 断线 / 重连
		if t == offlineStart {
			for _, d := range devs {
				if d.id == deviceNames[offlineDev] {
					d.online = false
				}
			}
		}
		if t == offlineEnd {
			for _, d := range devs {
				if d.id != deviceNames[offlineDev] {
					continue
				}
				d.online = true
				d.recoverTarget = int64(len(serverMsgs))
				d.recoverBytes = -d.reqBytes - d.respBytes // 差分成补齐期间的字节
				if p == protoPush {
					doFullSync(d, t+1) // 只推：断线丢失只能靠重连全量补齐
				} else {
					doPull(d, t+1) // 拉取协议：重连后立刻拉一次增量
				}
			}
		}
		// 记录补齐耗时（以"补齐到断线时刻的 seq"为准，新消息不阻塞判定）
		if t > offlineEnd {
			for _, d := range devs {
				if d.id != deviceNames[offlineDev] || d.recoverDurMs != 0 || d.recoverTarget == 0 {
					continue
				}
				if d.cursor >= d.recoverTarget {
					d.recoverDurMs = t - offlineEnd
					d.recoverBytes += d.reqBytes + d.respBytes
					res.recoverDurMs = append(res.recoverDurMs, d.recoverDurMs)
					res.recoverBytes += d.recoverBytes
					res.recoverOK++
				}
			}
		}
		_ = syncReq
	}

	// 收尾：能拉的最后拉一次；只推协议没有补救手段，缺口保留
	for _, d := range devs {
		if p != protoPush && d.online {
			doPull(d, durationMs+2000)
		}
	}

	// 汇总
	res.totalMsgs = int64(len(serverMsgs))
	for _, d := range devs {
		res.bytes += d.respBytes + d.reqBytes
		res.reqBytes += d.reqBytes
		res.deliveries += d.deliveries
		res.dups += d.dupDeliveries
		res.pulls += d.pulls
		res.fullSyncs += d.fullSyncs
		res.outOfOrder += d.outOfOrder
		res.dupPush += d.dupPush
		res.dupSync += d.dupSync
		res.known += d.applied
	}
	res.missing = len(devs)*len(serverMsgs) - res.known
	// 延迟：只统计"发布时该设备在线"的消息，离线区间的消息单独用补齐耗时衡量
	for _, d := range devs {
		for _, m := range serverMsgs {
			at, ok := d.applyAt[m.Seq]
			if !ok {
				continue
			}
			if m.Ts >= offlineStart && m.Ts < offlineEnd {
				continue
			}
			res.lat = append(res.lat, at-m.Ts)
		}
	}
	sort.Slice(res.lat, func(i, j int) bool { return res.lat[i] < res.lat[j] })
	return res
}

func pct(v []int64, p float64) int64 {
	if len(v) == 0 {
		return -1
	}
	return v[int(float64(len(v)-1)*p)]
}

func main() {
	fmt.Println("环境: macOS 15.6 / Apple M1 / Go 1.27.1")
	fmt.Printf("负载: 1 用户 × 3 设备, %d 秒, %d 条消息(每 %dms 一条), %s 在 120–150s 断线 30s\n",
		durationMs/1000, msgTotal, publishEvery, deviceNames[offlineDev])
	fmt.Printf("网络: 推送丢包 %.0f%%, 通道重复 %.0f%%, 推送延迟 5–25ms; 只拉轮询 %dms; 推叫醒兜底 %dms\n\n",
		pushLoss*100, dupRate*100, pollMs, backstopMs)

	results := []result{run(protoPush, 7), run(protoPull, 7), run(protoPushPull, 7)}

	fmt.Println("=== 三种同步协议对比（同一负载、同一随机种子）===")
	fmt.Printf("%-16s %10s %8s %7s %8s %8s %7s %16s\n",
		"协议", "总流量(B)", "数据重复率", "未送达", "P50延迟", "P99延迟", "请求数", "断线补齐(1Mbps折算)")
	for _, r := range results {
		dupRatePct := 0.0
		if r.deliveries > 0 {
			dupRatePct = 100 * float64(r.dups) / float64(r.deliveries)
		}
		rec := "-"
		if r.recoverOK > 0 {
			// 按 1Mbps 弱网链路 + 30ms RTT 折算补齐耗时：ms = 字节×8/1000 + 30，推导值
			recMs := int64(float64(r.recoverBytes)*8/1000.0) + 30
			rec = fmt.Sprintf("%dms/%dB", recMs, r.recoverBytes)
		}
		fmt.Printf("%-16s %10d %7.1f%% %7d %7dms %7dms %7d %16s\n",
			r.name, r.bytes, dupRatePct, r.missing, pct(r.lat, 0.50), pct(r.lat, 0.99), r.pulls+r.pushes, rec)
	}
	fmt.Println()
	for _, r := range results {
		fmt.Printf("%s: 推送到达=%d 拉取请求=%d 全量同步=%d 消息投递=%d(重复 %d = 通道 %d + 全量重放 %d) 乱序到达=%d 已知消息=%d/%d\n",
			r.name, r.pushes, r.pulls, r.fullSyncs, r.deliveries, r.dups, r.dupPush, r.dupSync,
			r.outOfOrder, r.known, len(deviceNames)*int(r.totalMsgs))
	}
}
