package main

import "math"

// GeoHashIndex：把 52 位 GeoHash 的前 5*P 位当桶键（等价于「按前缀分桶」）。
// 用 (latIdx, lonIdx) 直接量化，免去位交错，语义与 geohash 前缀完全一致。
type GeoHashIndex struct {
	P       int
	lonBits uint
	latBits uint
	Wrap    bool // 经度跨 ±180° 是否回绕（生产必须为 true）
	buckets map[uint64][]int32
}

func NewGeoHashIndex(pts []Pt, p int, wrap bool) *GeoHashIndex {
	bits := uint(5 * p)
	lonBits := (bits + 1) / 2 // geohash 从经度位开始
	latBits := bits / 2
	idx := &GeoHashIndex{
		P: p, lonBits: lonBits, latBits: latBits, Wrap: wrap,
		buckets: make(map[uint64][]int32, len(pts)/4+16),
	}
	for i := range pts {
		li, ai := idx.cellIndex(pts[i].Lat, pts[i].Lng)
		k := idx.key(li, ai)
		idx.buckets[k] = append(idx.buckets[k], pts[i].ID)
	}
	return idx
}

func (g *GeoHashIndex) nLon() uint32 { return uint32(pow2(g.lonBits)) }
func (g *GeoHashIndex) nLat() uint32 { return uint32(pow2(g.latBits)) }

func (g *GeoHashIndex) cellIndex(lat, lng float64) (li, ai uint32) {
	li = uint32(math.Floor((lng + 180.0) / 360.0 * pow2(g.lonBits)))
	ai = uint32(math.Floor((lat + 90.0) / 180.0 * pow2(g.latBits)))
	if li >= g.nLon() {
		li = g.nLon() - 1
	}
	if ai >= g.nLat() {
		ai = g.nLat() - 1
	}
	return
}

func (g *GeoHashIndex) key(li, ai uint32) uint64 { return uint64(ai)<<32 | uint64(li) }

// CellSizeAt 返回该层级在纬度 lat 处的格子边长（米，经向 × 纬向）。
func (g *GeoHashIndex) CellSizeAt(lat float64) (cw, ch float64) {
	return cellSize(lat, g.lonBits, g.latBits)
}

// RingFor 返回覆盖半径 r 所需环数 k（格子数 = (2k+1)^2）。
func (g *GeoHashIndex) RingFor(lat, r float64) int {
	cw, ch := g.CellSizeAt(lat)
	k := int(math.Ceil(r / math.Min(cw, ch)))
	if k < 1 {
		k = 1
	}
	return k
}

func (g *GeoHashIndex) Query(lat, lng, r float64) ([]int32, int) {
	return g.queryRing(lat, lng, g.RingFor(lat, r))
}

// QueryFixedCells 强制只查 (2k+1)^2 个格子，用来实测「9 格/25 格够不够」。
func (g *GeoHashIndex) QueryFixedCells(lat, lng float64, k int) ([]int32, int) {
	return g.queryRing(lat, lng, k)
}

func (g *GeoHashIndex) queryRing(lat, lng float64, k int) ([]int32, int) {
	if k > 1000 { // 极区：覆盖同等半径要百万格，视为该结构不可用
		return nil, (2*k + 1) * (2*k + 1)
	}
	li0, ai0 := g.cellIndex(lat, lng)
	nlon, nlat := int(g.nLon()), int(g.nLat())
	var out []int32
	cells := 0
	for di := -k; di <= k; di++ {
		ai := int(ai0) + di
		if ai < 0 || ai >= nlat {
			continue
		}
		for dj := -k; dj <= k; dj++ {
			li := int(li0) + dj
			if g.Wrap {
				li = ((li % nlon) + nlon) % nlon
			} else if li < 0 || li >= nlon {
				continue
			}
			cells++
			out = append(out, g.buckets[g.key(uint32(li), uint32(ai))]...)
		}
	}
	return out, cells
}
