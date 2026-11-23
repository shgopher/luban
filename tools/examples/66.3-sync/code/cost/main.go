// 实验 C：全量快照 vs 增量同步的真实成本与分界点
// 场景：10 万条历史消息、20 个会话、1 个新设备首登（全量）vs 老设备断线 1 小时后回来（增量）。
// 测法：真实 JSON 编码（含 HTTP 往返，httptest 回环），记录字节数、编码耗时、gzip 后字节、HTTP 往返耗时。
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"time"
)

// 随机正文：避免"重复串压缩比虚高"，让 gzip 数字接近真实业务文本
var alpha = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randBody(rng *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}

const (
	historyMsgs = 100_000 // 历史消息总量
	convs       = 20
	msgBodyLen  = 48
	offlineRate = 0.5 // 建模假设：单用户 0.5 条/秒（约 1.8 千条/小时）
)

type Msg struct {
	MsgID  string `json:"msg_id"`
	ConvID string `json:"conv_id"`
	Seq    int64  `json:"seq"`
	Sender string `json:"sender"`
	Body   string `json:"body"`
	Ts     int64  `json:"ts_ms"`
}

type Conv struct {
	ConvID  string `json:"conv_id"`
	LastSeq int64  `json:"last_seq"`
	Unread  int64  `json:"unread"`
	Pinned  bool   `json:"pinned"`
}

// Change 是用户维度变更流水里的"瘦条目"：只带定位信息，正文由会话游标拉。
type Change struct {
	ChangeSeq int64  `json:"change_seq"`
	Kind      string `json:"kind"`
	ConvID    string `json:"conv_id"`
	Seq       int64  `json:"seq"`
	MsgID     string `json:"msg_id"`
}

func buildMsgs() ([]Msg, []Conv) {
	rng := rand.New(rand.NewSource(20261121))
	msgs := make([]Msg, 0, historyMsgs)
	convList := make([]Conv, 0, convs)
	for c := 0; c < convs; c++ {
		cid := fmt.Sprintf("c%02d", c)
		per := historyMsgs / convs
		for i := 0; i < per; i++ {
			msgs = append(msgs, Msg{
				MsgID:  fmt.Sprintf("m_%s_%06d", cid, i+1),
				ConvID: cid,
				Seq:    int64(i + 1),
				Sender: fmt.Sprintf("u%04d", (i*7)%1000),
				Body:   randBody(rng, msgBodyLen),
				Ts:     int64(1750000000000 + i*137),
			})
		}
		convList = append(convList, Conv{ConvID: cid, LastSeq: int64(per), Unread: int64(per % 7)})
	}
	return msgs, convList
}

func gzipSize(b []byte) (int, int64) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	t0 := time.Now()
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Len(), time.Since(t0).Microseconds()
}

func median(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]int64{}, v...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

// measure 对一个响应体做 5 轮编码，返回中位字节数与中位编码耗时（微秒）
func measure(name string, build func() any, n int) (bytes int, encUs int64) {
	var enc []int64
	var sz int
	for k := 0; k < n; k++ {
		v := build()
		t0 := time.Now()
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		enc = append(enc, time.Since(t0).Microseconds())
		sz = len(b)
	}
	return sz, median(enc)
}

// httpRoundtrip 用 httptest 起一个真实 HTTP 服务，测客户端拿到并解码全量响应的耗时
func httpRoundtrip(payload []byte) (ms int64, recvBytes int) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	var times []int64
	var n int
	for k := 0; k < 5; k++ {
		t0 := time.Now()
		resp, err := http.Get(srv.URL + "/sync?change_cursor=0")
		if err != nil {
			panic(err)
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			panic(err)
		}
		times = append(times, time.Since(t0).Milliseconds())
		n = len(b)
	}
	return median(times), n
}

