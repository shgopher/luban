// 实测 D：Web Mercator(EPSG:3857) 平面距离 vs 球面距离的膨胀倍数。
package main

import (
	"fmt"
	"math"
	"os"
	"s701/internal/geo"
)

const Rsphere = 6371000.0
const Rmerc = 6378137.0

func main() {
	out := os.Stdout
	fmt.Fprintln(out, "# 实测 D：投影坐标算距离的误差（EPSG:3857 平面 vs haversine 球面）")
	fmt.Fprintln(out, "# 环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21")
	fmt.Fprintln(out, "# 构造：同一纬度上取一对点，球面距离约 1000 m（东西向）与约 1000 m（南北向）")
	fmt.Fprintln(out, "# 3857 平面距离 = hypot(x2-x1, y2-y1)，x=Rm*lng_rad，y=Rm*ln(tan(pi/4+lat_rad/2))，Rm=6378137")
	fmt.Fprintln(out)

	lats := []float64{0, 20, 40, 60, 80}

	fmt.Fprintln(out, "## D1 东西向：同一纬度上两点，球面距离 ≈ 1000 m")
	fmt.Fprintln(out, "| 纬度 | 经度差(°) | 球面距离 m | 3857 平面距离 m | 膨胀倍数 | 绝对误差 m | 1/cos(lat) |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|---|")
	for _, lat := range lats {
		// 求使球面距离约 1000m 的经度差：d = 2R*asin(cos(lat)*sin(dlng/2))
		target := 1000.0
		cosLat := math.Cos(lat * math.Pi / 180)
		val := math.Sin(target / (2 * Rsphere)) / cosLat
		if val > 1 {
			val = 1
		}
		dlng := 2 * math.Asin(val) * 180 / math.Pi
		a := geo.Point{Lng: 116.0, Lat: lat}
		b := geo.Point{Lng: 116.0 + dlng, Lat: lat}
		dSphere := geo.Haversine(a, b)
		dPlanar := geo.MercatorPlanarDist(a, b)
		ratio := dPlanar / dSphere
		invCos := 1 / math.Cos(lat*math.Pi/180)
		fmt.Fprintf(out, "| %.0f | %.9f | %.4f | %.4f | %.6f | %+.4f | %.6f |\n",
			lat, dlng, dSphere, dPlanar, ratio, dPlanar-dSphere, invCos)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "## D2 南北向：同一经度上两点，球面距离 ≈ 1000 m")
	fmt.Fprintln(out, "| 纬度 | 纬度差(°) | 球面距离 m | 3857 平面距离 m | 膨胀倍数 | 绝对误差 m | 1/cos(lat) |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|---|")
	for _, lat := range lats {
		target := 1000.0
		dlat := target / Rsphere * 180 / math.Pi
		a := geo.Point{Lng: 116.0, Lat: lat}
		b := geo.Point{Lng: 116.0, Lat: lat + dlat}
		dSphere := geo.Haversine(a, b)
		dPlanar := geo.MercatorPlanarDist(a, b)
		ratio := dPlanar / dSphere
		invCos := 1 / math.Cos(lat*math.Pi/180)
		fmt.Fprintf(out, "| %.0f | %.9f | %.4f | %.4f | %.6f | %+.4f | %.6f |\n",
			lat, dlat, dSphere, dPlanar, ratio, dPlanar-dSphere, invCos)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "## D3 膨胀倍数与真实距离无关：同一纬度下，100 m 与 10 km 的倍数一致（等角投影的局部性质）")
	fmt.Fprintln(out, "| 真实球面距离 m | 纬度 0 倍数 | 纬度 20 倍数 | 纬度 40 倍数 | 纬度 60 倍数 | 纬度 80 倍数 |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|")
	for _, target := range []float64{100, 500, 1000, 5000, 10000} {
		row := fmt.Sprintf("| %.0f |", target)
		for _, lat := range lats {
			cosLat := math.Cos(lat * math.Pi / 180)
			val := math.Sin(target/(2*Rsphere)) / cosLat
			if val > 1 {
				val = 1
			}
			dlng := 2 * math.Asin(val) * 180 / math.Pi
			a := geo.Point{Lng: 116.0, Lat: lat}
			b := geo.Point{Lng: 116.0 + dlng, Lat: lat}
			row += fmt.Sprintf(" %.4f |", geo.MercatorPlanarDist(a, b)/geo.Haversine(a, b))
		}
		fmt.Fprintln(out, row)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "## D4 只差球半径的贡献：3857 球半径 6378137 vs haversine 6371000")
	fmt.Fprintf(out, "- 半径比 Rmerc/Rsphere = %.9f，仅此一项就让 3857 平面距离系统性偏大 %.4f%%\n",
		Rmerc/Rsphere, (Rmerc/Rsphere-1)*100)
	fmt.Fprintf(out, "- 所以纬度 0 处膨胀倍数不是 1，而是 %.6f（D1/D2 表第一行）\n", Rmerc/Rsphere)

	// D5：用 3857 平面「米」做半径圈选，实际圈到多大
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## D5 用 3857 平面半径圈选：「1 km」实际圈到多少米")
	fmt.Fprintln(out, "| 纬度 | 平面半径 1000 m 对应的地面半径 m | 少圈比例 | 地面半径 1000 m 需要平面半径 m |")
	fmt.Fprintln(out, "|---|---|---|---|")
	for _, lat := range lats {
		cosLat := math.Cos(lat * math.Pi / 180)
		ground := 1000 * cosLat
		fmt.Fprintf(out, "| %.0f | %.1f | %.1f%% | %.1f |\n", lat, ground, (1-cosLat)*100, 1000/cosLat)
	}
	fmt.Fprintln(out, "# 低纬度影响小（20° 时少圈 6%），高纬度致命（60° 时少圈 50%，80° 时少圈 83%）")
}
