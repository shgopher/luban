// Command fanout 演示「一个发布者、多个订阅者」的慢消费者隔离：
// 每个订阅者一个带缓冲 channel，满了就丢弃，绝不让慢订阅者拖住发布者。
package main

import "fmt"

type Hub struct {
	subs []chan int
}

// Publish 非阻塞投递；返回本轮因为缓冲满而丢弃的次数。
func (h *Hub) Publish(v int) (dropped int) {
	for _, ch := range h.subs {
		select {
		case ch <- v:
		default:
			dropped++
		}
	}
	return dropped
}

func main() {
	fast := make(chan int, 2)
	slow := make(chan int, 2)
	hub := &Hub{subs: []chan int{fast, slow}}

	var got []int
	dropped := 0
	for i := 0; i < 5; i++ {
		dropped += hub.Publish(i)
		got = append(got, <-fast) // 快速消费者每轮都取走
	}

	fmt.Println("fast:", got)
	fmt.Println("slow buffered:", len(slow), "dropped:", dropped)

	close(fast)
	close(slow)
}
