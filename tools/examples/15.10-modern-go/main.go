// Command modern-go 用一段小流程串起 Go 1.20–1.27 的现代写法：
// strings.Cut、errors.Join、slices、range n、WaitGroup.Go、sync.OnceFunc。
package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Order struct {
	ID     string
	Amount int64
}

func parseOrder(line string) (Order, error) {
	id, amountText, ok := strings.Cut(line, ",") // Go 1.18：替代 Index + 手动切片
	if !ok {
		return Order{}, errors.New("want id,amount")
	}
	amount, err := strconv.ParseInt(strings.TrimSpace(amountText), 10, 64)
	if err != nil {
		return Order{}, fmt.Errorf("parse amount %q: %w", amountText, err)
	}
	return Order{ID: strings.TrimSpace(id), Amount: amount}, nil
}

func validate(o Order) error {
	var errs []error
	if o.ID == "" {
		errs = append(errs, errors.New("empty id"))
	}
	if o.Amount <= 0 {
		errs = append(errs, fmt.Errorf("amount %d: must be > 0", o.Amount))
	}
	return errors.Join(errs...) // Go 1.20：多错合一，nil 安全
}

func total(orders []Order) int64 {
	var sum int64
	for _, o := range orders {
		sum += o.Amount
	}
	return sum
}

func ids(orders []Order) []string {
	out := make([]string, 0, len(orders)) // 已知长度就预分配
	for _, o := range orders {
		out = append(out, o.ID)
	}
	return out
}

func main() {
	lines := []string{"a-1, 100", "b-2, 250", "c-3, 0", "broken"}
	orders := make([]Order, 0, len(lines))
	bad := 0
	for _, line := range lines {
		o, err := parseOrder(line)
		if err != nil {
			bad++
			continue
		}
		if err := validate(o); err != nil {
			fmt.Printf("invalid %s: %v\n", o.ID, err)
			bad++
			continue
		}
		orders = append(orders, o)
	}
	fmt.Printf("valid=%v total=%d bad=%d\n", ids(orders), total(orders), bad)

	fmt.Println("contains b-2:", slices.Contains(ids(orders), "b-2")) // Go 1.21
	cheapest := slices.MinFunc(orders, func(a, b Order) int {         // Go 1.21
		return int(a.Amount - b.Amount)
	})
	fmt.Println("cheapest:", cheapest.ID, cheapest.Amount)

	var done atomic.Int64 // Go 1.19：类型化原子
	var wg sync.WaitGroup
	for range 3 { // Go 1.22：range over int
		wg.Go(func() { // Go 1.25：Add + go + Done 收进一个方法
			done.Add(1)
		})
	}
	wg.Wait()
	fmt.Println("workers done:", done.Load())

	initOnce := sync.OnceFunc(func() { fmt.Println("init once") }) // Go 1.21
	initOnce()
	initOnce()
}
