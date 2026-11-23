// Command leakdemo 故意泄漏一个 goroutine，并用 Go 1.27 的 goroutineleak profile 抓出来。
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

func leak() {
	ch := make(chan int) // 函数返回后，ch 不再被任何可达对象引用
	go func() {
		ch <- 1 // 永远等不到接收者
	}()
}

func main() {
	leak()
	time.Sleep(10 * time.Millisecond) // 让泄漏的 goroutine 真正阻塞
	runtime.GC()                      // 泄漏检测依赖 GC 的可达性分析

	p := pprof.Lookup("goroutineleak")
	if p == nil {
		fmt.Println("当前 Go 版本没有 goroutineleak profile")
		return
	}
	fmt.Println("=== goroutineleak ===")
	if err := p.WriteTo(os.Stdout, 1); err != nil {
		fmt.Println("write profile:", err)
	}
}
