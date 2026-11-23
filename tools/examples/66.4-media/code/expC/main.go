// expC: 在 Pion 网络层注入丢包/时延，测"弱网 → 有效到达 / NACK / 延迟"。
//
// 做法：SettingEngine.SetNet() 换成自定义 transport.Net，包装 ListenPacket 返回的
// net.PacketConn：WriteTo 按概率丢包、按固定时延异步发送。两个 PeerConnection 各一份，
// 即每个方向都有 loss/delay。
//
// 媒体：A 用 VP8 TrackLocalStaticSample 每 20ms 写 800B 样本；B 用 ReadRTP 统计到达。
// 开关：-nack=false 时不注册 NACK 拦截器，对比"有重传/没重传"的有效到达率。
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/transport/v5"
	stdnet "github.com/pion/transport/v5/stdnet"
	"github.com/pion/webrtc/v4"
)

const (
	udpPortMin uint16 = 13500
	udpPortMax uint16 = 13509
)

// ---------- 网络注入 ----------

type netConfig struct {
	loss  float64
	delay time.Duration
}

type lossyNet struct {
	transport.Net // 其余方法（ResolveUDPAddr/Interfaces/Dial...）直接复用标准实现
	cfg           netConfig
	seed          int64
	sent          uint64
	dropped       uint64
	delayed       uint64
}

func (n *lossyNet) ListenPacket(network, address string) (net.PacketConn, error) {
	pc, err := n.Net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}
	return &lossyConn{
		PacketConn: pc,
		parent:     n,
		rnd:        rand.New(rand.NewSource(n.seed + time.Now().UnixNano())),
	}, nil
}

func (n *lossyNet) ListenUDP(network string, laddr *net.UDPAddr) (transport.UDPConn, error) {
	c, err := n.Net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	return &lossyUDPConn{
		UDPConn: c,
		parent:  n,
		rnd:     rand.New(rand.NewSource(n.seed + time.Now().UnixNano())),
	}, nil
}

func randFloat(p **rand.Rand) float64 { return (**p).Float64() }

type lossyConn struct {
	net.PacketConn
	parent *lossyNet
	mu     sync.Mutex
	rnd    *rand.Rand
}

// write 是所有写路径的唯一入口：先按概率丢，再决定是否延迟发。
func (n *lossyNet) write(b []byte, write func([]byte) (int, error), mu *sync.Mutex, rnd *rand.Rand) (int, error) {
	atomic.AddUint64(&n.sent, 1)
	mu.Lock()
	r := rnd.Float64()
	mu.Unlock()
	if r < n.cfg.loss { // 丢包：对上层返回成功（UDP 无语义上的写失败）
		atomic.AddUint64(&n.dropped, 1)
		return len(b), nil
	}
	if n.cfg.delay > 0 { // 时延：异步发送，避免阻塞 ICE/DTLS 的写调用栈
		atomic.AddUint64(&n.delayed, 1)
		buf := append([]byte(nil), b...)
		go func() {
			time.Sleep(n.cfg.delay)
			_, _ = write(buf)
		}()
		return len(b), nil
	}
	return write(b)
}

func (c *lossyConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.parent.write(b, func(x []byte) (int, error) { return c.PacketConn.WriteTo(x, addr) }, &c.mu, c.rnd)
}

// lossyUDPConn：ICE 的 host 候选走 transport.Net.ListenUDP（见 ice listenUDPInPortRange），
// 所以必须同时包住 UDPConn 的写路径，只包 ListenPacket 会漏掉全部 host 流量。
type lossyUDPConn struct {
	transport.UDPConn
	parent *lossyNet
	mu     sync.Mutex
	rnd    *rand.Rand
}

func (c *lossyUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.parent.write(b, func(x []byte) (int, error) { return c.UDPConn.WriteTo(x, addr) }, &c.mu, c.rnd)
}

func (c *lossyUDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	return c.parent.write(b, func(x []byte) (int, error) { return c.UDPConn.WriteToUDP(x, addr) }, &c.mu, c.rnd)
}

// ---------- 信令辅助（同 expA，trickle ICE 需缓存早到的 candidate） ----------

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

