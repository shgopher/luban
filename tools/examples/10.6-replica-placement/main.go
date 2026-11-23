// 3 副本放置仿真：机架故障 vs 同机架关联故障，比较随机放置与机架感知放置。
// 口径：每个场景 15 个节点、10 万个对象、固定随机种子；机架故障等概率取一个机架；
// 关联故障 = 先随机坏 1 节点，再在同机架内随机坏 1 节点（电源 / ToR / 交换机相关）。
package main

import (
	"fmt"
	"math/rand"
)

type cluster struct {
	name  string
	racks []int // 每个机架的节点数
}

func (c cluster) total() int {
	n := 0
	for _, r := range c.racks {
		n += r
	}
	return n
}

// 节点编号连续：机架 0 占 [0,r0)，机架 1 接着排……
func (c cluster) rackOf(node int) int {
	s := 0
	for i, r := range c.racks {
		s += r
		if node < s {
			return i
		}
	}
	return len(c.racks) - 1
}

type obj struct {
	nodes [3]int
	racks [3]int
}

func newObj(c cluster, nodes [3]int) obj {
	o := obj{nodes: nodes}
	for i, n := range nodes {
		o.racks[i] = c.rackOf(n)
	}
	return o
}

// 随机放置：3 个副本各自独立均匀选节点（允许重复）——典型的“不校验故障域”
func placeRandom(c cluster, rng *rand.Rand) obj {
	var ns [3]int
	for i := range ns {
		ns[i] = rng.Intn(c.total())
	}
	return newObj(c, ns)
}

// 机架感知：先选互不相同的机架，再在机架内随机选节点；机架不足 3 个时退化为“尽量分散”
func placeRackAware(c cluster, rng *rand.Rand) obj {
	var ns [3]int
	if len(c.racks) >= 3 {
		order := rng.Perm(len(c.racks))[:3]
		for i, r := range order {
			start := 0
			for j := 0; j < r; j++ {
				start += c.racks[j]
			}
			ns[i] = start + rng.Intn(c.racks[r])
		}
		return newObj(c, ns)
	}
	// 只有 1~2 个机架：机架数用满，多出来的副本放到副本数少的机架
	used := make([]int, len(c.racks))
	for i := range ns {
		best := -1
		for r := range c.racks {
			if best == -1 || used[r] < used[best] {
				best = r
			}
		}
		start := 0
		for j := 0; j < best; j++ {
			start += c.racks[j]
		}
		ns[i] = start + rng.Intn(c.racks[best])
		used[best]++
	}
	return newObj(c, ns)
}

// 机架 r 整体故障：返回 (丢数据, 仅剩 1 份)
func (o obj) loseRack(c cluster, r int) (lost, degraded bool) {
	alive := 0
	for _, node := range o.nodes {
		if c.rackOf(node) != r {
			alive++
		}
	}
	return alive == 0, alive == 1
}

// 两个节点同时故障：alive 统计未被这两个节点覆盖的副本数
func (o obj) loseNodes(a, b int) (lost, degraded bool) {
	alive := 0
	for _, node := range o.nodes {
		if node != a && node != b {
			alive++
		}
	}
	return alive == 0, alive == 1
}

// 组合数：随机放置时“3 副本全在同一机架”的概率
func sameRackProb(c cluster) float64 {
	n := float64(c.total())
	p := 0.0
	for _, r := range c.racks {
		p += (float64(r) / n) * (float64(r) / n) * (float64(r) / n)
	}
	return p
}

