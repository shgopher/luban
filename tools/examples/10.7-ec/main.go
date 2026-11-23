// EC(k,m) vs 3 副本：存储开销 / 容错 / 重建代价算例（单位：TB、MiB、秒）
// 口径：1PB 数据 = 1024TB；条带 stripe = 128MiB；单价与带宽均为示例，按自身账单替换。
package main

import "fmt"

const (
	dataTB       = 1024.0 // 1PB 原始数据
	stripeMiB    = 128.0  // 一个条带的原始数据量
	priceTBMonth = 20.0   // 元/TB/月，存储单价
	failRate     = 0.01   // 每年故障的块比例（示例）
	nodeBW       = 100.0  // MB/s，单节点读带宽
	sharedBW     = 125.0  // MB/s，共享出口带宽（1Gbps）
	decodeBW     = 500.0  // MB/s，单核解码速度
)

type cfg struct {
	name string
	k, m int
}

func (c cfg) overhead() float64   { return float64(c.k+c.m) / float64(c.k) }
func (c cfg) chunkMiB() float64   { return stripeMiB / float64(c.k) }
func (c cfg) rebuildMiB() float64 { return c.chunkMiB() * float64(c.k) } // 读 k 块 = 1 份条带
func (c cfg) readAmp() float64    { return float64(c.k) }                // 读字节 / 修复字节

func main() {
	ecs := []cfg{{"EC 4+2", 4, 2}, {"EC 6+3", 6, 3}, {"EC 8+3", 8, 3},
		{"EC 10+4", 10, 4}, {"EC 12+4", 12, 4}, {"EC 16+4", 16, 4}}

	fmt.Println("== 1. EC(k,m) 参数扫描（1PB 原始数据，条带 128MiB） ==")
	fmt.Println("方案 | 空间开销 | 实存TB | 允许失效 | 块大小MiB | 重建读块数 | 重建读MiB | 读放大")
	for _, c := range ecs {
		fmt.Printf("%s | %.3f | %.1f | %d | %.1f | %d | %.0f | %.0fx\n",
			c.name, c.overhead(), dataTB*c.overhead(), c.m, c.chunkMiB(), c.k, c.rebuildMiB(), c.readAmp())
	}
	fmt.Println("3 副本 | 3.000 | 3072.0 | 2 | 128.0 | 1 | 128 | 1x")

	fmt.Println("\n== 2. 重建代价：年修复流量与修复时间（年故障块比例 1%） ==")
	fmt.Println("方案 | 年故障存储TB | 年修复读TB | 修复流量比 | 单块修复s(单节点瓶颈) | 单块修复s(共享瓶颈) | 年修复小时")
	type row struct {
		name             string
		ov               float64
		rep              float64 // 每修复 1 字节需读的字节数
		chunkMiB, decMiB float64
	}
	rows := []row{{"3 副本", 3.0, 1.0, 128, 0}}
	for _, c := range ecs {
		rows = append(rows, row{c.name, c.overhead(), c.readAmp(), c.chunkMiB(), c.rebuildMiB()})
	}
	// 以 3 副本为基准的修复流量比 = (实存 × 放大) / (3 副本实存 × 1)
	baseRepair := dataTB * 3.0 * failRate
	for _, r := range rows {
		repairTB := dataTB * r.ov * failRate * r.rep
		// 单节点瓶颈：每块从各自节点并行读，时间 = 块大小/单节点带宽 + 解码
		tNode := r.chunkMiB/nodeBW + r.decMiB/decodeBW
		if r.rep == 1.0 {
			tNode = r.chunkMiB / nodeBW // 副本只拷 1 份，无解码
		}
		tShared := r.decMiB/sharedBW + r.decMiB/decodeBW // 共享带宽下先搬完整条带再解码
		if r.rep == 1.0 {
			tShared = r.chunkMiB / sharedBW
		}
		hours := repairTB * 1e6 / sharedBW / 3600 // 1TB = 1e6 MB
		fmt.Printf("%s | %.2f | %.2f | %.2fx | %.2f | %.2f | %.1f\n",
			r.name, dataTB*r.ov*failRate, repairTB, repairTB/baseRepair, tNode, tShared, hours)
	}

	fmt.Println("\n== 3. 小对象账：EC 4+2 vs 3 副本（元数据/IOPS 是每对象固定开销） ==")
	fmt.Println("对象大小 | 3副本实存KiB | EC4+2实存KiB | 元数据条目(副本/EC) | 写块数 | 读整对象块数 | 1万对象年省存储费元")
	for _, sizeKiB := range []float64{64, 1024, 16384, 1048576} {
		repStored := sizeKiB * 3
		ecStored := sizeKiB * 1.5
		savedTB := (repStored - ecStored) * 1024 / (1024.0 * 1024 * 1024 * 1024)
		fmt.Printf("%.0fKiB | %.0f | %.0f | 3/6 | 3/6 | 1/4 | %.4f\n",
			sizeKiB, repStored, ecStored, savedTB*priceTBMonth*12*10000)
	}

	fmt.Println("\n== 4. 随机更新 16MiB：EC 8+3 的读-改-写放大 ==")
	fmt.Println("方案 | 更新单位 | 读块数 | 写块数 | 读MiB | 写MiB | 总IO/更新字节")
	// EC 8+3：更新 1 个数据块(16MiB) → 读其余 7 块、重算 3 个校验、写 1 数据 + 3 校验
	ecIO := 7*16 + 4*16
	fmt.Printf("EC 8+3 | 16MiB | 7 | 4 | %d | %d | %.2fx\n", 7*16, 4*16, float64(ecIO)/16)
	// 3 副本：更新 128MiB 数据 = 写 3 份
	fmt.Printf("3 副本 | 16MiB | 0 | 3 | 0 | %d | %.2fx\n", 3*16, 3.0)
}
