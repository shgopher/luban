// 实验 A：3 设备 + 随机断网/乱序/重复推送的多端同步模拟
// 协议：每设备一个游标（change_cursor + 每会话 conv_cursor），推送只带水位（叫醒），
// 数据一律靠拉取增量补全；客户端按 msg_id 去重、按 conv_seq 排序、游标只走连续前缀。
// 断言：I1 不丢 / I2 不重 / I3 不乱序 / I4 已读单调 / I5 会话列表收敛。
package main

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
)

const (
	simEndMs   = 300_000 // 5 分钟虚拟时间
	maxMsgs    = 10_000  // 消息总量目标
	recoverMax = 600_000 // 收敛阶段最长虚拟时间
	pushLoss   = 0.20    // 推送丢包率
	syncLoss   = 0.20    // 同步请求/响应失败率
	dupRate    = 0.10    // 通道重复率
)

var (
	convIDs   = []string{"c1", "c2", "c3"}
	deviceIDs = []string{"d_phone", "d_pad", "d_pc"}
)

// ---------- 数据模型 ----------

// Msg 是一条消息。Seq 是会话内单调序号（conv_seq），MsgID 是全局唯一 ID。
type Msg struct {
	MsgID  string `json:"msg_id"`
	ConvID string `json:"conv_id"`
	Seq    int64  `json:"seq"`
	Sender string `json:"sender"`
	Body   string `json:"body"`
	Ts     int64  `json:"ts_ms"`
}

// Change 是用户维度的变更流水（user_change_seq）。消息在流水里只放"有更新"的瘦条目，
// 正文靠会话游标另拉；已读/会话元数据/设置则直接带值。
type Change struct {
	ChangeSeq   int64  `json:"change_seq"`
	Kind        string `json:"kind"` // msg | read | conv | setting
	ConvID      string `json:"conv_id,omitempty"`
	Seq         int64  `json:"seq,omitempty"`
	MsgID       string `json:"msg_id,omitempty"`
	ReadSeq     int64  `json:"read_seq,omitempty"`
	ConvVersion int64  `json:"conv_version,omitempty"`
	Pinned      bool   `json:"pinned,omitempty"`
	SettingKey  string `json:"setting_key,omitempty"`
	SettingVal  string `json:"setting_value,omitempty"`
}

// SyncResp 同步响应：增量变更 + 每会话消息增量 + has_more + snapshot_seq
type SyncResp struct {
	Changes     []Change         `json:"changes"`
	Messages    map[string][]Msg `json:"messages"`
	HasMore     bool             `json:"has_more"`
	SnapshotSeq int64            `json:"snapshot_seq"`
	EncodedSize int              `json:"-"`
}

type ConvState struct {
	ID          string
	LastSeq     int64
	LastMsgID   string
	ReadSeq     int64
	Pinned      bool
	ConvVersion int64
}

type SettingState struct {
	Val     string
	Version int64
}

// ---------- 事件驱动 ----------

type evKind int

const (
	evOp   evKind = iota
	evSync        // 周期性兜底同步
	evNow         // 推送触发的即时一次性同步
	evPush
	evOff
	evOn
)

type event struct {
	at   int64
	kind evKind
	dev  int
}

type evHeap []event

