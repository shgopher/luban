// Command fanin 演示「多副本并发、第一个成功即返回」：
// 结果通道带足缓冲，输掉的副本也能把结果发出去并退出，不会泄漏。
package main

import (
	"context"
	"fmt"
	"time"
)

type result struct {
	name string
	err  error
}

func call(ctx context.Context, name string, d time.Duration) result {
	select {
	case <-time.After(d):
		return result{name: name}
	case <-ctx.Done():
		return result{name: name, err: ctx.Err()}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replicas := []struct {
		name string
		lag  time.Duration
	}{
		{"replica-1", 30 * time.Millisecond},
		{"replica-2", 10 * time.Millisecond},
		{"replica-3", 20 * time.Millisecond},
	}

	ch := make(chan result, len(replicas)) // 关键：容量 = 副本数
	start := time.Now()
	for _, r := range replicas {
		go func(name string, lag time.Duration) {
			ch <- call(ctx, name, lag)
		}(r.name, r.lag)
	}

	first := <-ch
	cancel() // 第一个成功，取消其余副本
	for i := 1; i < len(replicas); i++ {
		<-ch // 收完，确保每个 goroutine 都退出
	}
	fmt.Printf("winner=%s elapsed≈%dms\n", first.name, time.Since(start).Milliseconds())
}
