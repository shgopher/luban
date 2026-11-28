// 数据库部署拓扑选型计算器：把「架构决策链」算成一组可核对的数。
//
// 它不回答「用哪个数据库」，只回答一个架构问题：
// 给定读 QPS、写 TPS、数据量、RPO/RTO 与单价，这台数据库在架构里该摆成什么形状、
// 要几个盒子、每月花多少钱、卡在哪一步。
package main

import (
	"flag"
	"fmt"
	"math"
	"strings"
)

type input struct {
	readQPS      float64 // 峰值读 QPS
	writeTPS     float64 // 峰值写 TPS
	readPerNode  float64 // 单个只读副本的安全读能力（已按 70% 水位折算）
	cacheAbsorb  float64 // 缓存吸收的读比例，0–0.9
	staleBudget  int     // 业务允许的陈旧上界（ms）
	rpoSec       int     // 允许丢多少秒数据，0 = 不能丢
	rtoSec       int     // 目标恢复时间（秒）
	rows         float64 // 单表行数
	dataGB       float64 // 单库数据量
	az           float64 // 可用区数
	pricePerNode float64 // 单实例月单价（元）
	binlogMBps   float64 // 写放大后 binlog 速率 MB/s
}

type plan struct {
	topology   string   // 结论：拓扑名
	verdict    string   // 一句话理由
	replicas   int      // 只读副本数
	sharded    bool     // 是否需要分片
	shardWhy   string   // 分片的驱动因素：写 / 容量
	steps      []string // 决策链每一步的结论
	risks      []string // 触发但未违反的风险
	blockers   []string // 硬约束（超过能力边界）
	readCap    float64  // 总读能力
	costMonth  float64
	avail      float64 // 单次故障 MTTR 折算的年可用性
	downtimeYr float64 // 年停机小时
	fanoutMBps float64
}

const (
	rowsShardThreshold = 20_000_000 // 2000 万行（行宽 1KB，3 层 B+ 树）
	gbShardThreshold   = 500.0      // 单库 500GB 以上先考虑归档/分片
	replicaComfortMax  = 5          // 单主 3–5 个从库是舒适区
	replicaHardMax     = 8          // 超过 8 个改级联/树形
)