func newSettingEngine(loss float64, delay time.Duration) (webrtc.SettingEngine, *lossyNet) {
	inner, err := stdnet.NewNet()
	if err != nil {
		panic(err)
	}
	ln := &lossyNet{Net: inner, cfg: netConfig{loss: loss, delay: delay}, seed: time.Now().UnixNano()}
	se := webrtc.SettingEngine{}
	_ = se.SetEphemeralUDPPortRange(udpPortMin, udpPortMax)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetIncludeLoopbackCandidate(true)
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetInterfaceFilter(func(name string) bool { return strings.HasPrefix(name, "lo") })
	se.SetFireOnTrackBeforeFirstRTP(true) // 否则 OnTrack 要等首个 RTP 包，发送端与接收端互相等
	se.SetNet(ln)
	return se, ln
}

// apiNoNack 手工装拦截器：去掉 NACK 生成器/响应器，其余（RTCP 报告、stats、TWCC）保留。
func apiNoNack(se webrtc.SettingEngine) (*webrtc.API, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	reg := &interceptor.Registry{}
	if err := webrtc.ConfigureRTCPReports(reg); err != nil {
		return nil, err
	}
	if err := webrtc.ConfigureSimulcastExtensionHeaders(m); err != nil {
		return nil, err
	}
	if err := webrtc.ConfigureStatsInterceptor(reg); err != nil {
		return nil, err
	}
	if err := webrtc.ConfigureTWCCSender(m, reg); err != nil {
		return nil, err
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(reg), webrtc.WithSettingEngine(se)), nil
}

// ---------- 结果 ----------

