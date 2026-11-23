// 边缘下沉 vs 全量回源：延迟 / 回源带宽 / 月成本模型（示例单价，按自身账单替换）
// 口径：1 万 QPS = 259.2 亿请求/月；成本单位先算“元/百万请求”，再乘 2.592 得“万元/月”。
package main

import "fmt"

const (
	qps       = 10000.0
	reqMonth  = qps * 86400 * 30 // 259.2 亿
	edgeFee   = 3.0              // 元/百万请求：边缘函数请求费
	edgeCPU   = 0.175            // 元/百万请求：4ms × 128MB（CPU 计价）
	centerCPU = 1.0              // 元/百万请求：中心处理摊销（示例）
	originGB  = 0.06             // 元/GB：边缘→中心回源流量
	rttCE     = 10.0             // ms：客户端↔边缘 RTT
	edgeExec  = 4.0              // ms：边缘执行
	rttEO     = 40.0             // ms：边缘↔中心 RTT
	originPrc = 25.0             // ms：中心处理
	coldRate  = 0.002            // 冷启动比例
	coldMs    = 200.0            // 冷启动额外延迟
)

// 每百万请求的回源字节成本（元）
func bytesCostPerM(sizeB float64) float64 { return sizeB * 1e6 / 1e9 * originGB }

// 本地完成比例 s、响应大小 sizeB 时的三类成本（元/百万请求）
func costs(s, sizeB float64) (edge, center, origin float64) {
	edge = s * (edgeFee + edgeCPU)
	center = (1 - s) * centerCPU
	origin = (1 - s) * bytesCostPerM(sizeB)
	return
}

// 平均延迟：s 走本地（含冷启动），其余走回源；s=0 的基线不走边缘=75ms
func avgLatency(s float64) float64 {
	local := rttCE + edgeExec + coldRate*coldMs
	viaEdgeFallback := rttCE + edgeExec + rttEO + originPrc
	return s*local + (1-s)*viaEdgeFallback
}

func p99(s float64) float64 {
	if 1-s > 0.01 {
		return rttCE + edgeExec + rttEO + originPrc // 回源比例 >1%，p99 落在回源路径
	}
	return rttCE + edgeExec
}

func main() {
	fmt.Println("== 1. 延迟与回源带宽（响应 20KB，边缘执行 4ms，回源 RTT 40ms） ==")
	fmt.Println("本地完成 | 平均延迟ms | p99 ms | 回源带宽TB/月")
	base := rttCE + rttEO + originPrc
	fmt.Printf("0%%（不走边缘） | %.1f | %.0f | %.1f\n", base, base, reqMonth*20000/1e12)
	for _, s := range []float64{0.3, 0.5, 0.8, 0.95, 1.0} {
		fmt.Printf("%.0f%% | %.1f | %.0f | %.1f\n", s*100, avgLatency(s), p99(s), (1-s)*reqMonth*20000/1e12)
	}

	fmt.Println("\n== 2. 月成本（万元；20KB 小响应 vs 200KB 大响应） ==")
	fmt.Println("响应 | 本地完成 | 边缘费 | 中心费 | 回源费 | 月合计 | 对比基线")
	for _, size := range []float64{20000, 200000} {
		bEdge, bCenter, bOrigin := costs(0, size)
		baseline := (bEdge + bCenter + bOrigin) * reqMonth / 1e10
		for _, s := range []float64{0, 0.5, 0.8, 1.0} {
			e, c, o := costs(s, size)
			total := (e + c + o) * reqMonth / 1e10
			tag := "基线"
			if s > 0 {
				tag = fmt.Sprintf("%+.2f", total-baseline)
			}
			fmt.Printf("%.0fKB | %.0f%% | %.2f | %.2f | %.2f | %.2f | %s\n",
				size/1000, s*100, e, c, o, total, tag)
		}
	}

	fmt.Println("\n== 3. 临界响应大小：下沉何时开始省带宽 ==")
	thr := (edgeFee + edgeCPU - centerCPU) / (1e6 / 1e9 * originGB)
	fmt.Printf("临界值 = (边缘费+CPU-中心计算) / 单位字节回源成本 = %.0f B ≈ %.1f KiB\n", thr, thr/1024)
	fmt.Println("响应大小 | 回源字节成本元/百万 | 下沉增量元/百万 | 结论")
	for _, size := range []float64{1000, 20000, 64000, 200000, 1048576} {
		bc := bytesCostPerM(size)
		delta := edgeFee + edgeCPU - centerCPU - bc
		verdict := "省钱"
		if delta > 0 {
			verdict = "更贵（买延迟）"
		}
		fmt.Printf("%.0fKB | %.2f | %+.2f | %s\n", size/1000, bc, delta, verdict)
	}

	fmt.Println("\n== 4. 只做前置拦截（不省中心计算）：拦截率阈值 ==")
	fmt.Println("响应大小 | 回源字节成本元/百万 | 需要拦掉的比例")
	for _, size := range []float64{1000, 20000, 200000} {
		bc := bytesCostPerM(size)
		r := (edgeFee + edgeCPU) / (centerCPU + bc)
		fmt.Printf("%.0fKB | %.2f | %.1f%%\n", size/1000, bc, r*100)
	}
}
