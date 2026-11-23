// expA: 本机回环 WebRTC 建连实验。
// 两个 PeerConnection 在同一个进程里用"内存信令"交换 SDP + trickle ICE，
// 记录：CreateOffer / ICE gathering / connected 各阶段耗时、选中的 candidate pair、
// DataChannel 往返 RTT（顺序 1000 次 + 突发 1000 次）。
//
// 模式：
//
//	-mode=host       只走 host candidate（UDP 回环）
//	-mode=relay-udp  ICETransportPolicy=relay，TURN over UDP 127.0.0.1:13478
//	-mode=relay-tcp  ICETransportPolicy=relay，TURN over TCP 127.0.0.1:13478
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

const (
	udpPortMin uint16 = 13500
	udpPortMax uint16 = 13509
	turnAddr          = "127.0.0.1:13478"
	turnUser          = "luban"
	turnPass          = "lubanpass"
)

// marks 记录相对 t0 的关键时间点（毫秒），带锁因为回调来自不同 goroutine。
type marks struct {
	mu  sync.Mutex
	m   map[string]float64
	t0  time.Time
	tl  []string
	tlM sync.Mutex
}

func newMarks(t0 time.Time) *marks { return &marks{m: map[string]float64{}, t0: t0} }

func (mk *marks) mark(name string, t time.Time) {
	mk.mu.Lock()
	if _, ok := mk.m[name]; !ok { // 只记第一次，避免状态抖动覆盖
		mk.m[name] = float64(t.Sub(mk.t0).Microseconds()) / 1000.0
	}
	mk.mu.Unlock()
}

func (mk *marks) log(format string, args ...any) {
	mk.tlM.Lock()
	mk.tl = append(mk.tl, fmt.Sprintf("[+%8.2fms] ", float64(time.Since(mk.t0).Microseconds())/1000.0)+fmt.Sprintf(format, args...))
	mk.tlM.Unlock()
}

// remoteState 解决 trickle ICE 的时序问题：远端 SDP 还没 Set 时先缓存 candidate。
type remoteState struct {
	mu      sync.Mutex
	ready   bool
	pending []webrtc.ICECandidateInit
}

func (rs *remoteState) apply(pc *webrtc.PeerConnection, ci webrtc.ICECandidateInit) {
	rs.mu.Lock()
	if !rs.ready {
		rs.pending = append(rs.pending, ci)
		rs.mu.Unlock()
		return
	}
	rs.mu.Unlock()
	_ = pc.AddICECandidate(ci)
}

func (rs *remoteState) flush(pc *webrtc.PeerConnection) {
	rs.mu.Lock()
	rs.ready = true
	p := rs.pending
	rs.pending = nil
	rs.mu.Unlock()
	for _, ci := range p {
		_ = pc.AddICECandidate(ci)
	}
}

type candStat struct {
	mu    sync.Mutex
	types map[string]int
}

func newCandStat() *candStat { return &candStat{types: map[string]int{}} }

func (c *candStat) add(t string) {
	c.mu.Lock()
	c.types[t]++
	c.mu.Unlock()
}

func (c *candStat) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int{}
	for k, v := range c.types {
		out[k] = v
	}
	return out
}