func main() {
	rng := rand.New(rand.NewSource(42))
	const objects = 100000
	clusters := []cluster{
		{"3 机架 × 5 节点", []int{5, 5, 5}},
		{"2 机架（8+7 节点）", []int{8, 7}},
		{"单机架 15 节点", []int{15}},
	}
	placers := []struct {
		name string
		fn   func(cluster, *rand.Rand) obj
	}{
		{"随机", placeRandom},
		{"机架感知", placeRackAware},
	}

	fmt.Println("== 1. 组合数核对（3 机架，随机放置） ==")
	c3 := clusters[0]
	fmt.Printf("P(3 副本同机架) = %.2f%%；机架故障等概率（每次 1/3）→ 理论丢数据率 = %.2f%%\n",
		sameRackProb(c3)*100, sameRackProb(c3)/3*100)
	// 仿真核对
	cnt := 0
	same := 0
	for i := 0; i < objects; i++ {
		o := placeRandom(c3, rng)
		if o.racks[0] == o.racks[1] && o.racks[1] == o.racks[2] {
			same++
		}
	}
	for r := 0; r < 3; r++ {
		for i := 0; i < objects; i++ {
			o := placeRandom(c3, rng)
			if lost, _ := o.loseRack(c3, r); lost {
				cnt++
			}
		}
	}
	fmt.Printf("10 万对象仿真：3 副本同机架 %.2f%%；机架故障丢数据 %.2f%%\n\n",
		float64(same)/objects*100, float64(cnt)/(3*objects)*100)

	fmt.Println("== 2. 机架故障（整机架断电） ==")
	fmt.Println("部署 | 放置 | 丢数据率 | 降级率(仅剩1份) | 健康率(≥2份)")
	for _, c := range clusters {
		for _, p := range placers {
			objs := make([]obj, objects)
			for i := range objs {
				objs[i] = p.fn(c, rng)
			}
			lost, deg := 0, 0
			for r := range c.racks {
				for _, o := range objs {
					l, d := o.loseRack(c, r)
					if l {
						lost++
					} else if d {
						deg++
					}
				}
			}
			total := objects * len(c.racks)
			l, d := float64(lost)/float64(total)*100, float64(deg)/float64(total)*100
			fmt.Printf("%s | %s | %.2f%% | %.2f%% | %.2f%%\n", c.name, p.name, l, d, 100-l-d)
		}
	}

	fmt.Println("\n== 3. 同机架双节点故障（关联故障：电源/ToR/交换机） ==")
	fmt.Println("部署 | 放置 | 丢数据率 | 降级率(仅剩1份)")
	for _, c := range clusters {
		for _, p := range placers {
			objs := make([]obj, objects)
			for i := range objs {
				objs[i] = p.fn(c, rng)
			}
			lost, deg, total := 0, 0, 0
			// 事件分布：先等概率选 1 个节点，再在同机架内等概率选 1 个不同节点
			for r := range c.racks {
				start := 0
				for j := 0; j < r; j++ {
					start += c.racks[j]
				}
				size := c.racks[r]
				for a := start; a < start+size; a++ {
					for b := start; b < start+size; b++ {
						if a == b {
							continue
						}
						total += objects
						for _, o := range objs {
							l, d := o.loseNodes(a, b)
							if l {
								lost++
							} else if d {
								deg++
							}
						}
					}
				}
			}
			l, d := float64(lost)/float64(total)*100, float64(deg)/float64(total)*100
			fmt.Printf("%s | %s | %.2f%% | %.2f%%\n", c.name, p.name, l, d)
		}
	}

	fmt.Println("\n== 4. 年化：机架故障 1 次/年时，3 年累计遇到丢数据的对象比例 ==")
	fmt.Println("部署 | 放置 | 单次故障 | 3 年累计")
	for _, c := range clusters {
		for _, p := range placers {
			objs := make([]obj, objects)
			for i := range objs {
				objs[i] = p.fn(c, rng)
			}
			lost := 0
			for r := range c.racks {
				for _, o := range objs {
					if l, _ := o.loseRack(c, r); l {
						lost++
					}
				}
			}
			pct := float64(lost) / float64(objects*len(c.racks))
			fmt.Printf("%s | %s | %.2f%% | %.2f%%\n", c.name, p.name, pct*100,
				(1-(1-pct)*(1-pct)*(1-pct))*100)
		}
	}
}
