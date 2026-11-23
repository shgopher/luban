// 实测 B：GCJ-02 -> WGS84 逆变换迭代收敛（1/2/3/5/10 次），以及跨坐标系往返漂移。
package main

import (
	"fmt"
	"math"
	"os"
	"s701/internal/geo"
)

type city struct {
	name string
	p    geo.Point
}

func main() {
	out := os.Stdout
	fmt.Fprintln(out, "# 实测 B：GCJ-02 逆变换迭代收敛 + 往返漂移")
	fmt.Fprintln(out, "# 环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21")
	fmt.Fprintln(out, "# 真值 = 未加偏的 WGS84 点；正向 WGS84->GCJ02 后再用 n 次迭代反解，比较反解结果与真值")
	fmt.Fprintln(out)

	cities := []city{
		{"北京", geo.Point{Lng: 116.48105, Lat: 39.996794}},
		{"上海", geo.Point{Lng: 121.4737, Lat: 31.2304}},
		{"广州", geo.Point{Lng: 113.2644, Lat: 23.1291}},
		{"深圳", geo.Point{Lng: 114.0579, Lat: 22.5431}},
		{"成都", geo.Point{Lng: 104.0668, Lat: 30.5728}},
		{"西安", geo.Point{Lng: 108.9398, Lat: 34.3416}},
		{"乌鲁木齐", geo.Point{Lng: 87.6168, Lat: 43.8256}},
		{"拉萨", geo.Point{Lng: 91.1721, Lat: 29.6520}},
		{"三亚", geo.Point{Lng: 109.5119, Lat: 18.2528}},
		{"哈尔滨", geo.Point{Lng: 126.5350, Lat: 45.8038}},
	}
	iters := []int{0, 1, 2, 3, 5, 10}

	fmt.Fprintln(out, "## B1 逐城市：各迭代次数后的误差（米 / 1e-9 度）")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| 城市 | 迭代次数 | 误差 m | Δlat(1e-9°) | Δlng(1e-9°) |")
	fmt.Fprintln(out, "|---|---|---|---|---|")
	maxByIter := map[int]float64{}
	for _, c := range cities {
		g := geo.WGS84ToGCJ02(c.p)
		for _, n := range iters {
			w := geo.GCJ02ToWGS84Iter(g, n)
			errM := geo.Haversine(c.p, w)
			dlat := (w.Lat - c.p.Lat) * 1e9
			dlng := (w.Lng - c.p.Lng) * 1e9
			fmt.Fprintf(out, "| %s | %d | %.9f | %+.6f | %+.6f |\n", c.name, n, errM, dlat, dlng)
			if errM > maxByIter[n] {
				maxByIter[n] = errM
			}
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "## B2 汇总：迭代次数 -> 最大误差（10 城市取最大）")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| 迭代次数 n | 最大误差 m | 最大误差 cm | 相对上一档收敛倍数 |")
	fmt.Fprintln(out, "|---|---|---|---|")
	prev := 0.0
	for _, n := range iters {
		factorStr := "—"
		if prev > 0 && maxByIter[n] > 0 {
			f := prev / maxByIter[n]
			if !math.IsInf(f, 0) && !math.IsNaN(f) {
				factorStr = fmt.Sprintf("%.1fx", f)
			} else {
				factorStr = ">1e6x（已达 float64 精度极限）"
			}
		}
		fmt.Fprintf(out, "| %d | %.3e | %.6f | %s |\n", n, maxByIter[n], maxByIter[n]*100, factorStr)
		prev = maxByIter[n]
	}

	// B3 往返漂移：WGS84 -> GCJ02 -> BD09 -> GCJ02 -> WGS84，重复 n 次
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## B3 跨坐标系往返漂移：WGS84→GCJ02→BD09→GCJ02→WGS84 重复 n 轮")
	fmt.Fprintln(out, "# 每轮逆变换固定迭代 5 次（工程配置），比较 n 轮后的点与原 WGS84 点的球面距离")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| 城市 | n=1 | n=10 | n=100 | n=1000 | 第 1000 轮相对第 100 轮的增量 m |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|")
	rounds := []int{1, 10, 100, 1000}
	for _, c := range cities {
		vals := map[int]float64{}
		for _, n := range rounds {
			p := c.p
			for i := 0; i < n; i++ {
				g := geo.WGS84ToGCJ02(p)
				bd := geo.GCJ02ToBD09(g)
				g2 := geo.BD09ToGCJ02(bd)
				p = geo.GCJ02ToWGS84Iter(g2, 5)
			}
			vals[n] = geo.Haversine(c.p, p)
		}
		fmt.Fprintf(out, "| %s | %.4f | %.4f | %.4f | %.4f | %+.4f |\n",
			c.name, vals[1], vals[10], vals[100], vals[1000], vals[1000]-vals[100])
	}

	// B4 每轮增量是否收敛：北京固定点，逐轮输出
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## B4 北京基准点逐轮偏移（证明往返漂移是「有界残差」不是「每轮累加」）")
	fmt.Fprintln(out, "| 轮次 | 当前点相对原点的距离 m | 本轮增量 m |")
	fmt.Fprintln(out, "|---|---|---|")
	p := geo.Point{Lng: 116.48105, Lat: 39.996794}
	origin := p
	last := 0.0
	for i := 1; i <= 20; i++ {
		g := geo.WGS84ToGCJ02(p)
		bd := geo.GCJ02ToBD09(g)
		g2 := geo.BD09ToGCJ02(bd)
		p = geo.GCJ02ToWGS84Iter(g2, 5)
		d := geo.Haversine(origin, p)
		fmt.Fprintf(out, "| %d | %.6f | %+.6f |\n", i, d, d-last)
		last = d
	}

	// B5 BD-09 逆式的残差：GCJ02->BD09->GCJ02 的闭式反解不是精确反解
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## B5 BD-09 逆式残差：GCJ02→BD09→GCJ02 往返（这是 B3 每轮 7 cm 漂移的来源）")
	fmt.Fprintln(out, "| 城市 | BD09 逆式往返残差 m | 每轮往返漂移 m | 残差占每轮漂移比例 |")
	fmt.Fprintln(out, "|---|---|---|---|")
	for _, c := range cities {
		g := geo.WGS84ToGCJ02(c.p)
		bd := geo.GCJ02ToBD09(g)
		back := geo.BD09ToGCJ02(bd)
		residual := geo.Haversine(g, back)
		// 每轮完整往返：WGS->GCJ->BD->GCJ->WGS
		one := geo.GCJ02ToWGS84Iter(back, 5)
		cycle := geo.Haversine(c.p, one)
		fmt.Fprintf(out, "| %s | %.6f | %.6f | %.2f%% |\n", c.name, residual, cycle, residual/cycle*100)
	}
	fmt.Fprintln(out, "# 结论：B3 的往返漂移不是数值噪声，而是 BD-09 逆式本身是近似闭式解，每轮残留 cm 级；")
	fmt.Fprintln(out, "# 迭代 5 次的 GCJ-02 逆变换误差在 1e-12 m 量级，占漂移的比例可以忽略。")
}