func compute(in input) plan {
	var p plan
	safeWrite := in.writePeriodSafe()

	// 步 1：写侧。写不动才是分片的理由，读不动只需加从库。
	if in.writeTPS > safeWrite {
		p.sharded = true
		p.shardWhy = "写 TPS 撞墙"
		p.blockers = append(p.blockers, fmt.Sprintf(
			"写 %0.0f TPS > 单主安全写 %0.0f TPS（已按 70%% 水位折算）：主库写入已撞墙，加从库无用",
			in.writeTPS, safeWrite))
		p.steps = append(p.steps, fmt.Sprintf("写侧：%0.0f TPS 超安全线 %0.0f → 必须拆写（分片 / NewSQL）", in.writeTPS, safeWrite))
	} else {
		p.steps = append(p.steps, fmt.Sprintf("写侧：%0.0f TPS 在安全线 %0.0f 内（水位 %0.0f%%）→ 写仍可单主",
			in.writeTPS, safeWrite, in.writeTPS/safeWrite*100))
	}

	// 步 2：数据量。单表 2000 万行 / 单库 500GB 是「该动手」的信号。
	if in.rows > rowsShardThreshold || in.dataGB > gbShardThreshold {
		p.sharded = true
		if p.shardWhy == "" {
			p.shardWhy = "单表容量到顶"
		} else {
			p.shardWhy += " + 单表容量到顶"
		}
		p.steps = append(p.steps, fmt.Sprintf("容量：单表 %0.0f 万行、单库 %0.0f GB 超阈值（%0.0f 万行 / %0.0f GB）→ 归档或分片",
			in.rows/1e4, in.dataGB, rowsShardThreshold/1e4, gbShardThreshold))
	} else {
		p.steps = append(p.steps, fmt.Sprintf("容量：单表 %0.0f 万行、单库 %0.0f GB 未超阈值 → 不因容量分片",
			in.rows/1e4, in.dataGB))
	}

	// 步 3：读侧。先扣缓存，再按 AZ 冗余折算，最后除以单副本安全读能力。
	effective := in.readQPS * (1 - in.cacheAbsorb)
	azFactor := 1 / (1 - 1/in.az)
	needCap := effective * azFactor
	// 主库本身也能承担一部分读：需求没超过一个副本的安全读能力时，不额外加副本。
	n := 0
	if needCap > in.readPerNode {
		n = int(math.Ceil(needCap / in.readPerNode))
	}
	// RTO 短于人工切换能力时，至少要有一台可提升的副本，否则切换没有对象；
	// 能接受小时级停机的小系统可以真的只有一台盒子。
	minReplicas := 0
	if in.rtoSec <= 300 {
		minReplicas = 1
	}
	if n < minReplicas {
		n = minReplicas
	}
	p.replicas = n
	// 主库也能承担一部分读，但先要扣掉写占用的水位；副本才是干净读能力。
	primaryRead := in.readPerNode * (1 - in.writeTPS/in.writePeriodSafe()*0.7)
	if primaryRead < 0 {
		primaryRead = 0
	}
	p.readCap = primaryRead + float64(n)*in.readPerNode
	p.steps = append(p.steps, fmt.Sprintf(
		"读侧：%0.0f QPS ×(1-缓存 %0.0f%%)=%0.0f，再乘 AZ 冗余 %0.2f → 需 %0.0f QPS，单副本 %0.0f → %d 个副本",
		in.readQPS, in.cacheAbsorb*100, effective, azFactor, needCap, in.readPerNode, n))

	// 步 4：副本数上限。加从库不是免费的：主库扇出、dump 线程、回放并行度都会先满。
	switch {
	case n > replicaHardMax:
		p.risks = append(p.risks, fmt.Sprintf(
			"%d 个副本已超硬上限 %d：改树形/级联复制，或把只读流量拆到分片/搜索引擎", n, replicaHardMax))
	case n > replicaComfortMax:
		p.risks = append(p.risks, fmt.Sprintf(
			"%d 个副本超舒适区 %d：先做缓存与归档，再考虑级联复制", n, replicaComfortMax))
	}
	p.fanoutMBps = float64(n) * in.binlogMBps
	if p.fanoutMBps > 500 {
		p.risks = append(p.risks, fmt.Sprintf(
			"主库扇出 %0.0f MB/s（%d×%0.1f）：网卡先于 CPU 成为瓶颈，需树形中继", p.fanoutMBps, n, in.binlogMBps))
	}
	p.steps = append(p.steps, fmt.Sprintf("扇出：%d 个副本 × %0.1f MB/s = %0.0f MB/s 主库上行",
		n, in.binlogMBps, p.fanoutMBps))

	// 步 5：复制模式由 RPO 决定，不由「最佳实践」决定。
	switch {
	case in.rpoSec == 0:
		p.steps = append(p.steps, "RPO=0：异步复制不达标，需半同步（AFTER_SYNC）或组复制/共识复制，写延迟 +1 次 RTT")
	case in.rpoSec <= 5:
		p.steps = append(p.steps, fmt.Sprintf("RPO≤%ds：异步复制通常满足（丢最近 1ms–数秒），但要接受切换时丢尾部", in.rpoSec))
	default:
		p.steps = append(p.steps, fmt.Sprintf("RPO≤%ds：异步复制 + 定时备份即可", in.rpoSec))
	}

	// 步 6：切换方式决定 MTTR，MTTR 决定几个 9。
	var mttr float64
	switch {
	case in.rtoSec <= 60:
		mttr = 60
		p.steps = append(p.steps, "RTO≤60s：必须自动切换（选主+VIP/代理改指向），人工不可能达标")
	case in.rtoSec <= 300:
		mttr = 300
		p.steps = append(p.steps, "RTO≤5min：脚本化切换 + 值班可执行，需预演过")
	default:
		mttr = 1800
		p.steps = append(p.steps, "RTO>5min：可接受人工切换，但要把「谁来点」写进值班表")
	}
	// 显式假设：每月 1 次故障（12 次/年），年停机 = 12 × MTTR。
	// 可用性必须同时说清 MTBF 与 MTTR，只报 MTTR 等于把可用性报成 6 个 9。
	p.downtimeYr = 12 * mttr / 3600
	p.avail = 1 - p.downtimeYr/8760

	// 步 7：钱。盒子数 × 单价，并给出「每万 QPS 每月多少钱」。
	nodes := 1 + n
	p.costMonth = float64(nodes) * in.pricePerNode
	per10k := p.costMonth / (p.readCap / 10000) // 每万 QPS 的月成本
	p.steps = append(p.steps, fmt.Sprintf(
		"成本：%d 个盒子（1 主 + %d 从）× %0.0f 元 = %0.0f 元/月；每万读 QPS %0.0f 元/月",
		nodes, n, in.pricePerNode, p.costMonth, per10k))

	// 结论
	switch {
	case p.sharded && n > 0:
		p.topology = "分片 + 只读副本"
		p.verdict = fmt.Sprintf("%s：按该维度分片，每个分片再按读维度挂副本", p.shardWhy)
	case p.sharded:
		p.topology = "分片（每片单主）"
		p.verdict = fmt.Sprintf("%s；读不是瓶颈，分片即可，不必先加副本", p.shardWhy)
	case n == 0:
		p.topology = "单机"
		p.verdict = "读写都在单机安全线内"
	case in.rpoSec == 0:
		p.topology = fmt.Sprintf("一主 %d 从（半同步/组复制）", n)
		p.verdict = "读需扩展且不能丢数据：半同步把 RPO 压到 0，代价是每次提交多一次 RTT"
	default:
		p.topology = fmt.Sprintf("一主 %d 从（异步）", n)
		p.verdict = "读需扩展、可容忍秒级丢失：异步复制最便宜"
	}
	return p
}

