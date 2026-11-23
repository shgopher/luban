// 同步 API 最小可运行实现：首登快照、增量拉取、游标过期回退、已读 max 合并、踢下线、限流 429。
// 端口 18103（本章分配）。纯内存，仅用于验证协议语义，不是生产实现。
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Msg struct {
	MsgID  string `json:"msg_id"`
	ConvID string `json:"conv_id"`
	Seq    int64  `json:"seq"`
	Sender string `json:"sender"`
	Body   string `json:"body"`
	Ts     int64  `json:"ts_ms"`
}

type Change struct {
	ChangeSeq  int64  `json:"change_seq"`
	Kind       string `json:"kind"`
	ConvID     string `json:"conv_id,omitempty"`
	Seq        int64  `json:"seq,omitempty"`
	MsgID      string `json:"msg_id,omitempty"`
	ReadSeq    int64  `json:"read_seq,omitempty"`
	SettingKey string `json:"setting_key,omitempty"`
	SettingVal string `json:"setting_val,omitempty"`
}

type Conv struct {
	ConvID  string `json:"conv_id"`
	LastSeq int64  `json:"last_seq"`
	Unread  int64  `json:"unread"`
	Pinned  bool   `json:"pinned"`
}

type Server struct {
	mu       sync.Mutex
	msgs     map[string][]Msg
	convs    map[string]*Conv
	readSeq  map[string]int64
	changes  []Change
	seq      int64
	retain   int // change 流水保留条数（生产按 7 天 TTL）
	devices  map[string]*Device
	rlWindow map[string][]int64 // device -> 最近请求时间戳（毫秒）
}

type Device struct {
	ID     string `json:"device_id"`
	Kicked bool   `json:"kicked"`
	Model  string `json:"model"`
}

func newServer() *Server {
	s := &Server{
		msgs: map[string][]Msg{}, convs: map[string]*Conv{}, readSeq: map[string]int64{},
		retain: 6, devices: map[string]*Device{}, rlWindow: map[string][]int64{},
	}
	for _, cid := range []string{"c1", "c2", "c3"} {
		conv := &Conv{ConvID: cid}
		// 预置 40 条历史消息：每会话 40 条，seq 1..40
		for i := int64(1); i <= 40; i++ {
			m := Msg{MsgID: fmt.Sprintf("m_%s_%04d", cid, i), ConvID: cid, Seq: i,
				Sender: fmt.Sprintf("u%04d", i%3+2), Body: fmt.Sprintf("history-%02d", i), Ts: 1750000000000 + i*1000}
			s.msgs[cid] = append(s.msgs[cid], m)
			conv.LastSeq = i
			s.seq++
			s.changes = append(s.changes, Change{ChangeSeq: s.seq, Kind: "msg", ConvID: cid, Seq: i, MsgID: m.MsgID})
		}
		s.convs[cid] = conv
	}
	for i, id := range []string{"t_phone", "t_pad", "t_pc", "t_burst"} {
		s.devices[id] = &Device{ID: id, Model: []string{"iPhone", "iPad", "MacBook", "Pixel"}[i]}
	}
	return s
}

func (s *Server) append(c Change) Change {
	s.seq++
	c.ChangeSeq = s.seq
	s.changes = append(s.changes, c)
	if len(s.changes) > s.retain*10 {
		s.changes = s.changes[len(s.changes)-s.retain*10:]
	}
	return c
}

func (s *Server) lastSeq(conv string) int64 { return s.convs[conv].LastSeq }

