package main

import (
	"math"
	"sort"
)

// 自实现 STR（Sort-Tile-Recursive）批量装载 R 树。
// 投影：x = lng * 111320 * cos(35°)，y = lat * 111320（等距圆柱，只用于索引裁剪，
// 精确距离一律用 haversine 复算，所以投影畸变不影响召回）。
const treeLeafCap = 32
const cosRef = 0.8191520442889918 // cos(35°)

func projectX(lng float64) float64 { return lng * 111320.0 * cosRef }
func projectY(lat float64) float64 { return lat * 111320.0 }

type Rect struct {
	MinX, MinY, MaxX, MaxY float64
}

func emptyRect() Rect {
	return Rect{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
}
func (r *Rect) expandPoint(x, y float64) {
	if x < r.MinX {
		r.MinX = x
	}
	if x > r.MaxX {
		r.MaxX = x
	}
	if y < r.MinY {
		r.MinY = y
	}
	if y > r.MaxY {
		r.MaxY = y
	}
}
func (r *Rect) expandRect(o Rect) {
	r.expandPoint(o.MinX, o.MinY)
	r.expandPoint(o.MaxX, o.MaxY)
}
func (r Rect) overlaps(o Rect) bool {
	return r.MinX <= o.MaxX && r.MaxX >= o.MinX && r.MinY <= o.MaxY && r.MaxY >= o.MinY
}

type RNode struct {
	rect     Rect
	leaf     bool
	children []int32
	pts      []int32
}

type RTree struct {
	nodes []RNode
	root  int32
}

func NewSTRTree(pts []Pt) *RTree {
	t := &RTree{}
	ids := make([]int32, len(pts))
	for i := range ids {
		ids[i] = int32(i)
	}
	t.root = t.build(pts, ids)
	return t
}

func (t *RTree) build(pts []Pt, ids []int32) int32 {
	if len(ids) <= treeLeafCap {
		r := emptyRect()
		for _, id := range ids {
			r.expandPoint(projectX(pts[id].Lng), projectY(pts[id].Lat))
		}
		t.nodes = append(t.nodes, RNode{rect: r, leaf: true, pts: append([]int32(nil), ids...)})
		return int32(len(t.nodes) - 1)
	}
	n := len(ids)
	p := int(math.Ceil(float64(n) / treeLeafCap))
	s := int(math.Ceil(math.Sqrt(float64(p))))
	sliceSize := int(math.Ceil(float64(n) / float64(s)))
	// STR 第一步：按 x 排序切竖条
	sorted := append([]int32(nil), ids...)
	sort.Slice(sorted, func(a, b int) bool { return pts[sorted[a]].Lng < pts[sorted[b]].Lng })
	var children []int32
	for start := 0; start < n; start += sliceSize {
		end := start + sliceSize
		if end > n {
			end = n
		}
		sl := sorted[start:end]
		// STR 第二步：竖条内按 y 排序切叶
		sort.Slice(sl, func(a, b int) bool { return pts[sl[a]].Lat < pts[sl[b]].Lat })
		for k := 0; k < len(sl); k += treeLeafCap {
			e := k + treeLeafCap
			if e > len(sl) {
				e = len(sl)
			}
			children = append(children, t.build(pts, sl[k:e]))
		}
	}
	rect := emptyRect()
	for _, c := range children {
		rect.expandRect(t.nodes[c].rect)
	}
	t.nodes = append(t.nodes, RNode{rect: rect, leaf: false, children: children})
	return int32(len(t.nodes) - 1)
}

// Query 返回候选点 id 与被访问的叶子节点数（作为「索引访问单元」指标）。
func (t *RTree) Query(lat, lng, r float64) ([]int32, int) {
	qx, qy := projectX(lng), projectY(lat)
	dx := r * cosRef / math.Cos(lat*deg)
	q := Rect{MinX: qx - dx, MinY: qy - r, MaxX: qx + dx, MaxY: qy + r}
	var out []int32
	leaves := 0
	stack := []int32{t.root}
	for len(stack) > 0 {
		ni := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := &t.nodes[ni]
		if !node.rect.overlaps(q) {
			continue
		}
		if node.leaf {
			leaves++
			out = append(out, node.pts...)
			continue
		}
		stack = append(stack, node.children...)
	}
	return out, leaves
}
