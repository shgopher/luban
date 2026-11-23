// Package geo 实现本节实测所需的坐标转换与距离函数。
// 只依赖标准库：GCJ-02 正向公开近似式、逆向迭代、BD-09、haversine、Web Mercator。
package geo

import "math"

// WGS84 椭球参数（GCJ-02 公开近似式使用的克拉索夫斯基参数）
const (
	ax  = 6378245.0
	ee  = 0.00669342162296594323
	R   = 6371000.0 // haversine 用球半径，全书统一 6371 km
	Rmu = 6378137.0 // EPSG:3857 的球半径
)

// Point 是一个经纬度点，单位度。
type Point struct {
	Lng, Lat float64
}

// OutOfChina 判断是否在中国大陆范围外（境外不加偏）。
func OutOfChina(p Point) bool {
	if p.Lng < 72.004 || p.Lng > 137.8347 {
		return true
	}
	if p.Lat < 0.8293 || p.Lat > 55.8271 {
		return true
	}
	return false
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}

// WGS84ToGCJ02 正向加偏：WGS84 -> GCJ-02（高德/腾讯/国内底图）。
func WGS84ToGCJ02(p Point) Point {
	if OutOfChina(p) {
		return p
	}
	dLat := transformLat(p.Lng-105.0, p.Lat-35.0)
	dLng := transformLng(p.Lng-105.0, p.Lat-35.0)
	radLat := p.Lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((ax * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (ax / sqrtMagic * math.Cos(radLat) * math.Pi)
	return Point{Lng: p.Lng + dLng, Lat: p.Lat + dLat}
}

// GCJ02ToWGS84Iter 逆向迭代求解：GCJ-02 -> WGS84，迭代 n 次。
// 没有闭式解，因为正向式里 dLat/dLng 又是坐标自身的函数。
func GCJ02ToWGS84Iter(g Point, n int) Point {
	w := g
	for i := 0; i < n; i++ {
		forward := WGS84ToGCJ02(w)
		w = Point{
			Lng: w.Lng + (g.Lng - forward.Lng),
			Lat: w.Lat + (g.Lat - forward.Lat),
		}
	}
	return w
}

// GCJ02ToBD09 高德系 -> 百度系。
func GCJ02ToBD09(p Point) Point {
	z := math.Sqrt(p.Lng*p.Lng+p.Lat*p.Lat) + 0.00002*math.Sin(p.Lat*math.Pi*3000.0/180.0)
	theta := math.Atan2(p.Lat, p.Lng) + 0.000003*math.Cos(p.Lng*math.Pi*3000.0/180.0)
	return Point{
		Lng: z*math.Cos(theta) + 0.0065,
		Lat: z*math.Sin(theta) + 0.006,
	}
}

// BD09ToGCJ02 百度系 -> 高德系（同样只有近似闭式解，误差 cm 级）。
func BD09ToGCJ02(p Point) Point {
	x := p.Lng - 0.0065
	y := p.Lat - 0.006
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*math.Pi*3000.0/180.0)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*math.Pi*3000.0/180.0)
	return Point{Lng: z * math.Cos(theta), Lat: z * math.Sin(theta)}
}

// Haversine 球面距离（米），R = 6371 km。
func Haversine(a, b Point) float64 {
	rad := math.Pi / 180.0
	dLat := (b.Lat - a.Lat) * rad
	dLng := (b.Lng - a.Lng) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}

// MetersPerDeg 本地度->米换算：经度方向乘 cos(lat)，纬度方向用子午圈半径近似 110574 m/度。
func MetersPerDeg(lat float64) (east, north float64) {
	latRad := lat * math.Pi / 180.0
	// 子午圈曲率半径 M 在 WGS84 下的近似（米/弧度）→ 米/度
	m := 6335439.0 / math.Pow(1-ee*math.Sin(latRad)*math.Sin(latRad), 1.5)
	n := 6378137.0 / math.Sqrt(1-ee*math.Sin(latRad)*math.Sin(latRad))
	return n * math.Pi / 180.0 * math.Cos(latRad), m * math.Pi / 180.0
}

// WebMercator 把经纬度投影到 EPSG:3857 平面，单位米。
func WebMercator(p Point) (x, y float64) {
	lngRad := p.Lng * math.Pi / 180.0
	latRad := p.Lat * math.Pi / 180.0
	x = Rmu * lngRad
	y = Rmu * math.Log(math.Tan(math.Pi/4+latRad/2))
	return
}

// MercatorPlanarDist 两个经纬度点在 3857 平面上的欧氏距离（米）。
func MercatorPlanarDist(a, b Point) float64 {
	x1, y1 := WebMercator(a)
	x2, y2 := WebMercator(b)
	return math.Hypot(x2-x1, y2-y1)
}
