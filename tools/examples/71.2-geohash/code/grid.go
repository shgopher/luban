package main

import "math"

// GridIndex：固定边长（米）的均匀经纬网格，格宽按参考纬度 35° 折算经度步长。
// 查询按「圆的经纬 bbox」枚举格子 → 无邻接歧义，代价是格子数随 bbox 面积增长。
type GridIndex struct {
	CellM   float64
	dLat    float64
	dLon    float64
	nCols   int64
	buckets map[uint64][]int32
}

const refLat = 35.0

func NewGridIndex(pts []Pt, cellM float64) *GridIndex {
	// 把步长取整成「整格铺满 360°/180°」，否则 ±180° 与极区边上会留半格缝隙
	nLatCells := math.Round(180.0 / (cellM / 111320.0))
	nLonCells := math.Round(360.0 / (cellM / (111320.0 * math.Cos(refLat*deg))))
	g := &GridIndex{
		CellM:   cellM,
		dLat:    180.0 / nLatCells,
		dLon:    360.0 / nLonCells,
		nCols:   int64(nLonCells),
		buckets: make(map[uint64][]int32, len(pts)/4+16),
	}
	for i := range pts {
		k := g.cellKey(pts[i].Lat, pts[i].Lng)
		g.buckets[k] = append(g.buckets[k], pts[i].ID)
	}
	return g
}

func (g *GridIndex) rowOf(lat float64) int64 {
	return int64(math.Floor((lat + 90.0) / g.dLat))
}
func (g *GridIndex) colOf(lng float64) int64 {
	return int64(math.Floor((lng + 180.0) / g.dLon))
}
func (g *GridIndex) nCol() int64 { return g.nCols }

func (g *GridIndex) cellKey(lat, lng float64) uint64 {
	return uint64(g.rowOf(lat))<<32 | uint64(uint32(g.colOf(lng)))
}

func (g *GridIndex) Query(lat, lng, r float64) ([]int32, int) {
	dLatQ := r / 111320.0
	dLonQ := r / (111320.0 * math.Cos(lat*deg))
	if math.Cos(lat*deg) < 1e-9 {
		return nil, 0
	}
	aiMin := g.rowOf(lat - dLatQ)
	aiMax := g.rowOf(lat + dLatQ)
	liMin := g.colOf(lng - dLonQ)
	liMax := g.colOf(lng + dLonQ)
	ncol := g.nCol()
	nlat := int64(math.Round(180.0 / g.dLat))
	if aiMin < 0 {
		aiMin = 0
	}
	if aiMax >= nlat {
		aiMax = nlat - 1
	}
	var out []int32
	cells := 0
	for ai := aiMin; ai <= aiMax; ai++ {
		for li := liMin; li <= liMax; li++ {
			li2 := ((li % ncol) + ncol) % ncol
			cells++
			out = append(out, g.buckets[uint64(ai)<<32|uint64(uint32(li2))]...)
		}
	}
	return out, cells
}