func (h evHeap) Len() int           { return len(h) }
func (h evHeap) Less(i, j int) bool { return h[i].at < h[j].at }
func (h evHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *evHeap) Push(x any)        { *h = append(*h, x.(event)) }
func (h *evHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// ---------- 服务端 ----------

type Server struct {
	rng      *rand.Rand
	msgs     map[string][]Msg
	convs    map[string]*ConvState
	changes  []Change
	readSeq  map[string]int64 // 服务端已读（max 合并）
	settings map[string]*SettingState
	seq      int64

	devices  []*Device
	events   *evHeap
	now      int64
	reliable bool // 收敛阶段关闭随机故障

	msgCount int64

	// 统计
	pushSent, pushLost, pushDup, pushDelivered int64
	pushBytes                                  int64
	syncRequests, syncBytes                    int64
	readReports, readStale                     int64
}

func newServer(rng *rand.Rand) *Server {
	s := &Server{
		rng:      rng,
		msgs:     map[string][]Msg{},
		convs:    map[string]*ConvState{},
		readSeq:  map[string]int64{},
		settings: map[string]*SettingState{},
	}
	for _, c := range convIDs {
		s.convs[c] = &ConvState{ID: c}
	}
	for _, k := range []string{"notify", "theme"} {
		s.settings[k] = &SettingState{Val: "default", Version: 0}
	}
	return s
}

func (s *Server) append(change Change) Change {
	s.seq++
	change.ChangeSeq = s.seq
	s.changes = append(s.changes, change)
	return change
}

// Send 分配会话内单调 seq，写消息 + 变更流水，然后推送唤醒在线设备。
func (s *Server) Send(sender, conv, body string) Msg {
	m := Msg{
		MsgID:  fmt.Sprintf("m_%s_%06d", conv, s.convs[conv].LastSeq+1),
		ConvID: conv,
		Seq:    s.convs[conv].LastSeq + 1,
		Sender: sender,
		Body:   body,
		Ts:     s.now,
	}
	s.msgs[conv] = append(s.msgs[conv], m)
	cs := s.convs[conv]
	cs.LastSeq, cs.LastMsgID = m.Seq, m.MsgID
	s.msgCount++
	c := s.append(Change{Kind: "msg", ConvID: conv, Seq: m.Seq, MsgID: m.MsgID})
	s.pushNotify(c.ChangeSeq)
	return m
}

// MarkRead 已读位置取 max 合并：回退的 reported 值直接被丢弃，天然幂等、无需锁。
func (s *Server) MarkRead(conv string, readSeq int64) {
	s.readReports++
	if readSeq <= s.readSeq[conv] { // max 合并：更旧的已读上报直接丢弃，天然幂等
		s.readStale++
		return
	}
	s.readSeq[conv] = readSeq
	s.convs[conv].ReadSeq = readSeq
	c := s.append(Change{Kind: "read", ConvID: conv, ReadSeq: readSeq})
	s.pushNotify(c.ChangeSeq)
}

// SetSetting 设置类数据用服务端序号做 LWW 版本，不用客户端时钟。
func (s *Server) SetSetting(dev, key, val string) {
	st := s.settings[key]
	v := s.seq + 1
	st.Val, st.Version = val, v
	c := s.append(Change{Kind: "setting", SettingKey: key, SettingVal: val})
	s.pushNotify(c.ChangeSeq)
}

// TogglePin 会话元数据 LWW：版本号 = 服务端 change_seq，旧版本写入被丢弃。
func (s *Server) TogglePin(dev, conv string) {
	cs := s.convs[conv]
	cs.ConvVersion = s.seq + 1
	cs.Pinned = !cs.Pinned
	c := s.append(Change{Kind: "conv", ConvID: conv, ConvVersion: cs.ConvVersion, Pinned: cs.Pinned})
	s.pushNotify(c.ChangeSeq)
}

// pushNotify 推送"只带水位"：payload 里没有消息正文，只有 change_seq。
// 随机丢 20%、重复 10%、延迟 1~15ms，且事件堆会自然产生乱序到达。
func (s *Server) pushNotify(changeSeq int64) {
	payload, _ := json.Marshal(map[string]int64{"change_seq": changeSeq}) // 推只带水位
	for i, d := range s.devices {
		if !d.online {
			continue
		}
		s.pushSent++
		if !s.reliable && s.rng.Float64() < pushLoss {
			s.pushLost++
			continue
		}
		n := 1
		if !s.reliable && s.rng.Float64() < dupRate {
			n = 2
			s.pushDup++
		}
		for k := 0; k < n; k++ {
			lat := int64(1 + s.rng.Intn(15))
			s.pushBytes += int64(len(payload))
			s.pushDelivered++
			heap.Push(s.events, event{at: s.now + lat, kind: evPush, dev: i})
		}
	}
}

// HandleSync 返回 (change_seq, conv_seq] 的变更增量 + 每会话消息增量。
// 通道是 at-least-once：响应里随机重复 10% 条目、消息数组乱序，客户端必须幂等。
func (s *Server) HandleSync(dev int, changeCursor int64, convCursors map[string]int64) SyncResp {
	s.syncRequests++
	maxChanges := 1 + s.rng.Intn(40)
	start := changeCursor
	if start < 0 {
		start = 0
	}
	end := start + int64(maxChanges)
	if end > s.seq {
		end = s.seq
	}
	resp := SyncResp{
		Changes:     append([]Change{}, s.changes[start:end]...),
		Messages:    map[string][]Msg{},
		SnapshotSeq: s.seq,
	}
	hasMore := end < s.seq
	for _, cid := range convIDs {
		cur := convCursors[cid]
		all := s.msgs[cid]
		if cur < int64(len(all)) {
			n := 1 + s.rng.Intn(50)
			lo := int(cur)
			hi := lo + n
			if hi > len(all) {
				hi = len(all)
			}
			if lo < hi {
				resp.Messages[cid] = append([]Msg{}, all[lo:hi]...)
			}
			if hi < len(all) {
				hasMore = true
			}
		}
	}
	resp.HasMore = hasMore
	// 模拟 at-least-once 通道：乱序 + 重复（按固定 conv 顺序消费随机数，保证可复现）
	for _, cid := range convIDs {
		arr, ok := resp.Messages[cid]
		if !ok {
			continue
		}
		s.rng.Shuffle(len(arr), func(i, j int) { arr[i], arr[j] = arr[j], arr[i] })
		extra := []Msg{}
		for i := range arr {
			if s.rng.Float64() < dupRate {
				extra = append(extra, arr[i])
			}
		}
		resp.Messages[cid] = append(arr, extra...)
	}
	s.rng.Shuffle(len(resp.Changes), func(i, j int) { resp.Changes[i], resp.Changes[j] = resp.Changes[j], resp.Changes[i] })
	extra := []Change{}
	for i := range resp.Changes {
		if s.rng.Float64() < dupRate {
			extra = append(extra, resp.Changes[i])
		}
	}
	resp.Changes = append(resp.Changes, extra...)
	b, _ := json.Marshal(resp)
	resp.EncodedSize = len(b)
	s.syncBytes += int64(len(b))
	return resp
}

// ---------- 设备 ----------

type Device struct {
	ID     string
	idx    int
	online bool

	changeCursor   int64
	appliedChanges map[int64]bool
	convCursor     map[string]int64

	msgs           map[string]map[int64]Msg
	seen           map[string]bool
	remoteRead     map[string]int64
	readView       map[string]int64
	convVersion    map[string]int64
	pinned         map[string]bool
	settingVersion map[string]int64
	settingVal     map[string]string
	applyTime      map[string]int64 // msg_id -> 首次可见的虚拟毫秒

	// 统计
	syncAttempts, syncFails, syncOK, pages  int64
	syncFailOffline, syncFailNet, offlineMs int64
	offlineSince                            int64
	msgsFetched, dupDropped                 int64
	oooApplied, pushRecv, pushTrig          int64
	bytesIn                                 int64
	readRegressions                         int64
	lastSyncAt                              int64
}

func newDevice(id string, idx int) *Device {
	d := &Device{
		ID: id, idx: idx, online: true,
		appliedChanges: map[int64]bool{},
		convCursor:     map[string]int64{},
		msgs:           map[string]map[int64]Msg{},
		seen:           map[string]bool{},
		remoteRead:     map[string]int64{},
		readView:       map[string]int64{},
		convVersion:    map[string]int64{},
		pinned:         map[string]bool{},
		settingVersion: map[string]int64{},
		settingVal:     map[string]string{},
		applyTime:      map[string]int64{},
	}
	for _, c := range convIDs {
		d.msgs[c] = map[int64]Msg{}
	}
	return d
}

// send 发消息：本地 echo 立即可见（读己之写），服务端 seq 在 ack 里校准。
func (d *Device) send(s *Server, conv, body string) {
	m := s.Send(d.ID, conv, body)
	d.msgs[conv][m.Seq] = m
	d.seen[m.MsgID] = true
	d.applyTime[m.MsgID] = s.now
}

// markRead 把"已看到的最高连续 seq"上报；服务端取 max。
func (d *Device) markRead(s *Server, conv string) {
	if !d.online {
		return
	}
	r := d.convCursor[conv]
	if r <= d.remoteRead[conv] {
		return
	}
	s.MarkRead(conv, r)
	if r > d.readView[conv] {
		d.readView[conv] = r
	}
}

// Sync 一次同步：拉变更页 + 拉消息页，直到 has_more=false。翻页途中可能断网。
func (d *Device) Sync(s *Server, now int64) {
	if now-d.lastSyncAt < 50 { // 50ms 内合并，避免推送风暴放大请求
		return
	}
	d.lastSyncAt = now
	d.syncAttempts++
	if !d.online {
		d.syncFails++
		d.syncFailOffline++
		return
	}
	if !s.reliable && s.rng.Float64() < syncLoss {
		d.syncFails++
		d.syncFailNet++
		return
	}
	for page := 0; page < 20; page++ {
		if page > 0 && !s.reliable && s.rng.Float64() < syncLoss {
			d.syncFails++ // 翻到一半断网：已应用的页保留，游标停在连续前缀
			d.syncFailNet++
			return
		}
		resp := s.HandleSync(d.idx, d.changeCursor, d.convCursor)
		d.bytesIn += int64(resp.EncodedSize)
		if !s.reliable && s.rng.Float64() < syncLoss { // 响应在网络上丢了
			d.syncFails++
			d.syncFailNet++
			return
		}
		d.pages++
		d.apply(resp, now)
		if !resp.HasMore {
			d.syncOK++
			return
		}
	}
	d.syncOK++ // 未能在 20 页内追平，下次同步继续
}

// apply 幂等应用一个增量页：变更按 seq 去重、消息按 msg_id 去重，游标只推进连续前缀。
func (d *Device) apply(resp SyncResp, now int64) {
	before := map[string]int64{}
	for _, cid := range convIDs {
		before[cid] = d.readView[cid]
	}
	for _, c := range resp.Changes {
		if d.appliedChanges[c.ChangeSeq] {
			continue // 重复投递：按 change_seq 去重
		}
		d.appliedChanges[c.ChangeSeq] = true
		switch c.Kind {
		case "msg":
			// 瘦变更只表示"这个会话有新消息"，正文由 conv_cursor 拉取
		case "read":
			cur := d.remoteRead[c.ConvID]
			if c.ReadSeq > cur {
				cur = c.ReadSeq
			}
			if cur > d.readView[c.ConvID] {
				d.readView[c.ConvID] = cur // 只升不降
			}
			d.remoteRead[c.ConvID] = cur
		case "conv":
			if c.ConvVersion > d.convVersion[c.ConvID] {
				d.convVersion[c.ConvID] = c.ConvVersion
				d.pinned[c.ConvID] = c.Pinned
			}
		case "setting":
			// 版本号 = 服务端 change_seq，客户端时间不参与排序
			if c.ChangeSeq > d.settingVersion[c.SettingKey] {
				d.settingVersion[c.SettingKey] = c.ChangeSeq
				d.settingVal[c.SettingKey] = c.SettingVal
			}
		}
	}
	for _, cid := range convIDs {
		if d.readView[cid] < before[cid] {
			d.readRegressions++ // 已读回退：不变量 I4 会被判红
		}
	}
	for d.appliedChanges[d.changeCursor+1] { // 只推进连续前缀，不跳过缺口
		d.changeCursor++
	}

	for cid, arr := range resp.Messages {
		for _, m := range arr {
			d.msgsFetched++
			if d.seen[m.MsgID] {
				d.dupDropped++ // 按 msg_id 去重
				continue
			}
			if m.Seq != d.convCursor[cid]+1 {
				d.oooApplied++ // 乱序到达：不是下一个连续序号
			}
			d.msgs[cid][m.Seq] = m
			d.seen[m.MsgID] = true
			d.applyTime[m.MsgID] = now
		}
		// 游标只推进"连续前缀"，且必须在去重判断之外推进：
		// 本地 echo 已经写进 msgs 的消息会走上面的 continue 分支跳过，
		// 若把游标推进写在插入分支里，本地 echo 造成的连续段就永远推不动。
		for {
			next := d.convCursor[cid] + 1
			if _, ok := d.msgs[cid][next]; !ok {
				break
			}
			d.convCursor[cid] = next
		}
	}
}

// render 返回展示序列：按 conv_seq 升序（客户端绝不能按到达顺序 append 展示）。
func (d *Device) render(conv string) []Msg {
	seqs := make([]int64, 0, len(d.msgs[conv]))
	for seq := range d.msgs[conv] {
		seqs = append(seqs, seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	out := make([]Msg, 0, len(seqs))
	for _, s := range seqs {
		out = append(out, d.msgs[conv][s])
	}
	return out
}

func (d *Device) setting(key string) string {
	if v, ok := d.settingVal[key]; ok {
		return v
	}
	return "default"
}

// ---------- 模拟器 ----------

type sim struct {
	s   *Server
	ev  *evHeap
	rng *rand.Rand
}

func (sm *sim) schedule(at int64, kind evKind, dev int) {
	heap.Push(sm.ev, event{at: at, kind: kind, dev: dev})
}

func (sm *sim) run() {
	// 初始事件：每个设备的操作、周期同步、断网检测
	for i := range sm.s.devices {
		sm.schedule(sm.rng.Int63n(50), evOp, i)
		sm.schedule(int64(100+sm.rng.Intn(1900)), evSync, i)
		sm.schedule(int64(5_000+sm.rng.Intn(5_000)), evOff, i)
	}
	for sm.ev.Len() > 0 {
		e := heap.Pop(sm.ev).(event)
		if e.at > simEndMs {
			break
		}
		sm.s.now = e.at
		d := sm.s.devices[e.dev]
		switch e.kind {
		case evOp:
			if d.online {
				r := sm.rng.Float64()
				conv := convIDs[sm.rng.Intn(len(convIDs))]
				switch {
				case r < 0.55 && sm.s.msgCount < maxMsgs: // 发消息
					d.send(sm.s, conv, fmt.Sprintf("hello-%d", sm.s.msgCount))
				case r < 0.80: // 标已读
					d.markRead(sm.s, conv)
				case r < 0.92: // 改设置（LWW）
					k := []string{"notify", "theme"}[sm.rng.Intn(2)]
					sm.s.SetSetting(d.ID, k, fmt.Sprintf("%s-%d", d.ID, sm.rng.Intn(5)))
				default: // 置顶会话（LWW 元数据）
					sm.s.TogglePin(d.ID, conv)
				}
			}
			// 压测式负载：每设备约 33 次操作/秒，55% 发消息 => 3 设备合计约 55 条/秒（再乘在线率），跑到 10000 条即停发
			sm.schedule(e.at+20+sm.rng.Int63n(20), evOp, e.dev)
		case evSync: // 周期兜底：即使推送全丢，也能靠它最终追平
			d.Sync(sm.s, e.at)
			sm.schedule(e.at+1000+sm.rng.Int63n(2000), evSync, e.dev)
		case evNow: // 推送叫醒后的即时拉取：一次性事件，不产生新的周期链
			d.Sync(sm.s, e.at)
		case evPush:
			d.pushRecv++
			if d.online {
				d.pushTrig++
				sm.schedule(e.at+1+sm.rng.Int63n(20), evNow, e.dev)
			}
		case evOff:
			if d.online && sm.rng.Float64() < 0.20 {
				d.online = false
				d.offlineSince = e.at
				sm.schedule(e.at+5_000+sm.rng.Int63n(25_000), evOn, e.dev)
			} else {
				sm.schedule(e.at+10_000, evOff, e.dev)
			}
		case evOn:
			if !d.online && d.offlineSince > 0 && d.offlineSince < simEndMs {
				d.offlineMs += e.at - d.offlineSince
			}
			d.online = true
			d.offlineSince = 0
			sm.schedule(e.at+10_000, evOff, e.dev)
		}
	}
}

// recover 停掉负载后让所有设备在线，拉取直到没有新数据；返回收敛耗时（毫秒）。
func (sm *sim) recover() int64 {
	for _, d := range sm.s.devices {
		d.online = true
	}
	sm.s.reliable = true
	start := sm.s.now
	now := sm.s.now
	for round := 0; round < 20000; round++ {
		now += 20
		steady := true
		for _, d := range sm.s.devices {
			before := d.changeCursor
			d.Sync(sm.s, now)
			if d.changeCursor != before {
				steady = false
			}
			for _, cid := range convIDs {
				if d.convCursor[cid] != sm.s.convs[cid].LastSeq {
					steady = false
				}
			}
		}
		if steady {
			return now - start
		}
	}
	// 诊断：打印卡住的位置
	fmt.Printf("DEBUG converge stuck: server.seq=%d\n", sm.s.seq)
	for _, d := range sm.s.devices {
		fmt.Printf("  %s changeCursor=%d applied=%d\n", d.ID, d.changeCursor, len(d.appliedChanges))
		for _, cid := range convIDs {
			fmt.Printf("    %s convCursor=%d serverLastSeq=%d appliedMsgs=%d\n", cid, d.convCursor[cid], sm.s.convs[cid].LastSeq, len(d.msgs[cid]))
		}
	}
	panic("did not converge")
}

// ---------- 不变量断言 ----------

type report struct {
	pass  bool
	lines []string
}

func (r *report) add(pass bool, line string) {
	r.pass = r.pass && pass
	r.lines = append(r.lines, line)
}

func pct(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p)
	return sorted[i]
}

func main() {
	seed := int64(20261121)
	rng := rand.New(rand.NewSource(seed))
	s := newServer(rng)
	sm := &sim{s: s, ev: &evHeap{}, rng: rng}
	heap.Init(sm.ev)
	s.events = sm.ev // 服务端推送需要往同一条事件堆里排事件
	for i, id := range deviceIDs {
		s.devices = append(s.devices, newDevice(id, i))
	}
	sm.run()

	// 统计被观测到的故障动作（在收敛前记录）
	var pushRecv, pushTrig, dupDropped, ooo, fetches int64
	for _, d := range s.devices {
		pushRecv += d.pushRecv
		pushTrig += d.pushTrig
		dupDropped += d.dupDropped
		ooo += d.oooApplied
		fetches += d.msgsFetched
	}
	convMsgs := s.msgCount
	convergeMs := sm.recover()

	// 收敛延迟：每条消息从 publish 到所有设备可见的最大间隔
	lat := make([]int64, 0, convMsgs)
	missingLat := 0
	for _, cid := range convIDs {
		for _, m := range s.msgs[cid] {
			var worst int64 = -1
			all := true
			for _, d := range s.devices {
				t, ok := d.applyTime[m.MsgID]
				if !ok {
					all = false
					continue
				}
				lat = append(lat, t-m.Ts)
				if t-m.Ts > worst {
					worst = t - m.Ts
				}
			}
			if !all {
				missingLat++
			}
			_ = worst
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })

	fmt.Printf("环境: macOS 15.6 / Apple M1 / Go 1.27.1 / seed=%d / 虚拟 5 分钟 + 收敛阶段\n", seed)
	fmt.Printf("配置: %d 设备 × %d 会话, 消息 %d 条, 推送丢包 %.0f%%, 同步失败 %.0f%%, 通道重复 %.0f%%\n",
		len(s.devices), len(convIDs), convMsgs, pushLoss*100, syncLoss*100, dupRate*100)
	fmt.Println()

	r := &report{pass: true}

	// I1 不丢：每设备每会话的已应用集合 == 服务端全量
	{
		var detail []string
		ok := true
		for _, d := range s.devices {
			var total, misc int
			for _, cid := range convIDs {
				want := int(s.convs[cid].LastSeq)
				got := len(d.msgs[cid])
				total += want
				if got < want {
					misc += want - got
				}
				for seq := int64(1); seq <= s.convs[cid].LastSeq; seq++ {
					if _, has := d.msgs[cid][seq]; !has {
						misc++
					}
				}
			}
			detail = append(detail, fmt.Sprintf("%s: %d 条应收 / 缺失 %d", d.ID, total, misc))
			if misc != 0 {
				ok = false
			}
		}
		r.add(ok, "I1 不丢 —— "+strings.Join(detail, "; "))
	}

	// I2 不重：展示序列按 msg_id 唯一，且去重计数 > 0 证明通道确实在重复
	{
		var detail []string
		ok := true
		for _, d := range s.devices {
			for _, cid := range convIDs {
				seen := map[string]bool{}
				dup := 0
				for _, m := range d.render(cid) {
					if seen[m.MsgID] {
						dup++
					}
					seen[m.MsgID] = true
				}
				if dup > 0 {
					ok = false
				}
			}
			detail = append(detail, fmt.Sprintf("%s: 去重丢弃 %d 条", d.ID, d.dupDropped))
		}
		if dupDropped == 0 {
			ok = false
			detail = append(detail, "（警告：模拟未触发重复投递，判定无意义）")
		}
		r.add(ok, "I2 不重 —— "+strings.Join(detail, "; "))
	}

	// I3 不乱序：展示序列严格按 conv_seq 递增且逐条等于服务端
	{
		ok := true
		var detail []string
		for _, d := range s.devices {
			bad := 0
			for _, cid := range convIDs {
				list := d.render(cid)
				var prev int64
				for _, m := range list {
					if m.Seq != prev+1 {
						bad++
					}
					prev = m.Seq
					if m.Seq <= int64(len(s.msgs[cid])) {
						if s.msgs[cid][m.Seq-1].MsgID != m.MsgID {
							bad++
						}
					}
				}
			}
			if bad > 0 {
				ok = false
			}
			detail = append(detail, fmt.Sprintf("%s: 乱序到达 %d 条/已纠正, 序错 %d", d.ID, d.oooApplied, bad))
		}
		r.add(ok, "I3 不乱序 —— "+strings.Join(detail, "; "))
	}

	// I4 已读单调：全程无回退，且终态 == 服务端 max 合并后的值
	{
		ok := true
		var detail []string
		for _, d := range s.devices {
			for _, cid := range convIDs {
				if d.readView[cid] != s.convs[cid].ReadSeq {
					ok = false
				}
			}
			if d.readRegressions != 0 {
				ok = false
			}
			detail = append(detail, fmt.Sprintf("%s: read_seq=%v 回退 %d 次", d.ID, []int64{d.readView["c1"], d.readView["c2"], d.readView["c3"]}, d.readRegressions))
		}
		r.add(ok, "I4 已读单调 —— "+strings.Join(detail, "; "))
	}

	// I5 会话列表收敛：三端的 last_seq/last_msg_id/read/pinned/unread 完全一致
	{
		ok := true
		var detail []string
		for _, d := range s.devices {
			for _, cid := range convIDs {
				cs := s.convs[cid]
				lastSeq := d.convCursor[cid]
				if lastSeq != cs.LastSeq {
					ok = false
				}
				if printUnread := lastSeq - d.readView[cid]; printUnread < 0 {
					ok = false
				}
				if d.pinned[cid] != cs.Pinned {
					ok = false
				}
				if lastSeq > 0 {
					lm := d.msgs[cid][lastSeq]
					if lm.MsgID != cs.LastMsgID {
						ok = false
					}
				}
			}
			detail = append(detail, fmt.Sprintf("%s: c1(last=%d,unread=%d,pin=%v)", d.ID,
				d.convCursor["c1"], d.convCursor["c1"]-d.readView["c1"], d.pinned["c1"]))
		}
		// 设置 LWW 收敛
		settingsOK := true
		for _, d := range s.devices {
			for k, st := range s.settings {
				if d.setting(k) != st.Val {
					settingsOK = false
				}
			}
		}
		if !settingsOK {
			ok = false
		}
		detail = append(detail, fmt.Sprintf("设置收敛=%v", settingsOK))
		r.add(ok, "I5 会话列表收敛 —— "+strings.Join(detail, "; "))
	}

	fmt.Println("=== 不变量断言 ===")
	for _, line := range r.lines {
		fmt.Println(line)
	}
	verdict := "全绿 (I1–I5 PASS)"
	if !r.pass {
		verdict = "有红 (FAIL)"
	}
	fmt.Printf("结论: %s\n\n", verdict)

	// 收敛延迟（publish -> 全部设备可见）
	fmt.Printf("=== 收敛延迟（%d 条消息 × %d 设备，单位 ms）===\n", convMsgs, len(s.devices))
	if len(lat) > 0 {
		fmt.Printf("P50=%d  P90=%d  P99=%d  max=%d  样本=%d\n",
			pct(lat, 0.50), pct(lat, 0.90), pct(lat, 0.99), lat[len(lat)-1], len(lat))
	}
	fmt.Printf("终态收敛耗时: %d ms（停负载后 3 设备全部追平）; 明细缺失 %d\n\n", convergeMs, missingLat)

	// 流量与次数
	fmt.Println("=== 同步成本 ===")
	fmt.Printf("同步请求(服务端计)=%d, 同步响应字节=%d (%.2f MB), 推送条数=%d (丢 %d, 重复 %d), 推送字节=%d\n",
		s.syncRequests, s.syncBytes, float64(s.syncBytes)/1024/1024, s.pushDelivered, s.pushLost, s.pushDup, s.pushBytes)
	var att, fails, okn, bytesIn int64
	for _, d := range s.devices {
		att += d.syncAttempts
		fails += d.syncFails
		okn += d.syncOK
		bytesIn += d.bytesIn
		fmt.Printf("%s: 尝试=%d 失败=%d(离线 %d/网络 %d) 成功=%d 拉取消息=%d 去重丢弃=%d 乱序到达=%d 推送叫醒=%d 离线占比=%.1f%% 入站字节=%d\n",
			d.ID, d.syncAttempts, d.syncFails, d.syncFailOffline, d.syncFailNet, d.syncOK, d.msgsFetched, d.dupDropped, d.oooApplied, d.pushTrig, 100*float64(d.offlineMs)/float64(simEndMs), d.bytesIn)
	}
	fmt.Printf("合计: 尝试=%d 失败=%.1f%% 成功=%d 消息拉取=%d 去重丢弃=%d 乱序到达=%d 入站字节=%d\n",
		att, 100*float64(fails)/float64(att), okn, fetches, dupDropped, ooo, bytesIn)
	fmt.Printf("已读上报=%d 其中更旧被 max 合并丢弃=%d\n", s.readReports, s.readStale)
	var pgs int64
	for _, d := range s.devices {
		pgs += d.pages
	}
	fmt.Printf("变更流水=%d 条, 同步页数=%d, 服务端推送/在线设备组合=%d\n", s.seq, pgs, s.pushSent)
	fmt.Printf("推送到达/触发: %d / %d\n", pushRecv, pushTrig)

	// 退出码给外部脚本用
	if !r.pass {
		os.Exit(1)
	}
}