type rttStat struct {
	Count int     `json:"count"`
	Fail  int     `json:"fail"`
	Min   float64 `json:"min_ms"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P99   float64 `json:"p99_ms"`
	Max   float64 `json:"max_ms"`
	Avg   float64 `json:"avg_ms"`
}

func summarize(ds []time.Duration, fail int) rttStat {
	if len(ds) == 0 {
		return rttStat{Fail: fail}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
	pct := func(p float64) float64 {
		idx := int(math.Ceil(p/100.0*float64(len(ds)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(ds) {
			idx = len(ds) - 1
		}
		return ms(ds[idx])
	}
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	return rttStat{
		Count: len(ds), Fail: fail,
		Min: ms(ds[0]), P50: pct(50), P90: pct(90), P99: pct(99), Max: ms(ds[len(ds)-1]),
		Avg: ms(sum / time.Duration(len(ds))),
	}
}

type result struct {
	Mode       string             `json:"mode"`
	Marks      map[string]float64 `json:"marks_ms"`
	CandA      map[string]int     `json:"candidates_a"`
	CandB      map[string]int     `json:"candidates_b"`
	LocalCand  string             `json:"selected_local"`
	RemoteCand string             `json:"selected_remote"`
	PairProto  string             `json:"pair_protocol"`
	Seq        rttStat            `json:"rtt_sequential"`
	Burst      rttStat            `json:"rtt_burst"`
	Timeline   []string           `json:"timeline"`
}

func newSettingEngine(iface string, relayWait time.Duration) webrtc.SettingEngine {
	se := webrtc.SettingEngine{}
	// 固定 UDP 端口段，便于与并行任务隔离、也便于抓包
	_ = se.SetEphemeralUDPPortRange(udpPortMin, udpPortMax)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled) // 回环环境无需 mDNS
	se.SetIncludeLoopbackCandidate(true)                    // 否则 127.0.0.1 候选被过滤，回环实验建不起来
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	if iface == "loopback" { // 只留 lo0，排除 192.168 网卡，避免"同一台机器走局域网"的干扰
		se.SetInterfaceFilter(func(name string) bool { return strings.HasPrefix(name, "lo") })
	}
	if relayWait >= 0 { // Pion 默认提名 relay 候选前要等 2s，TURN 场景下这是建连 P50 的主要来源
		se.SetRelayAcceptanceMinWait(relayWait)
	}
	return se
}

func configFor(mode string) (webrtc.Configuration, error) {
	cfg := webrtc.Configuration{}
	switch mode {
	case "host":
		return cfg, nil
	case "relay-udp", "relay-tcp":
		tr := "udp"
		if mode == "relay-tcp" {
			tr = "tcp"
		}
		cfg.ICEServers = []webrtc.ICEServer{{
			URLs:           []string{fmt.Sprintf("turn:%s?transport=%s", turnAddr, tr)},
			Username:       turnUser,
			Credential:     turnPass,
			CredentialType: webrtc.ICECredentialTypePassword,
		}}
		cfg.ICETransportPolicy = webrtc.ICETransportPolicyRelay // 强制 relay，禁用 host/srflx
		return cfg, nil
	default:
		return cfg, fmt.Errorf("unknown mode %q", mode)
	}
}

func main() {
	mode := flag.String("mode", "host", "host | relay-udp | relay-tcp")
	pings := flag.Int("pings", 1000, "number of DataChannel pings")
	out := flag.String("out", "", "optional json output path")
	iface := flag.String("iface", "all", "all | loopback：限制 ICE 网卡，loopback 只留 lo0")
	relayWait := flag.Duration("relaywait", -1, "覆盖 ICE relay 候选提名等待；<0 用 Pion 默认（2000ms）")
	flag.Parse()

	cfg, err := configFor(*mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	t0 := time.Now()
	mk := newMarks(t0)
	res := result{Mode: *mode}

	seA := newSettingEngine(*iface, *relayWait)
	seB := newSettingEngine(*iface, *relayWait)
	pcA, err := webrtc.NewAPI(webrtc.WithSettingEngine(seA)).NewPeerConnection(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pcA:", err)
		os.Exit(1)
	}
	pcB, err := webrtc.NewAPI(webrtc.WithSettingEngine(seB)).NewPeerConnection(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pcB:", err)
		os.Exit(1)
	}
	defer pcA.Close()
	defer pcB.Close()

	candA, candB := newCandStat(), newCandStat()
	aCandCh := make(chan webrtc.ICECandidateInit, 64)
	bCandCh := make(chan webrtc.ICECandidateInit, 64)
	aRemote, bRemote := &remoteState{}, &remoteState{}

	pcA.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			close(aCandCh)
			mk.log("A ICE gathering complete")
			return
		}
		candA.add(c.Typ.String())
		mk.log("A candidate %s %s:%d", c.Typ, c.Address, c.Port)
		aCandCh <- c.ToJSON()
	})
	pcB.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			close(bCandCh)
			mk.log("B ICE gathering complete")
			return
		}
		candB.add(c.Typ.String())
		mk.log("B candidate %s %s:%d", c.Typ, c.Address, c.Port)
		bCandCh <- c.ToJSON()
	})
	go func() {
		for ci := range aCandCh {
			bRemote.apply(pcB, ci)
		}
	}()
	go func() {
		for ci := range bCandCh {
			aRemote.apply(pcA, ci)
		}
	}()

	pcA.OnICEGatheringStateChange(func(s webrtc.ICEGatheringState) { mk.log("A ice-gathering -> %s", s) })
	pcB.OnICEGatheringStateChange(func(s webrtc.ICEGatheringState) { mk.log("B ice-gathering -> %s", s) })
	pcA.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		mk.log("A ice-conn -> %s", s)
		if s == webrtc.ICEConnectionStateConnected {
			mk.mark("ice_connected_a", time.Now())
		}
	})
	pcA.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		mk.log("A peer-conn -> %s", s)
		if s == webrtc.PeerConnectionStateConnected {
			mk.mark("connected_a", time.Now())
		}
	})
	pcB.OnConnectionStateChange(func(s webrtc.PeerConnectionState) { mk.log("B peer-conn -> %s", s) })

	dc, err := pcA.CreateDataChannel("ping", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datachannel:", err)
		os.Exit(1)
	}
	ackCh := make(chan []byte, *pings)
	openCh := make(chan struct{})
	var openOnce sync.Once
	dc.OnOpen(func() {
		mk.log("A datachannel open")
		openOnce.Do(func() { close(openCh) })
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		ackCh <- append([]byte(nil), m.Data...)
	})
	pcB.OnDataChannel(func(d *webrtc.DataChannel) {
		mk.log("B got datachannel %q", d.Label())
		d.OnMessage(func(m webrtc.DataChannelMessage) {
			_ = d.Send(m.Data) // 原样回显：只测传输路径，不做业务逻辑
		})
	})

	// ---- 信令：offer -> answer，SDP 走一次 JSON 序列化模拟"过线" ----
	offer, err := pcA.CreateOffer(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CreateOffer:", err)
		os.Exit(1)
	}
	mk.mark("create_offer", time.Now())
	if err = pcA.SetLocalDescription(offer); err != nil {
		fmt.Fprintln(os.Stderr, "A SetLocalDescription:", err)
		os.Exit(1)
	}
	mk.mark("set_local_offer", time.Now())

	ob, _ := json.Marshal(pcA.LocalDescription())
	var offerWire webrtc.SessionDescription
	_ = json.Unmarshal(ob, &offerWire)
	if err = pcB.SetRemoteDescription(offerWire); err != nil {
		fmt.Fprintln(os.Stderr, "B SetRemoteDescription(offer):", err)
		os.Exit(1)
	}
	bRemote.flush(pcB)
	mk.mark("b_set_remote_offer", time.Now())

	answer, err := pcB.CreateAnswer(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CreateAnswer:", err)
		os.Exit(1)
	}
	mk.mark("create_answer", time.Now())
	if err = pcB.SetLocalDescription(answer); err != nil {
		fmt.Fprintln(os.Stderr, "B SetLocalDescription:", err)
		os.Exit(1)
	}
	mk.mark("b_set_local_answer", time.Now())

	ab, _ := json.Marshal(pcB.LocalDescription())
	var answerWire webrtc.SessionDescription
	_ = json.Unmarshal(ab, &answerWire)
	if err = pcA.SetRemoteDescription(answerWire); err != nil {
		fmt.Fprintln(os.Stderr, "A SetRemoteDescription(answer):", err)
		os.Exit(1)
	}
	aRemote.flush(pcA)
	mk.mark("a_set_remote_answer", time.Now())

	// ---- 等 connected + datachannel open ----
	deadline := time.After(20 * time.Second)
	waitConnected := func() bool {
		for {
			select {
			case <-openCh:
				return true
			case <-deadline:
				return false
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if !waitConnected() {
		fmt.Fprintln(os.Stderr, "timeout waiting connected/datachannel; timeline:")
		for _, l := range mk.tl {
			fmt.Fprintln(os.Stderr, l)
		}
		os.Exit(3)
	}
	mk.mark("datachannel_open", time.Now())
	time.Sleep(100 * time.Millisecond) // 让 ICE 稳定性检查/选对完成

	// ---- 选中的 candidate pair ----
	pair, err := pcA.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil {
		fmt.Fprintln(os.Stderr, "GetSelectedCandidatePair:", err)
	} else {
		res.LocalCand = fmt.Sprintf("%s/%s %s:%d", pair.Local.Typ, pair.Local.Protocol, pair.Local.Address, pair.Local.Port)
		res.RemoteCand = fmt.Sprintf("%s/%s %s:%d", pair.Remote.Typ, pair.Remote.Protocol, pair.Remote.Address, pair.Remote.Port)
		mk.log("selected pair: %s -> %s", res.LocalCand, res.RemoteCand)
	}

	// ---- RTT：顺序 1000 次 ----
	fail := 0
	seqRTT := make([]time.Duration, 0, *pings)
	for i := 0; i < *pings; i++ {
		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, uint32(i))
		start := time.Now()
		if err := dc.Send(payload); err != nil {
			fail++
			continue
		}
		select {
		case <-ackCh:
			seqRTT = append(seqRTT, time.Since(start))
		case <-time.After(2 * time.Second):
			fail++
		}
	}
	res.Seq = summarize(seqRTT, fail)

	// ---- RTT：突发 1000 次（一次性灌入，测排队效应）----
	burstRTT := make([]time.Duration, 0, *pings)
	sentAt := make(map[uint32]time.Time, *pings)
	fail = 0
	for i := 0; i < *pings; i++ {
		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, uint32(i))
		sentAt[uint32(i)] = time.Now()
		if err := dc.Send(payload); err != nil {
			fail++
			delete(sentAt, uint32(i))
		}
	}
	recv := 0
	for recv < *pings-fail {
		select {
		case b := <-ackCh:
			if len(b) == 4 {
				if t, ok := sentAt[binary.BigEndian.Uint32(b)]; ok {
					burstRTT = append(burstRTT, time.Since(t))
					recv++
				}
			}
		case <-time.After(5 * time.Second):
			recv = *pings // 超时退出
		}
	}
	res.Burst = summarize(burstRTT, *pings-len(burstRTT))

	mk.mu.Lock()
	res.Marks = make(map[string]float64, len(mk.m))
	for k, v := range mk.m {
		res.Marks[k] = v
	}
	mk.mu.Unlock()
	res.CandA = candA.snapshot()
	res.CandB = candB.snapshot()
	mk.tlM.Lock()
	res.Timeline = append([]string(nil), mk.tl...)
	mk.tlM.Unlock()

	// ---- 人读摘要 ----
	fmt.Printf("=== expA mode=%s pings=%d ===\n", res.Mode, *pings)
	for _, k := range []string{"create_offer", "set_local_offer", "b_set_remote_offer", "b_set_local_answer", "a_set_remote_answer", "ice_connected_a", "connected_a", "datachannel_open"} {
		if v, ok := res.Marks[k]; ok {
			fmt.Printf("%-22s %8.2f ms\n", k, v)
		}
	}
	fmt.Printf("candidates A=%v B=%v\n", res.CandA, res.CandB)
	fmt.Printf("selected  local=%s remote=%s\n", res.LocalCand, res.RemoteCand)
	fmt.Printf("RTT seq   count=%d fail=%d p50=%.3fms p99=%.3fms max=%.3fms avg=%.3fms\n",
		res.Seq.Count, res.Seq.Fail, res.Seq.P50, res.Seq.P99, res.Seq.Max, res.Seq.Avg)
	fmt.Printf("RTT burst count=%d fail=%d p50=%.3fms p99=%.3fms max=%.3fms avg=%.3fms\n",
		res.Burst.Count, res.Burst.Fail, res.Burst.P50, res.Burst.P99, res.Burst.Max, res.Burst.Avg)
	fmt.Println("--- timeline ---")
	for _, l := range res.Timeline {
		fmt.Println(l)
	}
	if *out != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		_ = os.WriteFile(*out, b, 0o644)
		fmt.Println("json written:", *out)
	}
}