func main() {
	msgs, convList := buildMsgs()
	fmt.Printf("环境: macOS 15.6 / Apple M1 / Go 1.27.1; 历史 %d 条消息, %d 个会话; 消息正文 %d 字符\n\n",
		len(msgs), convs, msgBodyLen)

	// ---------- 1) 全量快照：新设备首登 ----------
	fullBytes, fullEncUs := measure("full", func() any {
		return map[string]any{
			"snapshot_seq":  int64(historyMsgs),
			"conversations": convList,
			"messages":      msgs,
			"has_more":      false,
		}
	}, 5)
	fullGzip, fullGzipUs := gzipSize(mustJSON(map[string]any{
		"snapshot_seq": int64(historyMsgs), "conversations": convList, "messages": msgs, "has_more": false,
	}))
	fullHTTPMs, fullRecv := httpRoundtrip(mustJSON(map[string]any{
		"snapshot_seq": int64(historyMsgs), "conversations": convList, "messages": msgs, "has_more": false,
	}))
	fmt.Println("=== 1) 新设备首登：全量快照 ===")
	fmt.Printf("消息体 JSON 合计: %.2f MB (%d B), 编码耗时中位 %d µs\n",
		float64(fullBytes)/1024/1024, fullBytes, fullEncUs)
	fmt.Printf("gzip 后: %.2f MB (%d B, 压缩比 %.1fx), gzip 耗时 %d µs\n",
		float64(fullGzip)/1024/1024, fullGzip, float64(fullBytes)/float64(fullGzip), fullGzipUs)
	fmt.Printf("本机回环 HTTP 全量拉取: %d ms, 收到 %d B（回环无带宽约束，真实弱网见下）\n", fullHTTPMs, fullRecv)
	fmt.Printf("按 1Mbps 弱网链路折算: %.1f s（字节数 × 8 / 1Mbps）\n\n", float64(fullBytes)*8/1_000_000)

	// ---------- 2) 断线 1 小时增量 ----------
	offlineChanges := int(offlineRate * 3600)
	fmt.Printf("=== 2) 断线 1 小时增量（建模：%.1f 条/秒 => %d 条变更）===\n", offlineRate, offlineChanges)
	rng := rand.New(rand.NewSource(20261122))
	inc := func(n int) (int, int64) {
		return measure("inc", func() any {
			chs := make([]Change, 0, n)
			ims := make([]Msg, 0, n)
			for i := 0; i < n; i++ {
				cid := fmt.Sprintf("c%02d", i%convs)
				seq := int64(historyMsgs/convs + i)
				chs = append(chs, Change{ChangeSeq: int64(historyMsgs + i), Kind: "msg", ConvID: cid, Seq: seq, MsgID: fmt.Sprintf("m_%s_%06d", cid, seq)})
				ims = append(ims, Msg{MsgID: fmt.Sprintf("m_%s_%06d", cid, seq), ConvID: cid, Seq: seq, Sender: "u0001", Body: randBody(rng, msgBodyLen), Ts: int64(1750000000000 + i*137)})
			}
			return map[string]any{"change_cursor": int64(historyMsgs + n), "changes": chs, "messages": ims, "has_more": false}
		}, 5)
	}
	incBytes, incEncUs := inc(offlineChanges)
	incGzip, incGzipUs := gzipSize(mustJSON(map[string]any{
		"change_cursor": int64(historyMsgs + offlineChanges),
		"changes":       makeChanges(offlineChanges),
		"messages":      makeMsgs(offlineChanges),
		"has_more":      false,
	}))
	fmt.Printf("增量响应: %.1f KB (%d B), 编码耗时中位 %d µs, gzip 后 %.1f KB (%d B, %.1fx, %d µs)\n",
		float64(incBytes)/1024, incBytes, incEncUs, float64(incGzip)/1024, incGzip,
		float64(incBytes)/float64(incGzip), incGzipUs)
	fmt.Printf("全量/增量字节比: %.1fx; 按 1Mbps 折算增量 1h 补齐 ≈ %.1f s\n\n",
		float64(fullBytes)/float64(incBytes), float64(incBytes)*8/1_000_000)

	// ---------- 3) 分界点：增量涨到多少条追平全量 ----------
	fmt.Println("=== 3) 分界点（增量字节数追平全量快照）===")
	perChange := float64(incBytes) / float64(offlineChanges)
	breakEvenN := int(float64(fullBytes) / perChange)
	fmt.Printf("实测每条变更的增量成本 ≈ %.0f B/条（含 change 瘦条目 + 消息正文 + JSON 开销）\n", perChange)
	fmt.Printf("分界点 ≈ %d 条变更（按 %.1f 条/秒 ≈ %.1f 小时离线）\n",
		breakEvenN, float64(offlineRate), float64(breakEvenN)/float64(offlineRate)/3600)
	gzPerChange := float64(incGzip) / float64(offlineChanges)
	fmt.Printf("gzip 通道下：全量 %.2f MB / 每条增量 %.0f B => 分界点 ≈ %d 条（压缩后全量优势变小）\n\n",
		float64(fullGzip)/1024/1024, gzPerChange, int(float64(fullGzip)/gzPerChange))

	// ---------- 4) 曲线：不同离线时长/变更数下的增量字节 ----------
	fmt.Println("=== 4) 增量字节随离线变更数增长（每 2 小时一档）===")
	fmt.Printf("%-10s %-10s %14s %10s\n", "离线时长", "变更数", "增量字节", "相对全量")
	for _, hours := range []float64{0.5, 1, 2, 4, 8, 24, 72} {
		n := int(offlineRate * 3600 * hours)
		b, _ := inc(n)
		fmt.Printf("%-10.1fh %-10d %14d %9.2f%%\n", hours, n, b, 100*float64(b)/float64(fullBytes))
	}

	// 说明 change 流水的保留期成本：change log 只存瘦条目，不存消息正文
	thin := len(mustJSON(Change{ChangeSeq: 12345678, Kind: "msg",
		ConvID: "c07", Seq: 4321, MsgID: "m_c07_004321"}))
	perDay := int(float64(offlineRate) * 86400)
	fmt.Printf("\n=== 5) change 流水存储成本（只存瘦条目，不存正文）===\n")
	fmt.Printf("单条 change 瘦条目 JSON = %d B; 重度用户(%.1f 条/秒) 每天 %d 条, 7 天 TTL ≈ %d 条 ≈ %.1f MB\n",
		thin, float64(offlineRate), perDay, perDay*7, float64(perDay*7*thin)/1024/1024)
	avgPerDay := 200 // 建模假设：普通用户每天 200 条变更
	fmt.Printf("若 100 万 DAU 平均 %d 条/天 => 100 万 × %d × 7 天 × %d B ≈ %.0f GB（需分片 + 按活跃度 TTL/落盘）\n",
		avgPerDay, avgPerDay, thin, float64(1_000_000*avgPerDay*7*thin)/1024/1024/1024)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func makeChanges(n int) []Change {
	chs := make([]Change, 0, n)
	for i := 0; i < n; i++ {
		cid := fmt.Sprintf("c%02d", i%convs)
		seq := int64(historyMsgs/convs + i)
		chs = append(chs, Change{ChangeSeq: int64(historyMsgs + i), Kind: "msg", ConvID: cid, Seq: seq, MsgID: fmt.Sprintf("m_%s_%06d", cid, seq)})
	}
	return chs
}

func makeMsgs(n int) []Msg {
	rng := rand.New(rand.NewSource(20261122))
	ims := make([]Msg, 0, n)
	for i := 0; i < n; i++ {
		cid := fmt.Sprintf("c%02d", i%convs)
		seq := int64(historyMsgs/convs + i)
		ims = append(ims, Msg{MsgID: fmt.Sprintf("m_%s_%06d", cid, seq), ConvID: cid, Seq: seq, Sender: "u0001",
			Body: randBody(rng, msgBodyLen), Ts: int64(1750000000000 + i*137)})
	}
	return ims
}