type oneWay struct {
	Count int     `json:"count"`
	Min   float64 `json:"min_ms"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P99   float64 `json:"p99_ms"`
	Max   float64 `json:"max_ms"`
}

func stat(ds []time.Duration) oneWay {
	if len(ds) == 0 {
		return oneWay{}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
	pct := func(p float64) float64 {
		i := int(float64(len(ds))*p/100.0) - 1
		if i < 0 {
			i = 0
		}
		if i >= len(ds) {
			i = len(ds) - 1
		}
		return ms(ds[i])
	}
	return oneWay{len(ds), ms(ds[0]), pct(50), pct(90), pct(99), ms(ds[len(ds)-1])}
}

type result struct {
	Loss          float64            `json:"injected_loss"`
	DelayMs       float64            `json:"injected_delay_ms"`
	Nack          bool               `json:"nack_enabled"`
	Samples       int                `json:"samples_sent"`
	RTPUnique     int                `json:"rtp_unique_received"`
	RTPMissing    int                `json:"rtp_missing"`
	RTPDup        int                `json:"rtp_duplicates"`
	RTPOutOfOrder int                `json:"rtp_out_of_order"`
	RTPDelay      oneWay             `json:"rtp_one_way_delay"`
	DC            oneWay             `json:"datachannel_rtt"`
	DCFail        int                `json:"datachannel_fail"`
	ConnMs        float64            `json:"connect_ms"`
	Inbound       map[string]any     `json:"inbound_rtp_stats"`
	NetStats      map[string]float64 `json:"net_layer"`
}

func main() {
	loss := flag.Float64("loss", 0.05, "写方向丢包率，例如 0.05 / 0.2")
	delay := flag.Duration("delay", 0, "写方向固定时延，例如 50ms")
	nack := flag.Bool("nack", true, "是否注册 NACK 拦截器（重传）")
	n := flag.Int("n", 400, "发送的媒体样本数（每个 20ms）")
	dcPings := flag.Int("dcpings", 100, "DataChannel RTT 次数")
	out := flag.String("out", "", "json 输出路径")
	flag.Parse()

	seA, netA := newSettingEngine(*loss, *delay)
	seB, netB := newSettingEngine(*loss, *delay)

	var apiA, apiB *webrtc.API
	var err error
	if *nack {
		apiA = webrtc.NewAPI(webrtc.WithSettingEngine(seA))
		apiB = webrtc.NewAPI(webrtc.WithSettingEngine(seB))
	} else {
		apiA, err = apiNoNack(seA)
		if err != nil {
			panic(err)
		}
		apiB, err = apiNoNack(seB)
		if err != nil {
			panic(err)
		}
	}

	pcA, err := apiA.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	pcB, err := apiB.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		panic(err)
	}
	defer pcA.Close()
	defer pcB.Close()

	// 媒体轨：直接写 RTP 包（不经过打包器），每个样本 = 1 个 RTP 包。
	// Payload 前 4 字节放样本序号，接收端据此算单向时延（同一进程共用时钟）。
	const vp8PT = 96
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}, "video", "luban")
	if err != nil {
		panic(err)
	}
	sender, err := pcA.AddTrack(track)
	if err != nil {
		panic(err)
	}
	// 关键：Pion 不会自己消费 sender 的 RTCP，必须有人 Read，
	// 否则 NACK 响应器收不到 NACK、REMB/TWCC 反馈也全部空转。
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()

	type arrival struct {
		seq uint16
		idx uint32 // payload 里的样本序号
		at  time.Time
	}
	var arrMu sync.Mutex
	var arrivals []arrival
	trackReady := make(chan struct{})
	var trackOnce sync.Once

	pcB.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		trackOnce.Do(func() { close(trackReady) })
		for {
			pkt, _, err := remote.ReadRTP()
			if err != nil {
				return
			}
			if len(pkt.Payload) < 4 {
				continue
			}
			arrMu.Lock()
			arrivals = append(arrivals, arrival{pkt.SequenceNumber, binary.BigEndian.Uint32(pkt.Payload[:4]), time.Now()})
			arrMu.Unlock()
		}
	})

	// signaling
	aCandCh := make(chan webrtc.ICECandidateInit, 64)
	bCandCh := make(chan webrtc.ICECandidateInit, 64)
	aRemote, bRemote := &remoteState{}, &remoteState{}
	pcA.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			close(aCandCh)
			return
		}
		aCandCh <- c.ToJSON()
	})
	pcB.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			close(bCandCh)
			return
		}
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

	connected := make(chan struct{})
	var connOnce sync.Once
	t0 := time.Now()
	pcA.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateConnected {
			connOnce.Do(func() { close(connected) })
		}
	})

	dc, err := pcA.CreateDataChannel("ping", nil)
	if err != nil {
		panic(err)
	}
	ackCh := make(chan []byte, 4*(*dcPings))
	dcOpen := make(chan struct{})
	var dcOnce sync.Once
	dc.OnOpen(func() { dcOnce.Do(func() { close(dcOpen) }) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) { ackCh <- append([]byte(nil), m.Data...) })
	pcB.OnDataChannel(func(d *webrtc.DataChannel) {
		d.OnMessage(func(m webrtc.DataChannelMessage) { _ = d.Send(m.Data) })
	})

	offer, err := pcA.CreateOffer(nil)
	if err != nil {
		panic(err)
	}
	if err = pcA.SetLocalDescription(offer); err != nil {
		panic(err)
	}
	ob, _ := json.Marshal(pcA.LocalDescription())
	var ow webrtc.SessionDescription
	_ = json.Unmarshal(ob, &ow)
	if err = pcB.SetRemoteDescription(ow); err != nil {
		panic(err)
	}
	bRemote.flush(pcB)
	answer, err := pcB.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	if err = pcB.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	ab, _ := json.Marshal(pcB.LocalDescription())
	var aw webrtc.SessionDescription
	_ = json.Unmarshal(ab, &aw)
	if err = pcA.SetRemoteDescription(aw); err != nil {
		panic(err)
	}
	aRemote.flush(pcA)

	select {
	case <-connected:
	case <-time.After(30 * time.Second):
		fmt.Fprintln(os.Stderr, "connect timeout")
		os.Exit(3)
	}
	connMs := float64(time.Since(t0).Microseconds()) / 1000.0
	fmt.Printf("connected in %.1fms (loss=%.0f%% delay=%s nack=%v)\n", connMs, *loss*100, *delay, *nack)

	// DataChannel RTT：可靠通道在丢包下是"变慢"而不是"丢"
	select {
	case <-dcOpen:
	case <-time.After(10 * time.Second):
		fmt.Fprintln(os.Stderr, "datachannel open timeout")
		os.Exit(6)
	}
	dcRTT := make([]time.Duration, 0, *dcPings)
	dcFail := 0
	for i := 0; i < *dcPings; i++ {
		start := time.Now()
		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, uint32(i))
		if err := dc.Send(payload); err != nil {
			dcFail++
			continue
		}
		select {
		case <-ackCh:
			dcRTT = append(dcRTT, time.Since(start))
		case <-time.After(3 * time.Second):
			dcFail++
		}
	}

	// 等 B 的 track 就绪，再开始发样本
	select {
	case <-trackReady:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "track not ready")
		os.Exit(4)
	}
	fmt.Printf("dc rtt p50=%.2fms p99=%.2fms fail=%d\n", stat(dcRTT).P50, stat(dcRTT).P99, dcFail)

	const interval = 20 * time.Millisecond
	sendAt := make([]time.Time, *n)
	baseSeq := uint16(1000)
	baseTS := uint32(90000)
	for i := 0; i < *n; i++ {
		payload := make([]byte, 800)
		binary.BigEndian.PutUint32(payload, uint32(i)) // 样本序号，接收端用来对齐发送时刻
		for j := 4; j < len(payload); j++ {
			payload[j] = byte(i)
		}
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    vp8PT,
				SequenceNumber: baseSeq + uint16(i),
				Timestamp:      baseTS + uint32(i)*1800, // 20ms @ 90kHz
				SSRC:           0x1234,
			},
			Payload: payload,
		}
		sendAt[i] = time.Now()
		if err := track.WriteRTP(pkt); err != nil {
			fmt.Fprintln(os.Stderr, "WriteRTP:", err)
			break
		}
		time.Sleep(interval)
	}
	// 多等一会，让 NACK 重传、RTCP 报告都落地
	time.Sleep(3 * time.Second)

	arrMu.Lock()
	arrs := append([]arrival(nil), arrivals...)
	arrMu.Unlock()

	if len(arrs) == 0 {
		fmt.Fprintln(os.Stderr, "no RTP packet received")
		os.Exit(5)
	}
	// 分析：一个样本一个 RTP 包，payload[0:4] 是样本序号，发送时刻在同进程里可对齐
	uniq := map[uint16]bool{}
	seen := map[uint16]int{}
	delays := make([]time.Duration, 0, *n)
	outOfOrder := 0
	lastSeq := arrs[0].seq
	for _, a := range arrs {
		seen[a.seq]++
		if less(a.seq, lastSeq) { // 序号回退 = 乱序到达
			outOfOrder++
		} else {
			lastSeq = a.seq
		}
		if uniq[a.seq] {
			continue
		}
		uniq[a.seq] = true
		if int(a.idx) < len(sendAt) {
			delays = append(delays, a.at.Sub(sendAt[a.idx]))
		}
	}
	dup := 0
	for _, c := range seen {
		if c > 1 {
			dup += c - 1
		}
	}
	missing := *n - len(uniq)

	// 结束时的入向 RTP 统计
	res := result{
		Loss: *loss, DelayMs: float64(*delay / time.Millisecond), Nack: *nack,
		Samples: *n, RTPUnique: len(uniq), RTPMissing: missing, RTPDup: dup, RTPOutOfOrder: outOfOrder,
		RTPDelay: stat(delays), DC: stat(dcRTT), DCFail: dcFail, ConnMs: connMs,
		Inbound: map[string]any{}, NetStats: map[string]float64{},
	}
	for _, s := range pcB.GetStats() {
		if in, ok := s.(webrtc.InboundRTPStreamStats); ok && in.Kind == "video" {
			res.Inbound["packetsReceived"] = in.PacketsReceived
			res.Inbound["packetsLost"] = in.PacketsLost
			res.Inbound["nackCount"] = in.NACKCount
			res.Inbound["jitter_ms"] = in.Jitter * 1000
			res.Inbound["bytesReceived"] = in.BytesReceived
			break
		}
	}
	res.NetStats["sent"] = float64(atomic.LoadUint64(&netA.sent) + atomic.LoadUint64(&netB.sent))
	res.NetStats["dropped"] = float64(atomic.LoadUint64(&netA.dropped) + atomic.LoadUint64(&netB.dropped))
	res.NetStats["delayed"] = float64(atomic.LoadUint64(&netA.delayed) + atomic.LoadUint64(&netB.delayed))
	res.NetStats["deliver_ratio"] = float64(*n-len(uniq)) / float64(*n)

	fmt.Printf("samples=%d unique=%d missing=%d dup=%d ooo=%d\n", *n, res.RTPUnique, res.RTPMissing, res.RTPDup, res.RTPOutOfOrder)
	fmt.Printf("one-way delay p50=%.2fms p99=%.2fms max=%.2fms\n", res.RTPDelay.P50, res.RTPDelay.P99, res.RTPDelay.Max)
	fmt.Printf("inbound stats: %v\n", res.Inbound)
	fmt.Printf("net layer: sent=%v dropped=%v delayed=%v\n", res.NetStats["sent"], res.NetStats["dropped"], res.NetStats["delayed"])

	if *out != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		_ = os.WriteFile(*out, b, 0o644)
		fmt.Println("json written:", *out)
	}
}

// less 用于检测序号回退（演示用，不做完整 16 位回绕处理）
func less(a, b uint16) bool { return int(a) < int(b) }
