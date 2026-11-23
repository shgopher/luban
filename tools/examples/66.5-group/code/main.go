package main

// main.go: 实验入口，按 -mode 分发。
//   calib   : 标定本机 Redis 管道化吞吐（确定实验规模）
//   fanout  : 实验 A  三策略扇出基准（-strategy w|r|h -n -p -m -devices）
//   read    : 实验 A2 读侧放大（-groups 每用户群数）
//   e2e     : 实验 A3 真实订阅者端到端"最后一人收到"延迟（-n -m）
//   receipts: 实验 B  已读聚合 vs 逐人回执（-n -readers）
//   hot     : 实验 C  热点群读热点 + 混部（-readers 并发数）
//   push    : 实验 A4 逐人推送 vs 网关批量推送（-readers 在线人数 -m -batch）
//   memmap  : 内存结构标定（MEMORY USAGE + OBJECT ENCODING）
import (
	"flag"
	"fmt"
	"time"
)

func main() {
	mode := flag.String("mode", "fanout", "calib|fanout|read|e2e|receipts|hot")
	strategy := flag.String("strategy", "w", "w|r|h")
	n := flag.Int("n", 1000, "群成员数")
	p := flag.Float64("p", 1.0, "在线率 0-1")
	m := flag.Int("m", 200, "消息条数")
	devices := flag.Int("devices", 1, "人均在线设备数")
	readers := flag.Int("readers", 1000, "已读人数（实验 B）/并发读者（实验 C）")
	groups := flag.Int("groups", 10, "每用户群数（实验 A2）")
	batch := flag.Int("batch", 500, "批量推送每批收件人数（实验 A4）")
	flag.Parse()
	start := time.Now()
	switch *mode {
	case "calib":
		calib()
	case "fanout":
		fanout(*strategy, *n, *p, *m, *devices)
	case "read":
		readSide(*groups)
	case "e2e":
		e2e(*n, *m)
	case "receipts":
		receipts(*n, *readers)
	case "hot":
		hot(*readers)
	case "push":
		pushMode(*readers, *m, *batch)
	case "memmap":
		memmap()
	default:
		fmt.Println("unknown mode")
	}
	fmt.Printf("# wall=%v\n", time.Since(start).Round(time.Millisecond))
}
