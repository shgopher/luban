// 长连接网关可执行程序。
//
// 启动两台（18081 / 18082）共用一台 Redis（16381）即可复现跨网关投递：
//
//	go run ./cmd/gateway -id gw1 -addr 127.0.0.1:18081 -redis 127.0.0.1:16381
//	go run ./cmd/gateway -id gw2 -addr 127.0.0.1:18082 -redis 127.0.0.1:16381
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"im-gw/internal/gw"
)

func main() {
	cfg := gw.Config{}
	flag.StringVar(&cfg.ID, "id", "gw1", "网关实例 ID")
	flag.StringVar(&cfg.Addr, "addr", "127.0.0.1:18081", "监听地址")
	flag.StringVar(&cfg.Advertise, "advertise", "", "注册到 Redis 的地址，默认同 addr")
	flag.StringVar(&cfg.RedisAddr, "redis", "127.0.0.1:16381", "Redis 地址")
	flag.IntVar(&cfg.ReadBuf, "readbuf", 4096, "websocket 读缓冲字节数")
	flag.IntVar(&cfg.WriteBuf, "writebuf", 4096, "websocket 写缓冲字节数")
	flag.DurationVar(&cfg.Heartbeat, "heartbeat", 20*time.Second, "心跳间隔")
	flag.DurationVar(&cfg.HeartbeatAck, "hb-ack", 60*time.Second, "心跳应答窗口")
	flag.DurationVar(&cfg.RouteTTL, "route-ttl", 60*time.Second, "路由条目 TTL")
	flag.DurationVar(&cfg.RouteCacheTTL, "cache-ttl", 2*time.Second, "本地路由缓存 TTL")
	flag.DurationVar(&cfg.DrainTimeout, "drain-timeout", 10*time.Second, "优雅下线排空窗口")
	flag.IntVar(&cfg.LimitPerSec, "limit", 0, "建连限速：每秒令牌数，0=不限")
	flag.IntVar(&cfg.LimitBurst, "burst", 100, "建连限速桶容量")
	flag.BoolVar(&cfg.RouteCache, "route-cache", true, "下行路由使用本地缓存+失效通知")
	flag.Parse()
	if cfg.Advertise == "" {
		cfg.Advertise = cfg.Addr
	}

	g := gw.New(cfg)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("%s: SIGTERM -> drain", cfg.ID)
		g.Stop()
		os.Exit(0)
	}()
	log.Printf("%s: listening %s redis=%s readbuf=%d writebuf=%d route-cache=%v limit=%d/s",
		cfg.ID, cfg.Addr, cfg.RedisAddr, cfg.ReadBuf, cfg.WriteBuf, cfg.RouteCache, cfg.LimitPerSec)
	if err := g.Start(); err != nil {
		log.Fatal(err)
	}
}
