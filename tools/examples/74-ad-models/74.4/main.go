package main

import (
	"fmt"
	"math"
)

func lcg(seed uint64) func() float64 {
	x := seed
	return func() float64 {
		x = x*6364136223846793005 + 1442695040888963407
		return float64(x>>11) / float64(1<<53)
	}
}

type touch struct {
	ch string
	t  float64 // 距首次触达的小时数
}

type user struct {
	id     string
	touch  []touch
	convAt float64 // 转化时间（小时）
}

var channels = []string{"信息流", "搜索", "联盟", "私域"}

// 按归因模型把一次转化拆给各渠道，返回渠道到归因份额的映射
func credit(u user, model string) map[string]float64 {
	c := map[string]float64{}
	var valid []touch
	for _, t := range u.touch {
		if u.convAt-t.t <= 168 { // 点击归因窗口 7 天
			valid = append(valid, t)
		}
	}
	if len(valid) == 0 {
		return c
	}
	switch model {
	case "last-click":
		c[valid[len(valid)-1].ch] = 1
	case "first-click":
		c[valid[0].ch] = 1
	case "linear":
		for _, t := range valid {
			c[t.ch] += 1 / float64(len(valid))
		}
	case "time-decay":
		sum := 0.0
		w := make([]float64, len(valid))
		for i, t := range valid {
			w[i] = math.Pow(0.5, (u.convAt-t.t)/48) // 半衰期 48 小时
			sum += w[i]
		}
		for i, t := range valid {
			c[t.ch] += w[i] / sum
		}
	case "position-based": // U 形：首末各 40%，中间平分 20%
		if len(valid) == 1 {
			c[valid[0].ch] = 1
		} else if len(valid) == 2 {
			c[valid[0].ch] += 0.5
			c[valid[1].ch] += 0.5
		} else {
			c[valid[0].ch] += 0.4
			c[valid[len(valid)-1].ch] += 0.4
			share := 0.2 / float64(len(valid)-2)
			for _, t := range valid[1 : len(valid)-1] {
				c[t.ch] += share
			}
		}
	}
	return c
}

func main() {
	users := []user{
		{"u1", []touch{{"信息流", 0}, {"搜索", 48}, {"联盟", 120}}, 144},
		{"u2", []touch{{"联盟", 24}, {"信息流", 96}}, 98},
		{"u3", []touch{{"搜索", 0}, {"搜索", 72}, {"私域", 144}}, 192},
		{"u4", []touch{{"信息流", 0}, {"联盟", 12}, {"搜索", 20}, {"私域", 30}}, 31},
		{"u5", []touch{{"私域", 0}}, 2},
		{"u6", []touch{{"信息流", 100}, {"搜索", 160}, {"联盟", 200}}, 220},
	}
	const value = 200.0
	spend := map[string]float64{"信息流": 300, "搜索": 250, "联盟": 150, "私域": 120}
	models := []string{"last-click", "first-click", "linear", "time-decay", "position-based"}

	fmt.Println("=== 74.4 六次转化、四个渠道：五种归因模型各分到多少钱（每次转化 200 元）===")
	fmt.Printf("%-14s", "模型")
	for _, ch := range channels {
		fmt.Printf("%10s", ch)
	}
	fmt.Println()
	rev := map[string]map[string]float64{}
	for _, m := range models {
		row := map[string]float64{}
		for _, u := range users {
			for ch, share := range credit(u, m) {
				row[ch] += share * value
			}
		}
		rev[m] = row
		fmt.Printf("%-14s", m)
		for _, ch := range channels {
			fmt.Printf("%10.1f", row[ch])
		}
		fmt.Println()
	}

	fmt.Println("\n=== 同一笔花费，不同模型下的 ROAS（归因收入 / 花费）===")
	fmt.Printf("%-14s", "模型")
	for _, ch := range channels {
		fmt.Printf("%10s", ch)
	}
	fmt.Println()
	for _, m := range models {
		fmt.Printf("%-14s", m)
		for _, ch := range channels {
			fmt.Printf("%10.2f", rev[m][ch]/spend[ch])
		}
		fmt.Println()
	}

	fmt.Println("\n=== 归因窗口：窗口越短，能归因的转化越少 ===")
	for _, win := range []float64{1, 24, 72, 168, 720} {
		cnt := 0
		for _, u := range users {
			hit := false
			for _, t := range u.touch {
				if u.convAt-t.t <= win {
					hit = true
				}
			}
			if hit {
				cnt++
			}
		}
		fmt.Printf("窗口 %4.0f 小时 → 可归因转化 %d / 6 次（%.0f%%）\n", win, cnt, float64(cnt)/6*100)
	}

	fmt.Println("\n=== 反作弊：异常点击识别与扣量（联盟 CPC 0.60 元/次）===")
	type click struct {
		dev     int
		ctitSec float64 // 点击到“安装”的间隔
		install bool
	}
	r := lcg(7404)
	var clicks []click
	for d := 0; d < 200; d++ {
		n := 1 + int(r()*2) // 正常设备 1~2 次点击
		for i := 0; i < n; i++ {
			install := r() < 0.10
			ctit := 60 + r()*3600
			if install {
				ctit = 3 + r()*600 // 正常安装：点击后 3~603 秒
			}
			clicks = append(clicks, click{d, ctit, install})
		}
	}
	for i := 0; i < 60; i++ { // 作弊设备：60 次点击、全部 1 秒内“安装”
		clicks = append(clicks, click{999, 0.5 + r()*0.4, true})
	}

	perDev := map[int][]click{}
	for _, c := range clicks {
		perDev[c.dev] = append(perDev[c.dev], c)
	}
	suspicious := 0
	deduct := 0.0
	fastInstalls := 0
	for dev, cs := range perDev {
		fast := 0
		for _, c := range cs {
			if c.install && c.ctitSec < 3 {
				fast++
			}
		}
		if len(cs) > 20 || (fast > 10 && fast*2 > len(cs)) {
			suspicious++
			deduct += float64(len(cs)) * 0.60
			if dev == 999 {
				fastInstalls = fast
			}
		}
	}
	fmt.Printf("总点击 %d 次，设备 %d 台；判定作弊设备 %d 台（含 999 号）\n", len(clicks), len(perDev), suspicious)
	fmt.Printf("999 号设备：点击 60 次，其中 %d 次点击后 3 秒内“安装”\n", fastInstalls)
	fmt.Printf("扣量点击 %d 次，扣回 %.0f 元\n", suspicious*60, deduct)
}
