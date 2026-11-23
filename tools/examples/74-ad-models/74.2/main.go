package main

import "fmt"

// 确定性伪随机：固定种子，保证每次运行输出一致
func lcg(seed uint64) func() float64 {
	x := seed
	return func() float64 {
		x = x*6364136223846793005 + 1442695040888963407
		return float64(x>>11) / float64(1<<53)
	}
}

func count(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

func main() {
	const universe = 1_000_000
	r := lcg(20261122)
	tag := make([]bool, universe)    // 兴趣标签：母婴
	upload := make([]bool, universe) // 广告主上传的 ID 包
	for i := 0; i < universe; i++ {
		if r() < 0.12 {
			tag[i] = true
		}
		if r() < 0.08 {
			upload[i] = true
		}
	}
	exclude := make([]bool, universe) // 排除包：30 天内已购买
	for i := 0; i < universe; i++ {
		if upload[i] && r() < 0.25 {
			exclude[i] = true
		}
	}

	tagN, uploadN := count(tag), count(upload)
	interN := 0
	for i := 0; i < universe; i++ {
		if tag[i] && upload[i] {
			interN++
		}
	}
	union := make([]bool, universe)
	exN := 0
	for i := 0; i < universe; i++ {
		union[i] = tag[i] || upload[i]
		if union[i] && exclude[i] {
			exN++
		}
	}
	unionN := count(union)

	fmt.Println("=== 74.2 人群集合运算（本机仿真，universe = 100 万设备）===")
	fmt.Printf("兴趣标签包 %7d\n", tagN)
	fmt.Printf("上传 ID 包 %7d\n", uploadN)
	fmt.Printf("交集（标签 ∩ 上传） %7d\n", interN)
	fmt.Printf("并集（标签 ∪ 上传） %7d\n", unionN)
	fmt.Printf("排除包（上传包中已购买） %7d\n", exN)
	fmt.Printf("可投放人群（并集 − 排除） %7d（占 universe %.1f%%）\n",
		unionN-exN, float64(unionN-exN)/universe*100)

	fmt.Println("\n=== lookalike 扩量：扩得越狠，精度越低（精度为仿真假设值）===")
	type step struct {
		mult      int
		precision float64
	}
	for _, s := range []step{{1, 0.45}, {3, 0.28}, {10, 0.16}, {30, 0.09}, {100, 0.05}} {
		reach := interN * s.mult
		if reach > unionN-exN {
			reach = unionN - exN // 扩量不能超过可投放人群
		}
		fmt.Printf("%3d 倍扩量 → 覆盖 %8d 人，精度 %.2f（可触达真目标 %8d 人）\n",
			s.mult, reach, s.precision, int(float64(reach)*s.precision))
	}

	fmt.Println("\n=== 匹配率：人群包能不能在媒体侧落地（以官方文档为准）===")
	for _, m := range []struct {
		id   string
		rate float64
	}{
		{"手机号哈希", 0.55},
		{"OAID / Android", 0.70},
		{"IDFA / iOS（ATT 后）", 0.15},
		{"设备号 + IP 兜底", 0.35},
	} {
		fmt.Printf("%-22s 匹配率 %.0f%% → 最终可投 %7d 人\n",
			m.id, m.rate*100, int(float64(unionN-exN)*m.rate))
	}
}
