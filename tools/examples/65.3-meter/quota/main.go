// 配额并发预扣：1000 个并发请求抢 100 的配额，对比「先查再改」与「原子预扣」的真实超卖量
// 纯标准库：go run quota.go
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	limit    = 100
	requests = 1000
)

// 错误实现：先查再改，且"查"有存储往返（Redis/DB 约 1ms）。
// 窗口内所有请求都读到同一个旧值，各自判定通过。
func wrongWithLatency() int64 {
	var used, admitted int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen := atomic.LoadInt64(&used) // ① 查
			time.Sleep(time.Millisecond)    // ② 存储往返
			if seen < limit {               // ③ 用旧值判定
				atomic.AddInt64(&used, 1) // ④ 扣
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}
	wg.Wait()
	return admitted
}

// 错误实现：先查再改，窗口极窄（没有模拟延迟），中间让出一次调度。
// 窗口窄不等于不存在：真实系统里 GC、线程切换、P 抢占都会撑开它。
func wrongNarrow(gosched bool) int64 {
	var used, admitted int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen := atomic.LoadInt64(&used)
			if gosched {
				runtime.Gosched()
			}
			if seen < limit {
				atomic.AddInt64(&used, 1)
				atomic.AddInt64(&admitted, 1)
			}
		}()
	}
	wg.Wait()
	return admitted
}

// 正确实现：CAS 预扣。判定与扣减是同一个原子操作，没有窗口。
func casReserve() int64 {
	var used, admitted int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				cur := atomic.LoadInt64(&used)
				if cur >= limit {
					return // 配额已满，拒绝
				}
				if atomic.CompareAndSwapInt64(&used, cur, cur+1) {
					atomic.AddInt64(&admitted, 1)
					return
				}
			}
		}()
	}
	wg.Wait()
	return atomic.LoadInt64(&admitted)
}

// 预扣 + 补偿：先抢名额（CAS），抢到的去执行业务，20% 失败。
// refund=false 模拟"忘了退配额"，refund=true 是正确做法。
func reserveAndSettle(refund bool) (reserved, success, refunded, finalUsed int64) {
	var used, res int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ { // 阶段一：并发预扣
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				cur := atomic.LoadInt64(&used)
				if cur >= limit {
					return
				}
				if atomic.CompareAndSwapInt64(&used, cur, cur+1) {
					atomic.AddInt64(&res, 1)
					return
				}
			}
		}()
	}
	wg.Wait()
	reserved = atomic.LoadInt64(&res)
	for n := int64(1); n <= reserved; n++ { // 阶段二：业务执行，每 5 个失败 1 个
		if n%5 == 0 {
			if refund {
				atomic.AddInt64(&used, -1)
				refunded++
			}
			continue
		}
		success++
	}
	return reserved, success, refunded, atomic.LoadInt64(&used)
}

func main() {
	fmt.Printf("环境：%s/%s，GOMAXPROCS=%d\n", runtime.GOOS, runtime.GOARCH, runtime.GOMAXPROCS(0))
	fmt.Printf("场景：配额上限 %d，%d 个并发请求同时抢\n\n", limit, requests)

	fmt.Println("== 错误实现：先查再改（读有 1ms 存储往返）==")
	a := wrongWithLatency()
	fmt.Printf("放行 %d 个，超卖 %d 个（放行量是配额的 %.0f%%）\n\n", a, a-limit, float64(a)*100/limit)

	fmt.Println("== 错误实现：先查再改（无模拟延迟，仅在读写之间让出一次调度）==")
	var plain, sched []int64
	for i := 0; i < 5; i++ {
		plain = append(plain, wrongNarrow(false))
		sched = append(sched, wrongNarrow(true))
	}
	fmt.Printf("不让出调度 5 次放行：%v（超卖 %d）\n", plain, plain[0]-limit)
	fmt.Printf("让出一次调度 5 次放行：%v（超卖 %d–%d）\n", sched, sched[0]-limit, sched[4]-limit)
	fmt.Println("→ 同样的代码，本地连跑 5 次都可能「正好 100」，上线才超卖")
	fmt.Println()

	fmt.Println("== 正确实现：CAS 原子预扣 ==")
	b := casReserve()
	fmt.Printf("放行 %d 个，拒绝 %d 个，超卖 %d 个\n\n", b, requests-b, b-limit)

	fmt.Println("== 预扣 + 补偿（100 个预扣成功的请求里 20% 下游失败）==")
	r1, s1, _, u1 := reserveAndSettle(false)
	fmt.Printf("忘了退配额：预扣 %d，成功 %d，最终用量 %d → 按 %d 计费，多收 %d 次（占成功量的 %.0f%%）\n",
		r1, s1, u1, u1, u1-s1, float64(u1-s1)*100/float64(s1))
	r2, s2, f2, u2 := reserveAndSettle(true)
	fmt.Printf("失败退配额：预扣 %d，成功 %d，释放 %d，最终用量 %d → 按 %d 计费，多收 0%%\n",
		r2, s2, f2, u2, u2)
}
