package main

// 量“每租户一个连接池”的真实代价：单进程能维持多少条空闲 TCP 连接、
// 每条连接占多少 RSS、撞到 fd 上限时是什么症状。
// 客户端连接数与数据库服务端连接数一一对应，所以这组数就是连接池的原料成本。
import (
	"fmt"
	"net"
	"syscall"
	"time"
)

const target = 30000 // 3000 租户 × 每租户 10 条空闲连接

// peakRSSKB 用 getrusage 读进程峰值 RSS（macOS 上 ru_maxrss 单位是字节）。
func peakRSSKB() int {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	return int(ru.Maxrss / 1024)
}

func nofileLimit() uint64 {
	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		return 0
	}
	return rl.Cur
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, target)
	go func() { // 服务端：全部收下并按住不放，模拟“连接池里空闲的连接”
		for {
			c, err := ln.Accept()
			if err != nil {
				close(accepted)
				return
			}
			accepted <- c
		}
	}()

	addr := ln.Addr().String()
	before := peakRSSKB()
	conns := make([]net.Conn, 0, target)
	var firstErr error
	start := time.Now()
	for i := 0; i < target; i++ {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			firstErr = err
			break
		}
		// 读一行，保证服务端真的接受了这条连接（三次握手 + accept 都完成）
		c.Write([]byte("x\n"))
		conns = append(conns, c)
	}
	dial := time.Since(start)
	time.Sleep(500 * time.Millisecond) // 等 accept 侧稳定
	after := peakRSSKB()

	fmt.Printf("目标连接数 %d，实际建立 %d 条，用时 %v\n", target, len(conns), dial.Round(time.Millisecond))
	if firstErr != nil {
		fmt.Printf("第 %d 条失败：%v\n", len(conns)+1, firstErr)
	}
	if before > 0 && after > 0 {
		fmt.Printf("进程 RSS：%d KB -> %d KB，增量 %d KB，平均每条连接 %.1f KB\n",
			before, after, after-before, float64(after-before)/float64(len(conns)))
	}
	if len(conns) > 0 {
		fmt.Printf("平均建连 %.1f µs/条\n", float64(dial.Microseconds())/float64(len(conns)))
	}
	// 打印 fd 软限制，说明撞墙位置由谁决定
	fmt.Printf("RLIMIT_NOFILE 软限制=%d（每条连接在本进程内占 2 个 fd：客户端 + 服务端）\n", nofileLimit())

	fmt.Printf("再打开 1 条连接：")
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		fmt.Printf("失败 %v\n", err)
	} else {
		fmt.Printf("成功\n")
		c.Close()
	}
}
