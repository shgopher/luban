package main

import (
	"sort"

	"github.com/golang/geo/s1"
	"github.com/golang/geo/s2"
)

// S2Index：点按 S2 CellId 的某个 level 排序存储；查询用 RegionCoverer
// 对「半径圆盘 Cap」做覆盖，取覆盖格子的 id 区间（RangeMin..RangeMax）二分查找。
type S2Index struct {
	Level    int
	MaxCells int
	ids      []uint64
	pt       []int32
}

func NewS2Index(pts []Pt, level, maxCells int) *S2Index {
	type pair struct {
		id uint64
		pt int32
	}
	ps := make([]pair, len(pts))
	for i := range pts {
		ll := s2.LatLngFromDegrees(pts[i].Lat, pts[i].Lng)
		ps[i] = pair{id: uint64(s2.CellIDFromLatLng(ll).Parent(level)), pt: pts[i].ID}
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].id < ps[b].id })
	idx := &S2Index{Level: level, MaxCells: maxCells, ids: make([]uint64, len(ps)), pt: make([]int32, len(ps))}
	for i := range ps {
		idx.ids[i] = ps[i].id
		idx.pt[i] = ps[i].pt
	}
	return idx
}

func (s *S2Index) Query(lat, lng, r float64) ([]int32, int) {
	center := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, lng))
	angle := s1.Angle(r / earthR)
	cap := s2.CapFromCenterAngle(center, angle)
	coverer := &s2.RegionCoverer{MaxLevel: s.Level, MaxCells: s.MaxCells}
	cu := coverer.Covering(cap)
	var out []int32
	cells := 0
	for _, c := range cu {
		lo := uint64(c.RangeMin())
		hi := uint64(c.RangeMax())
		i := sort.Search(len(s.ids), func(i int) bool { return s.ids[i] >= lo })
		for ; i < len(s.ids) && s.ids[i] <= hi; i++ {
			out = append(out, s.pt[i])
		}
		cells++
	}
	return out, cells
}
