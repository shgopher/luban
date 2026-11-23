// Command concurrency 演示 channel 的五个高频用法：
// 信号量、非阻塞收发、关闭广播、nil 分支禁用、可取消的等待。
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	semaphoreDemo()
	nonBlockingDemo()
	closeBroadcastDemo()
	nilBranchDemo()
	cancelableDemo()
}

// 1. 带缓冲 channel 当信号量：限制同时进行的任务数。
func semaphoreDemo() {
	const (
		tasks = 8
		limit = 3
	)
	sem := make(chan struct{}, limit)
	var cur, peak atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < tasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{} // 占槽位；满了就在这里排队，天然背压
			defer func() { <-sem }()

			n := cur.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond) // 模拟工作
			cur.Add(-1)
		}()
	}
	wg.Wait()
	fmt.Printf("信号量：limit=%d tasks=%d 并发峰值=%d\n", limit, tasks, peak.Load())
}

// 2. 非阻塞收发：select + default 相当于 TrySend / TryRecv。
func nonBlockingDemo() {
	ch := make(chan int, 2)
	ok, dropped := 0, 0
	for i := 0; i < 5; i++ {
		select {
		case ch <- i:
			ok++
		default:
			dropped++ // 满则立刻走这里，绝不阻塞
		}
	}
	fmt.Printf("非阻塞发送：容量=2 写入=%d 丢弃=%d\n", ok, dropped)
}

// 3. 关闭广播：close(ch) 是所有接收者同时收到信号的唯一方式。
func closeBroadcastDemo() {
	stop := make(chan struct{})
	var exited atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-stop // 阻塞到 close，或被广播唤醒
			exited.Add(1)
		}()
	}
	close(stop)
	wg.Wait()
	fmt.Printf("关闭广播：3 个接收者全部退出，计数=%d\n", exited.Load())
}

// 4. nil channel 永久阻塞，正好用来在 select 里禁用分支。
func nilBranchDemo() {
	var disabled chan int // nil：这个 case 永远不会就绪
	ready := make(chan int, 1)
	ready <- 42

	select {
	case v := <-ready:
		fmt.Printf("nil 分支：ready 命中=%d，nil 分支永不就绪\n", v)
	case <-disabled:
		fmt.Println("nil 分支：不可能走到这里")
	case <-time.After(100 * time.Millisecond):
		fmt.Println("nil 分支：超时")
	}
}

// 5. 可取消的等待：用 Timer 而不是 time.Sleep，ctx 取消时立刻返回。
func cancelableDemo() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := sleepCtx(ctx, 200*time.Millisecond)
	fmt.Printf("可取消等待：err=%v 实际耗时≈%dms（请求 200ms）\n",
		errors.Is(err, context.DeadlineExceeded), time.Since(start).Milliseconds())
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
