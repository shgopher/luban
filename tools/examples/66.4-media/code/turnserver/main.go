// turnserver: 本机 TURN 服务（pion/turn v4），同时监听 UDP 与 TCP 13478。
// 用法：go run ./turnserver -port 13478 -relay-min 13510 -relay-max 13520
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

func main() {
	port := flag.Int("port", 13478, "TURN 监听端口（UDP 与 TCP 同端口）")
	relayMin := flag.Int("relay-min", 13510, "relay 端口范围下限")
	relayMax := flag.Int("relay-max", 13520, "relay 端口范围上限")
	realm := flag.String("realm", "luban.local", "TURN realm")
	user := flag.String("user", "luban", "静态用户名")
	pass := flag.String("pass", "lubanpass", "静态密码")
	udpOn := flag.Bool("udp", true, "是否监听 UDP")
	tcpOn := flag.Bool("tcp", true, "是否监听 TCP")
	flag.Parse()

	var allocs int64

	// 短时凭证的标准做法是 HMAC 派生的临时 username/password，这里用静态用户演示。
	authHandler := func(username, r string, srcAddr net.Addr) ([]byte, bool) {
		ok := username == *user && r == *realm
		fmt.Printf("[auth] user=%q realm=%q src=%s ok=%v\n", username, r, srcAddr, ok)
		if !ok {
			return nil, false
		}
		return turn.GenerateAuthKey(username, r, *pass), true
	}

	newGenerator := func() *turn.RelayAddressGeneratorPortRange {
		return &turn.RelayAddressGeneratorPortRange{
			RelayAddress: net.ParseIP("127.0.0.1"),
			MinPort:      uint16(*relayMin),
			MaxPort:      uint16(*relayMax),
			Address:      "127.0.0.1",
		}
	}

	var packetConns []turn.PacketConnConfig
	var listeners []turn.ListenerConfig
	if *udpOn {
		udpConn, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", *port))
		if err != nil {
			fmt.Fprintln(os.Stderr, "listen udp:", err)
			os.Exit(1)
		}
		packetConns = append(packetConns, turn.PacketConnConfig{
			PacketConn:            udpConn,
			RelayAddressGenerator: newGenerator(),
		})
	}
	if *tcpOn {
		tcpListener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
		if err != nil {
			fmt.Fprintln(os.Stderr, "listen tcp:", err)
			os.Exit(1)
		}
		listeners = append(listeners, turn.ListenerConfig{
			Listener:              tcpListener,
			RelayAddressGenerator: newGenerator(),
		})
	}

	eventHandler := turn.EventHandler{
		OnAllocationCreated: func(srcAddr, dstAddr net.Addr, protocol, username, r string, relayAddr net.Addr, requestedPort int) {
			n := atomic.AddInt64(&allocs, 1)
			fmt.Printf("[allocation created] proto=%s user=%s client=%s relay=%s requestedPort=%d active=%d\n",
				protocol, username, srcAddr, relayAddr, requestedPort, n)
		},
		OnAllocationDeleted: func(srcAddr, dstAddr net.Addr, protocol, username, r string) {
			n := atomic.AddInt64(&allocs, -1)
			fmt.Printf("[allocation deleted] proto=%s user=%s client=%s active=%d\n", protocol, username, srcAddr, n)
		},
	}

	lf := logging.NewDefaultLoggerFactory()
	lf.DefaultLogLevel = logging.LogLevelError // 只保留错误日志，输出干净

	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             *realm,
		AuthHandler:       authHandler,
		PacketConnConfigs: packetConns,
		ListenerConfigs:   listeners,
		EventHandler:      eventHandler,
		LoggerFactory:     lf,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "turn server:", err)
		os.Exit(1)
	}
	defer func() { _ = srv.Close() }()

	fmt.Printf("TURN listening udp/tcp 127.0.0.1:%d realm=%s user=%s relay-ports=%d-%d\n",
		*port, *realm, *user, *relayMin, *relayMax)

	// 每秒打印一次活跃 allocation 数，便于和客户端日志对齐
	go func() {
		for range time.Tick(2 * time.Second) {
			fmt.Printf("[status] active allocations=%d\n", srv.AllocationCount())
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("shutting down")
}
