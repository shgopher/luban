package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Audit struct {
	Seq                   int
	Actor, Action, Detail string
	Prev, Hash            string
}

func digest(prev, actor, action, detail string) string {
	h := sha256.Sum256([]byte(prev + "|" + actor + "|" + action + "|" + detail))
	return hex.EncodeToString(h[:])[:16]
}

func build(ops []Audit) []Audit {
	prev := "0000000000000000"
	for i := range ops {
		ops[i].Prev = prev
		ops[i].Hash = digest(prev, ops[i].Actor, ops[i].Action, ops[i].Detail)
		prev = ops[i].Hash
	}
	return ops
}

// verify 逐条重算：前序哈希断链或本条哈希不匹配，都判校验失败。
func verify(ops []Audit) (bool, int) {
	prev := "0000000000000000"
	for i := range ops {
		if ops[i].Prev != prev {
			return false, ops[i].Seq
		}
		if ops[i].Hash != digest(ops[i].Prev, ops[i].Actor, ops[i].Action, ops[i].Detail) {
			return false, ops[i].Seq
		}
		prev = ops[i].Hash
	}
	return true, 0
}

func main() {
	fmt.Println("== 1. 审计留痕：操作日志哈希链 ==")
	ops := build([]Audit{
		{Seq: 1, Actor: "ops-li", Action: "开户", Detail: "客户 C9001 开立 I 类户，证件已核验"},
		{Seq: 2, Actor: "ops-li", Action: "限额变更", Detail: "C9001 单笔限额 50000→20000，附审批单 SP-771"},
		{Seq: 3, Actor: "ops-wang", Action: "手工调账", Detail: "内部户 2002 转入 1000 元，双人复核：ops-zhao"},
		{Seq: 4, Actor: "ops-zhao", Action: "复核", Detail: "同意调账凭证 CZ-012，核对原始流水 P03"},
		{Seq: 5, Actor: "ops-li", Action: "导出", Detail: "导出 T 日流水 1200 条，用途：监管报送"},
	})
	for _, o := range ops {
		fmt.Printf("seq=%d %-8s %-8s hash=%s\n", o.Seq, o.Actor, o.Action, o.Hash)
	}
	if ok, at := verify(ops); ok {
		fmt.Println("校验结果：整链有效，未发现篡改")
	} else {
		fmt.Printf("校验结果：断链于 seq=%d\n", at)
	}

	// 篡改第 3 条的金额：后续所有 Prev 都对不上，无法局部伪造
	ops[2].Detail = "内部户 2002 转入 10000 元，双人复核：ops-zhao"
	if ok, at := verify(ops); !ok {
		fmt.Printf("篡改 seq=3 后校验：失败于 seq=%d（该条哈希不匹配，且后续 Prev 断链）\n", at)
	}
	fmt.Println("结论：要改一条历史记录，必须重算该条之后全部哈希；链尾哈希已对外报送，改不动。")
	fmt.Println()

	fmt.Println("== 2. RTO 逐段估算（本机计算，单位为秒）==")
	type Step struct {
		Name string
		Sec  int
	}
	steps := []Step{
		{"故障探测（心跳 3 次 × 5s）", 15},
		{"切换决策（值班 + 双人复核）", 60},
		{"流量切换（DNS TTL 30s + 网关生效）", 30},
		{"备库提升为主（改复制拓扑 + fencing）", 20},
		{"应用预热（连接池/缓存/热点数据）", 90},
		{"切换后 5 项数据校验", 150},
	}
	cum := 0
	for _, s := range steps {
		cum += s.Sec
		fmt.Printf("%-38s %4ds  累计 %3d 秒\n", s.Name, s.Sec, cum)
	}
	fmt.Printf("本机估算 RTO = %d 秒 ≈ %.1f 分钟\n\n", cum, float64(cum)/60)

	fmt.Println("== 3. RPO 估算：丢多少笔 = 复制延迟 × 写入速率 ==")
	lagSec := 2.0
	tps := 1200.0
	for _, l := range []float64{0.2, 2.0, 30.0} {
		fmt.Printf("复制延迟 %5.1fs × %4.0f TPS → 最多丢 %6.0f 笔\n", l, tps, l*tps)
	}
	fmt.Printf("本次取值：延迟 %.1fs、写入 %.0f TPS → RPO ≈ %.0f 笔 ≈ %.0f 秒的写入量\n\n",
		lagSec, tps, lagSec*tps, lagSec)

	fmt.Println("== 4. 四种形态的 RTO / RPO（经验量级，需按实际演练校准）==")
	for _, s := range []struct {
		form, rto, rpo, cost string
	}{
		{"冷备（备份介质）", "小时级（4–8h）", "小时级（丢最后一次备份后的全部写入）", "1x"},
		{"温备（备机常驻不接流）", "10–40min", "分钟级（异步复制延迟）", "1.5–2x"},
		{"热备（同城，可接流）", "1–5min", "秒级（同步/半同步复制）", "2–3x"},
		{"两地三中心 / 双活", "秒级–0", "0–秒级（按复制模式定）", "4x 以上"},
	} {
		fmt.Printf("%-24s RTO %-14s RPO %-36s 成本 %s\n", s.form, s.rto, s.rpo, s.cost)
	}

	fmt.Println("\n== 5. 回切（切回来）的代价 ==")
	for _, s := range []struct {
		item string
		sec  int
	}{
		{"确认原主库数据已补齐（追平新增写入）", 300},
		{"回切流量（灰度 1%→10%→100%）", 180},
		{"回切后双跑校验", 120},
	} {
		fmt.Printf("%-36s %4ds\n", s.item, s.sec)
	}
	fmt.Println("回切总耗时 ≥ 10 分钟：切换方案必须把回切成本一起写进预案。")
}