// writePeriodSafe 返回单主库的安全写能力（70% 水位）。
func (in input) writePeriodSafe() float64 { return 4000 * 0.7 }

func (in input) stalenessAdvice(replicas int) string {
	if replicas == 0 {
		return "无副本：陈旧预算不适用，可用性只能由备份恢复时长（RTO）兜底"
	}
	switch {
	case in.staleBudget <= 50:
		return "陈旧预算 ≤50ms：全读从库会违反承诺，必须写后读主或按 GTID 位点等待"
	case in.staleBudget <= 1000:
		return "陈旧预算 ≤1s：会话粘滞 + 短 TTL 缓存 + 写后读主，三件套一起上"
	default:
		return "陈旧预算 ≥1s：可全量读从库，把「最坏能旧多久」写进架构图即可"
	}
}

func main() {
	in := input{}
	flag.Float64Var(&in.readQPS, "read", 60000, "峰值读 QPS")
	flag.Float64Var(&in.writeTPS, "write", 800, "峰值写 TPS")
	flag.Float64Var(&in.readPerNode, "read-per-node", 12000, "单个只读副本的安全读 QPS（70% 水位）")
	flag.Float64Var(&in.cacheAbsorb, "cache", 0.3, "缓存吸收的读比例，0–0.9")
	flag.IntVar(&in.staleBudget, "stale", 500, "业务允许的陈旧上界（ms）")
	flag.IntVar(&in.rpoSec, "rpo", 0, "允许丢多少秒数据，0 = 不能丢")
	flag.IntVar(&in.rtoSec, "rto", 60, "目标恢复时间（秒）")
	flag.Float64Var(&in.rows, "rows", 5e7, "最大单表行数")
	flag.Float64Var(&in.dataGB, "data", 800, "单库数据量 GB")
	flag.Float64Var(&in.az, "az", 3, "可用区数")
	flag.Float64Var(&in.pricePerNode, "price", 2000, "单实例月单价（元）")
	flag.Float64Var(&in.binlogMBps, "binlog", 2, "写放大后 binlog 速率 MB/s")
	flag.Parse()

	p := compute(in)

	fmt.Println("== 输入 ==")
	fmt.Printf("  读 %0.0f QPS / 写 %0.0f TPS / 缓存吸收 %0.0f%% / 陈旧预算 %dms\n",
		in.readQPS, in.writeTPS, in.cacheAbsorb*100, in.staleBudget)
	fmt.Printf("  单表 %0.0f 万行 / 单库 %0.0f GB / %0.0f AZ / RPO %ds / RTO %ds\n",
		in.rows/1e4, in.dataGB, in.az, in.rpoSec, in.rtoSec)

	fmt.Println("\n== 决策链 ==")
	for i, s := range p.steps {
		fmt.Printf("  %d. %s\n", i+1, s)
	}

	fmt.Println("\n== 结论 ==")
	fmt.Printf("  拓扑：%s\n  理由：%s\n", p.topology, p.verdict)
	fmt.Printf("  读能力：%0.0f QPS（水位 %0.0f%%）\n", p.readCap, in.readQPS*(1-in.cacheAbsorb)/p.readCap*100)
	fmt.Printf("  可用性：MTTR %0.0fs × 每月 1 次故障 → 年停机 %0.2fh，可用性 %0.4f%%（%0.2f 个 9）\n",
		mttrOf(in), p.downtimeYr, p.avail*100, nines(p.avail))
	fmt.Printf("  费用：%0.0f 元/月，%0.0f 元/年\n", p.costMonth, p.costMonth*12)

	fmt.Println("\n== 陈旧预算 ==")
	fmt.Printf("  %s\n", in.stalenessAdvice(p.replicas))

	if len(p.risks) > 0 {
		fmt.Println("\n== 风险（触发但未违反硬约束）==")
		for _, r := range p.risks {
			fmt.Printf("  - %s\n", r)
		}
	}
	if len(p.blockers) > 0 {
		fmt.Println("\n== 硬约束（必须处理，否则拓扑无效）==")
		for _, b := range p.blockers {
			fmt.Printf("  - %s\n", b)
		}
	}

	fmt.Println("\n== 架构图标注（可直接贴进设计文档）==")
	fmt.Printf("  [写入口] → (主库) ──扇出 %0.0f MB/s──→ %d × (只读副本) → [读入口]\n", p.fanoutMBps, p.replicas)
	fmt.Printf("  标注：RPO=%ds  RTO=%ds  最坏陈旧=%dms  写入口唯一  切换执行者=自动/值班  备份独立于副本\n",
		in.rpoSec, in.rtoSec, in.staleBudget)

	fmt.Println("\n== 决策记录（ADR 片段）==")
	fmt.Printf("  决定：数据库采用「%s」。\n", p.topology)
	rpoWhy := "RPO=0 决定必须半同步/组复制，异步被排除"
	if in.rpoSec > 0 {
		rpoWhy = fmt.Sprintf("RPO=%ds 由异步复制兜住，不额外付半同步的 RTT", in.rpoSec)
	}
	fmt.Printf("  依据：%s；读 %0.0f QPS 需 %d 个副本（含 AZ 冗余 %s）；%s；RTO=%ds 决定切换方式。\n",
		strings.TrimSuffix(p.verdict, "。"), in.readQPS, p.replicas, azNote(in.az), rpoWhy, in.rtoSec)
	fmt.Printf("  代价：每月 %0.0f 元；主库扇出 %0.0f MB/s；故障时年停机预算 %0.2f 小时。\n", p.costMonth, p.fanoutMBps, p.downtimeYr)
}

func mttrOf(in input) float64 {
	switch {
	case in.rtoSec <= 60:
		return 60
	case in.rtoSec <= 300:
		return 300
	default:
		return 1800
	}
}

func nines(a float64) float64 { return -math.Log10(1 - a) }

func azNote(az float64) string {
	if az >= 3 {
		return "1/(1-1/3)=1.5"
	}
	return fmt.Sprintf("1/(1-1/%0.0f)", az)
}
