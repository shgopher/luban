package main

import (
	"math"
	"math/rand"
	"runtime"
	"sort"
)

const earthR = 6371000.0 // m，与契约「球面 R=6371 km」一致
const deg = math.Pi / 180

type Pt struct {
	ID  int32
	Lat float64
	Lng float64
}

func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	dlat := (lat2 - lat1) * deg
	dlng := (lng2 - lng1) * deg
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1*deg)*math.Cos(lat2*deg)*math.Sin(dlng/2)*math.Sin(dlng/2)
	if a > 1 {
		a = 1
	}
	return 2 * earthR * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// 20 个城市中心（lat, lng）
var cityCenters = [][2]float64{
	{39.9042, 116.4074}, // 北京
	{31.2304, 121.4737}, // 上海
	{23.1291, 113.2644}, // 广州
	{22.5431, 114.0579}, // 深圳
	{30.5728, 104.0668}, // 成都
	{30.2741, 120.1551}, // 杭州
	{30.5928, 114.3055}, // 武汉
	{34.3416, 108.9398}, // 西安
	{32.0603, 118.7969}, // 南京
	{29.5630, 106.5516}, // 重庆
	{39.0842, 117.2009}, // 天津
	{31.2989, 120.5853}, // 苏州
	{28.2282, 112.9388}, // 长沙
	{34.7466, 113.6254}, // 郑州
	{36.0671, 120.3826}, // 青岛
	{41.8057, 123.4315}, // 沈阳
	{38.9140, 121.6147}, // 大连
	{24.4798, 118.0894}, // 厦门
	{24.8801, 102.8329}, // 昆明
	{45.8038, 126.5350}, // 哈尔滨
}

// GenPoints 合成数据：20 个城市中心做 Gaussian 聚集（σ=0.02° ≈ 2.2 km/轴），
// 再加 noise 个在中国 bbox 内均匀分布的噪声点。返回 len=n 的点集。
func GenPoints(n, noise int, seed int64) []Pt {
	rng := rand.New(rand.NewSource(seed))
	pts := make([]Pt, 0, n)
	perCity := (n - noise) / len(cityCenters)
	for i := range cityCenters {
		for j := 0; j < perCity; j++ {
			lat := cityCenters[i][0] + rng.NormFloat64()*0.02
			lng := cityCenters[i][1] + rng.NormFloat64()*0.02
			pts = append(pts, Pt{ID: int32(len(pts)), Lat: lat, Lng: lng})
		}
	}
	for len(pts) < n {
		lat := 18.0 + rng.Float64()*(54.0-18.0)
		lng := 73.0 + rng.Float64()*(135.0-73.0)
		pts = append(pts, Pt{ID: int32(len(pts)), Lat: lat, Lng: lng})
	}
	return pts
}

type Query struct {
	Lat, Lng, R float64 // R 单位米
}

// SampleQueries 以数据集中随机一点为查询中心（模拟「在某个位置附近搜」）。
func SampleQueries(pts []Pt, r float64, n int, rng *rand.Rand) []Query {
	qs := make([]Query, n)
	for i := range qs {
		p := pts[rng.Intn(len(pts))]
		qs[i] = Query{Lat: p.Lat, Lng: p.Lng, R: r}
	}
	return qs
}

// BruteTruth 暴力扫描求精确结果（真值）。
func BruteTruth(pts []Pt, qs []Query) [][]int32 {
	out := make([][]int32, len(qs))
	for qi, q := range qs {
		var res []int32
		for i := range pts {
			if Haversine(q.Lat, q.Lng, pts[i].Lat, pts[i].Lng) <= q.R {
				res = append(res, pts[i].ID)
			}
		}
		out[qi] = res
	}
	return out
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	k := int(math.Ceil(p*float64(len(ys)))) - 1
	if k < 0 {
		k = 0
	}
	if k >= len(ys) {
		k = len(ys) - 1
	}
	return ys[k]
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}

// measureHeap 测「构建这个索引新增的堆内存」：GC 两次后取 HeapAlloc 差值。
func measureHeap(build func()) uint64 {
	runtime.GC()
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	build()
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&m2)
	if m2.HeapAlloc <= m1.HeapAlloc {
		return 0
	}
	return m2.HeapAlloc - m1.HeapAlloc
}

func pow2(n uint) float64 { return math.Ldexp(1, int(n)) }

// cellSize 返回经纬方向格子边长（米）@lat。
func cellSize(lat float64, lonBits, latBits uint) (cw, ch float64) {
	cw = 360.0 / pow2(lonBits) * 111320.0 * math.Cos(lat*deg)
	ch = 180.0 / pow2(latBits) * 111320.0
	return
}
