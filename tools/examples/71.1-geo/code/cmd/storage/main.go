// 实测 C：同一批坐标在不同存储类型下的量化误差（米）与字节/点。
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"s701/internal/geo"
)

type pt struct {
	name string
	p    geo.Point
}

// 确定性伪随机：给城市基准点加上 9 位小数的抖动，模拟真实 GPS 输出（不是 6 位整齐小数）。
func jitter(seed int, base geo.Point) geo.Point {
	s := uint64(seed)*6364136223846793005 + 1442695040888963407
	next := func() float64 {
		s = s*6364136223846793005 + 1442695040888963407
		return float64(s>>11) / float64(1<<53)
	}
	dlng := (next() - 0.5) * 0.02
	dlat := (next() - 0.5) * 0.02
	return geo.Point{Lng: base.Lng + dlng, Lat: base.Lat + dlat}
}

func quantizeInt(p geo.Point, scale float64) geo.Point {
	return geo.Point{
		Lng: math.Round(p.Lng*scale) / scale,
		Lat: math.Round(p.Lat*scale) / scale,
	}
}

func quantizeFloat32(p geo.Point) geo.Point {
	return geo.Point{Lng: float64(float32(p.Lng)), Lat: float64(float32(p.Lat))}
}

func quantizeStr(p geo.Point, digits int) geo.Point {
	format := "%." + strconv.Itoa(digits) + "f"
	lng, _ := strconv.ParseFloat(fmt.Sprintf(format, p.Lng), 64)
	lat, _ := strconv.ParseFloat(fmt.Sprintf(format, p.Lat), 64)
	return geo.Point{Lng: lng, Lat: lat}
}

