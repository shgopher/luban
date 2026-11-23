// Command workerpool 演示生产环境最常见的 channel 结构：
// 有界任务队列 + 固定 worker + 单独的 closer 关闭结果通道。
package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Job struct{ ID int }

type Result struct {
	ID     int
	Worker int
}

func worker(ctx context.Context, id int, jobs <-chan Job, results chan<- Result) {
	for job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		time.Sleep(2 * time.Millisecond) // 模拟处理
		select {
		case results <- Result{ID: job.ID, Worker: id}:
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan Job, 4) // 有界：满了就阻塞生产者，形成背压
	results := make(chan Result, 16)
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			worker(ctx, id, jobs, results)
		}(i)
	}

	go func() {
		defer close(jobs) // 只有生产者关闭 jobs
		for i := 0; i < 10; i++ {
			jobs <- Job{ID: i}
		}
	}()

	go func() {
		wg.Wait()
		close(results) // 所有 worker 退出后，由 closer 关闭 results
	}()

	var ids []int
	workers := map[int]int{}
	for r := range results {
		ids = append(ids, r.ID)
		workers[r.Worker]++
	}
	sort.Ints(ids)
	fmt.Println("done:", len(ids))
	fmt.Println("ids:", ids)
	fmt.Println("workers:", workers)
}