// rateLimit 每设备 1 秒 3 次；超限返回 false，走 429。
func (s *Server) rateLimit(dev string) bool {
	now := time.Now().UnixMilli()
	w := s.rlWindow[dev]
	keep := w[:0]
	for _, t := range w {
		if now-t < 1000 {
			keep = append(keep, t)
		}
	}
	s.rlWindow[dev] = append(keep, now)
	return len(s.rlWindow[dev]) <= 3
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// parseCursors "c1:10,c2:5"
func parseCursors(s string) map[string]int64 {
	out := map[string]int64{}
	if s == "" {
		return out
	}
	for _, kv := range strings.Split(s, ",") {
		p := strings.Split(kv, ":")
		if len(p) != 2 {
			continue
		}
		n, _ := strconv.ParseInt(p[1], 10, 64)
		out[p[0]] = n
	}
	return out
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dev := q.Get("device_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[dev]
	if !ok {
		writeJSON(w, 404, map[string]any{"error": "unknown_device", "device_id": dev})
		return
	}
	if d.Kicked {
		writeJSON(w, 401, map[string]any{"error": "device_kicked", "device_id": dev,
			"hint": "令牌已失效，需重新登录（服务端同时断开 66.1 的连接）"})
		return
	}
	if !s.rateLimit(dev) {
		w.Header().Set("Retry-After", "1")
		writeJSON(w, 429, map[string]any{"error": "rate_limited", "retry_after_ms": 1000})
		return
	}
	cur, _ := strconv.ParseInt(q.Get("change_cursor"), 10, 64)
	convCursors := parseCursors(q.Get("conv_cursors"))

	// 首登：snapshot=1 => 快照（会话列表 + 每会话最近 3 条 + snapshot_seq），后续从 snapshot_seq 开始拉增量
	if q.Get("snapshot") == "1" {
		msgs := map[string][]Msg{}
		nextCur := map[string]int64{}
		hasMore := false
		for _, cid := range []string{"c1", "c2", "c3"} {
			all := s.msgs[cid]
			start := len(all) - 3
			if start < 0 {
				start = 0
			}
			msgs[cid] = all[start:]
			nextCur[cid] = int64(len(all))
			if start > 0 {
				hasMore = true
			}
		}
		convList := []Conv{}
		for _, cid := range []string{"c1", "c2", "c3"} {
			c := *s.convs[cid]
			c.Unread = c.LastSeq - s.readSeq[cid]
			convList = append(convList, c)
		}
		writeJSON(w, 200, map[string]any{
			"snapshot_seq": s.seq, "conversations": convList, "messages": msgs,
			"next_conv_cursors": nextCur, "has_more": hasMore, "change_cursor": s.seq,
		})
		return
	}

	// 游标过期（超出 change 流水保留窗口）=> 通知客户端回退到快照，而不是静默丢消息
	oldest := int64(1)
	if len(s.changes) > 0 {
		oldest = s.changes[0].ChangeSeq
	}
	if cur > 0 && cur < oldest-1 {
		writeJSON(w, 200, map[string]any{"reset_required": true, "snapshot_seq": s.seq,
			"reason": "change_cursor_expired", "oldest_change_seq": oldest})
		return
	}

	// 增量：changes 按 seq 顺序，消息按会话独立游标拉
	changes := []Change{}
	for _, c := range s.changes {
		if c.ChangeSeq > cur {
			changes = append(changes, c)
		}
	}
	hasMore := false
	if len(changes) > 10 {
		changes = changes[:10]
		hasMore = true
	}
	msgs := map[string][]Msg{}
	for _, cid := range []string{"c1", "c2", "c3"} {
		all := s.msgs[cid]
		start := convCursors[cid]
		if start < 0 {
			start = 0
		}
		if start > int64(len(all)) {
			start = int64(len(all))
		}
		n := int64(2) // 每会话每页 2 条，演示分页
		end := start + n
		if end > int64(len(all)) {
			end = int64(len(all))
		}
		if start < end {
			msgs[cid] = all[start:end]
		}
		if end < int64(len(all)) {
			hasMore = true
		}
	}
	newCur := cur
	if len(changes) > 0 {
		newCur = changes[len(changes)-1].ChangeSeq
	}
	writeJSON(w, 200, map[string]any{
		"snapshot_seq": s.seq, "change_cursor": newCur, "changes": changes,
		"messages": msgs, "has_more": hasMore,
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cid := q.Get("conv")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.convs[cid]; !ok {
		writeJSON(w, 400, map[string]any{"error": "unknown_conv"})
		return
	}
	m := Msg{MsgID: fmt.Sprintf("m_%s_%04d", cid, s.lastSeq(cid)+1), ConvID: cid,
		Seq: s.lastSeq(cid) + 1, Sender: q.Get("sender"), Body: q.Get("text"), Ts: time.Now().UnixMilli()}
	s.msgs[cid] = append(s.msgs[cid], m)
	s.convs[cid].LastSeq = m.Seq
	c := s.append(Change{Kind: "msg", ConvID: cid, Seq: m.Seq, MsgID: m.MsgID})
	writeJSON(w, 200, map[string]any{"msg": m, "change_seq": c.ChangeSeq})
}

// handleRead 已读 max 合并：更旧的上报直接丢弃（幂等、无需锁）
func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cid := q.Get("conv")
	n, _ := strconv.ParseInt(q.Get("read_seq"), 10, 64)
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= s.readSeq[cid] {
		writeJSON(w, 200, map[string]any{"applied": false, "read_seq": s.readSeq[cid],
			"reason": "stale_read_report_dropped"})
		return
	}
	s.readSeq[cid] = n
	c := s.append(Change{Kind: "read", ConvID: cid, ReadSeq: n})
	writeJSON(w, 200, map[string]any{"applied": true, "read_seq": n, "change_seq": c.ChangeSeq})
}

func (s *Server) handleKick(w http.ResponseWriter, r *http.Request) {
	dev := r.URL.Query().Get("device_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[dev]
	if !ok {
		writeJSON(w, 404, map[string]any{"error": "unknown_device"})
		return
	}
	d.Kicked = true
	// 生产动作：删令牌（49.5）+ 断开网关连接（66.1）+ 清理该设备游标
	writeJSON(w, 200, map[string]any{"kicked": dev, "actions": []string{
		"revoke_token", "close_gateway_conn", "keep_cursor_for_7d"}})
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []Device{}
	for _, d := range s.devices {
		list = append(list, *d)
	}
	writeJSON(w, 200, map[string]any{"devices": list})
}

func main() {
	s := newServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/sync", s.handleSync)
	mux.HandleFunc("/send", s.handleSend)
	mux.HandleFunc("/read", s.handleRead)
	mux.HandleFunc("/kick", s.handleKick)
	mux.HandleFunc("/devices", s.handleDevices)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "seq": s.seq})
	})
	addr := "127.0.0.1:18103"
	fmt.Println("syncapi listening on", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
