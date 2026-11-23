package main

import (
	"math"
	"strings"
)

const earthR = 6371000.0

func rad(d float64) float64 { return d * math.Pi / 180 }

// haversine 球面距离（米），R=6371 km
func haversine(lng1, lat1, lng2, lat2 float64) float64 {
	dLat := rad(lat2 - lat1)
	dLng := rad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthR * math.Asin(math.Sqrt(a))
}

// 点到线段的垂直距离，局部等距投影（米）
func perpDist(p, a, b Pt) float64 {
	c := math.Cos(rad(a.Lat))
	ax, ay := a.Lng*c*111320, a.Lat*110540
	bx, by := b.Lng*c*111320, b.Lat*110540
	px, py := p.Lng*c*111320, p.Lat*110540
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

const ghBase32 = "0123456789bcdefghjkmnpqrstuvwxyz"

func geohashEncode(lng, lat float64, precision int) string {
	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0
	out := make([]byte, 0, precision)
	bit, ch := 0, 0
	even := true
	for len(out) < precision {
		if even {
			mid := (lngMin + lngMax) / 2
			if lng > mid {
				ch |= 1 << (4 - bit)
				lngMin = mid
			} else {
				lngMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if lat > mid {
				ch |= 1 << (4 - bit)
				latMin = mid
			} else {
				latMax = mid
			}
		}
		even = !even
		if bit < 4 {
			bit++
		} else {
			out = append(out, ghBase32[ch])
			bit, ch = 0, 0
		}
	}
	return string(out)
}

// 返回格子中心与半宽（度）
func geohashDecodeCell(h string) (lng, lat, halfLng, halfLat float64) {
	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0
	even := true
	for i := 0; i < len(h); i++ {
		idx := strings.IndexByte(ghBase32, h[i])
		for b := 4; b >= 0; b-- {
			bit := (idx >> uint(b)) & 1
			if even {
				mid := (lngMin + lngMax) / 2
				if bit == 1 {
					lngMin = mid
				} else {
					lngMax = mid
				}
			} else {
				mid := (latMin + latMax) / 2
				if bit == 1 {
					latMin = mid
				} else {
					latMax = mid
				}
			}
			even = !even
		}
	}
	return (lngMin + lngMax) / 2, (latMin + latMax) / 2, (lngMax - lngMin) / 2, (latMax - latMin) / 2
}

// 9 格（中心 + 8 邻）同精度
func geohashNeighbors9(h string) []string {
	lng, lat, hw, hh := geohashDecodeCell(h)
	out := make([]string, 0, 9)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			out = append(out, geohashEncode(lng+float64(dx)*2*hw, lat+float64(dy)*2*hh, len(h)))
		}
	}
	return out
}

// Pt 一条轨迹点：时间戳 + WGS84 经纬度
type Pt struct {
	TS  int64
	Lng float64
	Lat float64
}