func main() {
	out := os.Stdout
	fmt.Fprintln(out, "# 实测 C：坐标存储类型的量化误差")
	fmt.Fprintln(out, "# 环境：macOS arm64 / Apple 芯片 / Go 1.27.1 / 2026-11-21")
	fmt.Fprintln(out, "# 误差 = haversine(原始坐标, 量化后坐标)，单位米")
	fmt.Fprintln(out, "# 样本 = 10 个城市基准点 + 确定性伪随机抖动 (±0.01°，12 位小数，模拟真实 GPS 输出) + 4 个边界点")
	fmt.Fprintln(out)

	bases := []struct {
		name string
		p    geo.Point
	}{
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
	var pts []pt
	for i, b := range bases {
		pts = append(pts, pt{name: b.name, p: jitter(i+1, b.p)})
	}
	pts = append(pts,
		pt{"境外西经", geo.Point{Lng: -122.4194234, Lat: 37.7749123}},
		pt{"境外南纬", geo.Point{Lng: 151.2093123, Lat: -33.8688123}},
		pt{"经度边界", geo.Point{Lng: 179.9999999, Lat: 45.0000001}},
		pt{"纬度边界", geo.Point{Lng: 45.0000001, Lat: 89.9999999}},
	)

	type method struct {
		name     string
		bytes    int
		quantize func(geo.Point) geo.Point
	}
	methods := []method{
		{"float64", 16, func(p geo.Point) geo.Point { return p }},
		{"float32", 8, quantizeFloat32},
		{"int32 定点 1e-7", 8, func(p geo.Point) geo.Point { return quantizeInt(p, 1e7) }},
		{"int32 定点 1e-6", 8, func(p geo.Point) geo.Point { return quantizeInt(p, 1e6) }},
		{"int32 定点 1e-5", 8, func(p geo.Point) geo.Point { return quantizeInt(p, 1e5) }},
		{`字符串 "%.7f"`, 0, func(p geo.Point) geo.Point { return quantizeStr(p, 7) }},
		{`字符串 "%.6f"`, 0, func(p geo.Point) geo.Point { return quantizeStr(p, 6) }},
		{`字符串 "%.5f"`, 0, func(p geo.Point) geo.Point { return quantizeStr(p, 5) }},
	}

	fmt.Fprintln(out, "## C1 逐坐标误差明细（米，原始样本 12 位小数）")
	fmt.Fprintln(out)
	head := "| 坐标点 | 原始 lat / lng |"
	sep := "|---|---|"
	for _, m := range methods {
		head += " " + m.name + " |"
		sep += "---|"
	}
	fmt.Fprintln(out, head)
	fmt.Fprintln(out, sep)
	for _, p := range pts {
		row := fmt.Sprintf("| %s | %.12f / %.12f |", p.name, p.p.Lat, p.p.Lng)
		for _, m := range methods {
			q := m.quantize(p.p)
			row += fmt.Sprintf(" %.5f |", geo.Haversine(p.p, q))
		}
		fmt.Fprintln(out, row)
	}

	// C2 汇总
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## C2 汇总：最大/平均误差、量化步长、字节/点")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| 存储类型 | 最大误差 m | 平均误差 m | 量化步长 m(纬度) | 两分量字节(浮点/定点;字符串见 C3) | 备注 |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|")
	strBytes := map[string]int{}
	for _, digits := range []int{5, 6, 7} {
		for _, p := range pts {
			lng := fmt.Sprintf("%."+strconv.Itoa(digits)+"f", p.p.Lng)
			lat := fmt.Sprintf("%."+strconv.Itoa(digits)+"f", p.p.Lat)
			strBytes[fmt.Sprintf("%d", digits)] += len(lng) + len(lat) + 1
		}
	}
	for _, m := range methods {
		maxErr, sum := 0.0, 0.0
		for _, p := range pts {
			e := geo.Haversine(p.p, m.quantize(p.p))
			sum += e
			if e > maxErr {
				maxErr = e
			}
		}
		stepStr := "—"
		bytes := m.bytes
		note := ""
		switch m.name {
		case "float64":
			note = "相对误差 2^-53≈1.1e-16，误差低于 haversine 浮点噪声"
		case "float32":
			note = "相对误差 2^-24≈5.96e-8；经度 121° 处单分量约 1.1e-5°≈0.9 m"
		case "int32 定点 1e-7":
			stepStr = "0.0111"
			note = "±214.7° 覆盖全球；半量化步长 0.56 cm"
		case "int32 定点 1e-6":
			stepStr = "0.1113"
			note = "半量化步长 5.6 cm；「附近的人」够用，车道级不够"
		case "int32 定点 1e-5":
			stepStr = "1.1132"
			note = "半量化步长 56 cm；只适合城市级热力"
		case `字符串 "%.7f"`:
			stepStr = "0.0111"
			bytes = strBytes["7"]
			note = "ASCII 数字，需注意按字符长度而不是字节数算列宽"
		case `字符串 "%.6f"`:
			stepStr = "0.1113"
			bytes = strBytes["6"]
			note = "最常见的「六位小数」JSON 口径"
		case `字符串 "%.5f"`:
			stepStr = "1.1132"
			bytes = strBytes["5"]
			note = "体积最小，但 1 m 级误差"
		}
		fmt.Fprintf(out, "| %s | %.5f | %.5f | %s | %d | %s |\n",
			m.name, maxErr, sum/float64(len(pts)), stepStr, bytes, note)
	}

	// C3 字符串字节/点
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## C3 字符串编码的实测字节/点（两个分量 + 1 个分隔符，ASCII）")
	fmt.Fprintln(out, "| 格式 | 总字节(14 点) | 字节/点 |")
	fmt.Fprintln(out, "|---|---|---|")
	for _, d := range []string{"5", "6", "7"} {
		fmt.Fprintf(out, "| %%.%sf | %d | %.2f |\n", d, strBytes[d], float64(strBytes[d])/float64(len(pts)))
	}
	fmt.Fprintln(out, "# 注：负经度（-122.4）比正数多 1 字节，所以字节/点是均值口径")

	// C4 float32 误差随纬度变化
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## C4 float32 量化误差随纬度变化（经度 116.481051234，纬度 0/20/40/60/80）")
	fmt.Fprintln(out, "| 纬度 | 东向误差 m | 北向误差 m | 合成 m | 1e-7 步长在该纬度的东向米值 |")
	fmt.Fprintln(out, "|---|---|---|---|---|")
	for _, lat := range []float64{0, 20, 40, 60, 80} {
		p := geo.Point{Lng: 116.481051234, Lat: lat + 0.000123456}
		q := quantizeFloat32(p)
		eastM, _ := geo.MetersPerDeg(lat)
		de := (q.Lng - p.Lng) * eastM
		_, northM := geo.MetersPerDeg(lat)
		dn := (q.Lat - p.Lat) * northM
		fmt.Fprintf(out, "| %.1f | %+.6f | %+.6f | %.5f | %.6f |\n", lat, de, dn, geo.Haversine(p, q), 1e-7*eastM)
	}

	// C5 int32 范围
	fmt.Fprintln(out)
	fmt.Fprintln(out, "## C5 int32 定点 1e-7 的可用范围（换算验证）")
	fmt.Fprintf(out, "- MaxInt32 = %d；MaxInt32/1e7 = %.4f°，覆盖经度 ±180°、纬度 ±90°：%v\n",
		math.MaxInt32, float64(math.MaxInt32)/1e7, float64(math.MaxInt32)/1e7 > 180)
	fmt.Fprintf(out, "- 若用 uint32（0..%d）：可表示 0～%.2f°，需加 180° 偏移才能表示西经\n",
		uint64(math.MaxUint32), float64(math.MaxUint32)/1e7)
	east40, north40 := geo.MetersPerDeg(40.0)
	fmt.Fprintf(out, "- 1e-7 步长下：纬度方向 %.6f m；经度方向在 40°N 为 %.6f m\n", 1e-7*north40, 1e-7*east40)
}
