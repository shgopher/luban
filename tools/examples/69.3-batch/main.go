// 上行合并：100 台设备各发 10 条 60 字节遥测，对比「逐条发」与「按 1s 窗口合并」的报文开销。
package main

import (
	"encoding/json"
	"fmt"
)

type Point struct {
	Device string  `json:"d"`
	TS     int64   `json:"t"`
	Temp   float64 `json:"v"`
}

func main() {
	const devices, perDevice = 100, 10
	var naiveBytes, batchedBytes int
	var batch []Point
	for d := 0; d < devices; d++ {
		for i := 0; i < perDevice; i++ {
			p := Point{Device: fmt.Sprintf("dev-%03d", d), TS: int64(1700000000 + i), Temp: 23.5}
			b, _ := json.Marshal(p)
			naiveBytes += len(b) + 2 + len("dev/"+p.Device+"/t") // 业务载荷 + MQTT 主题长度（QoS0 固定头 2B）
			batch = append(batch, p)
		}
	}
	// 合并：一次上报一个数组，设备名与时间戳只出现一次
	agg := map[string]struct {
		TS   []int64   `json:"t"`
		Temp []float64 `json:"v"`
	}{}
	for _, p := range batch {
		a := agg[p.Device]
		a.TS = append(a.TS, p.TS)
		a.Temp = append(a.Temp, p.Temp)
		agg[p.Device] = a
	}
	type pack struct {
		BatchID string `json:"batch"`
		Data    any    `json:"data"`
	}
	b, _ := json.Marshal(pack{BatchID: "gw-1-1700000000", Data: agg})
	batchedBytes = len(b) + 2 + len("gw/dev-001/telemetry/batch")
	fmt.Printf("逐条上报：%d 条，共 %d 字节\n", len(batch), naiveBytes)
	fmt.Printf("合并上报：1 条，共 %d 字节\n", batchedBytes)
	fmt.Printf("合并后为原来的 %.1f%%，省 %.1f%% 字节\n", 100*float64(batchedBytes)/float64(naiveBytes), 100*(1-float64(batchedBytes)/float64(naiveBytes)))
	fmt.Println("样例（前 160 字节）：", string(b[:min(160, len(b))]))
}

func min(a, b int) int { if a < b { return a }; return b }
