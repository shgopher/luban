//go:build v0broken

// 这些测试故意跑不过：它们用来复现 v0 的回绕与并发缺陷。
// 运行：go test -tags v0broken -race -run TestV0

package orderid

import (
	"sync"
	"testing"
)

func TestV0Concurrent(t *testing.T) {
	const n = 500
	var wg sync.WaitGroup
	out := make([][]int64, 8)
	for i := range out {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < n; j++ {
				out[i] = append(out[i], NextV0())
			}
		}(i)
	}
	wg.Wait()
	seen := make(map[int64]struct{}, 8*n)
	for _, ids := range out {
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate id %d", id)
			}
			seen[id] = struct{}{}
		}
	}
}

func TestV0SingleThreadRollover(t *testing.T) {
	seen := make(map[int64]struct{}, 20000)
	var prev int64 = -1
	for i := 0; i < 20000; i++ {
		id := NextV0()
		if _, dup := seen[id]; dup {
			t.Fatalf("single-thread duplicate at i=%d: %d", i, id)
		}
		if id <= prev {
			t.Fatalf("single-thread not increasing at i=%d: %d <= %d", i, id, prev)
		}
		seen[id] = struct{}{}
		prev = id
	}
}
