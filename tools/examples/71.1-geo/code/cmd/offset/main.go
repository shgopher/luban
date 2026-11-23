// 实测 A：10 个城市 WGS84 -> GCJ-02 偏移量（东向/北向分解，米），以及偏移随位置变化。
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
	fmt.Fprintln(out, "# 实测 A：WGS84 -> GCJ-02 偏移量逐城市（东向/北向，米）")
	fmt.Fprintln(out, "# 环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21")
	fmt.Fprintln(out, "# 公式：公开近似式（克拉索夫斯基椭球 a=6378245, ee=0.00669342162296594323），境外不加偏")
	fmt.Fprintln(out, "# 分解口径：east = dLng * (N*cos(lat)*pi/180)；north = dLat * (M*pi/180)；total = haversine(WGS, GCJ)")
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

	fmt.Fprintln(out, "| 城市 | WGS84 lat | WGS84 lng | Δlng(1e-7°) | Δlat(1e-7°) | 东向 m | 北向 m | 合成 m | 方位角° |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|---|---|---|")
	minD, maxD := math.Inf(1), 0.0
	for _, c := range cities {
		g := geo.WGS84ToGCJ02(c.p)
		dLng := g.Lng - c.p.Lng
		dLat := g.Lat - c.p.Lat
		eastM, northM := geo.MetersPerDeg(c.p.Lat)
		de := dLng * eastM
		dn := dLat * northM
		total := geo.Haversine(c.p, g)
		bearing := math.Atan2(de, dn) * 180 / math.Pi
		if bearing < 0 {
			bearing += 360
		}
		fmt.Fprintf(out, "| %s | %.6f | %.6f | %+d | %+d | %+.2f | %+.2f | %.2f | %.1f |\n",
			c.name, c.p.Lat, c.p.Lng,
			int(math.Round(dLng*1e7)), int(math.Round(dLat*1e7)),
			de, dn, total, bearing)
		if total < minD {
			minD = total
		}
		if total > maxD {
			maxD = total
		}
	}
	fmt.Fprintf(out, "\n# 偏移合成距离范围：%.2f m ～ %.2f m；极差 %.2f m\n", minD, maxD, maxD-minD)

	// 同一城市周边 5 个点的偏移变化：证明偏移不是常数
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## 偏移随位置变化：以北京为原点，经纬各偏移 ±1° 的 5 个采样点")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| 采样点 | lat | lng | 东向 m | 北向 m | 合成 m | 与基准点差 m |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|---|")
	base := geo.Point{Lng: 116.48105, Lat: 39.996794}
	bg := geo.WGS84ToGCJ02(base)
	baseOff := geo.Haversine(base, bg)
	samples := []geo.Point{
		{Lng: 116.48105, Lat: 39.996794},
		{Lng: 117.48105, Lat: 39.996794},
		{Lng: 115.48105, Lat: 39.996794},
		{Lng: 116.48105, Lat: 40.996794},
		{Lng: 116.48105, Lat: 38.996794},
	}
	for i, p := range samples {
		g := geo.WGS84ToGCJ02(p)
		eastM, northM := geo.MetersPerDeg(p.Lat)
		de := (g.Lng - p.Lng) * eastM
		dn := (g.Lat - p.Lat) * northM
		total := geo.Haversine(p, g)
		fmt.Fprintf(out, "| s%d | %.6f | %.6f | %+.2f | %+.2f | %.2f | %+.2f |\n",
			i, p.Lat, p.Lng, de, dn, total, total-baseOff)
	}

	// 偏移向量随经度/纬度变化的连续采样（每 5°）
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## 沿北纬 40° 每 5° 经度采样；沿东经 116° 每 5° 纬度采样")
	fmt.Fprintln(out, "| 方向 | lat | lng | 东向 m | 北向 m | 合成 m |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|")
	for lng := 75.0; lng <= 135.0; lng += 5.0 {
		p := geo.Point{Lng: lng, Lat: 40.0}
		g := geo.WGS84ToGCJ02(p)
		eastM, northM := geo.MetersPerDeg(p.Lat)
		fmt.Fprintf(out, "| 经度 %5.1f | 40.000000 | %.6f | %+.2f | %+.2f | %.2f |\n",
			lng, lng, (g.Lng-p.Lng)*eastM, (g.Lat-p.Lat)*northM, geo.Haversine(p, g))
	}
	for lat := 20.0; lat <= 55.0; lat += 5.0 {
		p := geo.Point{Lng: 116.0, Lat: lat}
		g := geo.WGS84ToGCJ02(p)
		eastM, northM := geo.MetersPerDeg(p.Lat)
		fmt.Fprintf(out, "| 纬度 %.1f | %.6f | 116.000000 | %+.2f | %+.2f | %.2f |\n",
			lat, lat, (g.Lng-p.Lng)*eastM, (g.Lat-p.Lat)*northM, geo.Haversine(p, g))
	}

	// 三套坐标系互差：同一地点在 WGS84 / GCJ-02 / BD-09 下的三个坐标
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## 三套坐标系互差：WGS84 / GCJ-02 / BD-09（同一地点三个坐标 + 两两距离）")
	fmt.Fprintln(out, "| 城市 | WGS84 -> GCJ02 m | GCJ02 -> BD09 m | WGS84 -> BD09 m | 三套坐标最大互差 m |")
	fmt.Fprintln(out, "|---|---|---|---|---|")
	min3, max3 := math.Inf(1), 0.0
	for _, c := range cities {
		g := geo.WGS84ToGCJ02(c.p)
		bd := geo.GCJ02ToBD09(g)
		d1 := geo.Haversine(c.p, g)
		d2 := geo.Haversine(g, bd)
		d3 := geo.Haversine(c.p, bd)
		m3 := math.Max(d1, math.Max(d2, d3))
		fmt.Fprintf(out, "| %s | %.2f | %.2f | %.2f | %.2f |\n", c.name, d1, d2, d3, m3)
		if m3 < min3 {
			min3 = m3
		}
		if m3 > max3 {
			max3 = m3
		}
	}
	fmt.Fprintf(out, "\n# 三套坐标最大互差范围：%.2f m ～ %.2f m\n", min3, max3)

	// 境外不加偏的三个对照点
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## 境外对照：不加偏（偏移必须为 0）")
	fmt.Fprintln(out, "| 地点 | lat | lng | 合成 m |")
	fmt.Fprintln(out, "|---|---|---|---|")
	abroad := []city{
		{"东京", geo.Point{Lng: 139.6917, Lat: 35.6895}},
		{"新加坡", geo.Point{Lng: 103.8198, Lat: 1.3521}},
		{"旧金山", geo.Point{Lng: -122.4194, Lat: 37.7749}},
	}
	for _, c := range abroad {
		g := geo.WGS84ToGCJ02(c.p)
		fmt.Fprintf(out, "| %s | %.6f | %.6f | %.6f |\n", c.name, c.p.Lat, c.p.Lng, geo.Haversine(c.p, g))
	}
}
