// 12.5 公平调度算法 · DRF 算例
// 集群：12 核 CPU / 24 GiB 内存。3 个用户、2 维资源、无限 backlog。
// A(1,4) 内存型，B(3,1) CPU 型，C(2,2) 均衡型。
// 输出：单任务主导份额 → DRF 分配序列 → 终局份额 → DRF/均分/FIFO 三策略对比。
package main

import "fmt"

type res struct{ cpu, mem float64 }

type user struct {
	name string
	task res // 单个任务形状
	used res // 已分配累计
	n    int // 已服务任务数
}

var capRes = res{12, 24} // 集群容量（12 核 / 24 GiB）

func (r res) plus(o res) res  { return res{r.cpu + o.cpu, r.mem + o.mem} }
func (r res) fits(c res) bool { return r.cpu <= c.cpu && r.mem <= c.mem }

// share：主导份额 = max(cpu/总CPU, mem/总内存)
func (r res) share() float64 {
	s := r.cpu / capRes.cpu
	if m := r.mem / capRes.mem; m > s {
		s = m
	}
	return s
}

func newUsers() []*user {
	return []*user{
		{name: "A", task: res{1, 4}},
		{name: "B", task: res{3, 1}},
		{name: "C", task: res{2, 2}},
	}
}

func usedAll(us []*user) res {
	var t res
	for _, u := range us {
		t = t.plus(u.used)
	}
	return t
}

func served(us []*user) int {
	n := 0
	for _, u := range us {
		n += u.n
	}
	return n
}

func main() {
	us := newUsers()
	fmt.Println("== 形状 → 主导份额 ==")
	for _, u := range us {
		fmt.Printf("  %s 任务 (%g核,%gG)  cpu份额 %.3f  内存份额 %.3f  主导资源=%s  Δd=%.3f\n",
			u.name, u.task.cpu, u.task.mem, u.task.cpu/capRes.cpu, u.task.mem/capRes.mem,
			dominant(u.task.cpu/capRes.cpu >= u.task.mem/capRes.mem), u.task.share())
	}

	fmt.Println("\n== DRF 分配序列（每步选主导份额最小且放得下者）==")
	for step := 1; ; step++ {
		var pick *user
		for _, u := range us {
			if !usedAll(us).plus(u.task).fits(capRes) {
				continue // 放不下，不能成为候选
			}
			if pick == nil || u.used.share() < pick.used.share()-1e-12 {
				pick = u // 主导份额最小者优先
			}
		}
		if pick == nil {
			break // 没有任何人的下一个任务放得下 → max-min 公平点
		}
		before := pick.used.share()
		pick.used = pick.used.plus(pick.task)
		pick.n++
		fmt.Printf("  第 %d 步 选中 %s  d %.3f→%.3f  该用户累计 (%g核,%gG)\n",
			step, pick.name, before, pick.used.share(), pick.used.cpu, pick.used.mem)
	}

	t := usedAll(us)
	fmt.Printf("\n== DRF 终局 ==\n")
	for _, u := range us {
		fmt.Printf("  %s 任务 %d 个  (%g核,%gG)  d=%.3f\n", u.name, u.n, u.used.cpu, u.used.mem, u.used.share())
	}
	fmt.Printf("  合计 (%g核,%gG) 剩余 (%g核,%gG) → 三个用户的下一个任务都因 CPU 不足放不下\n",
		t.cpu, t.mem, capRes.cpu-t.cpu, capRes.mem-t.mem)

	// 均分：每用户固定 1/3 配额（4 核 / 8 GiB），配额内先到先得，不借用
	eq := newUsers()
	for _, u := range eq {
		quota := res{capRes.cpu / 3, capRes.mem / 3}
		for u.used.plus(u.task).fits(quota) {
			u.used = u.used.plus(u.task)
			u.n++
		}
	}

	// FIFO：B 先一次性提交 6 个任务（突刺），然后 A×6、C×6；放不下就跳过、不重排
	fifo := newUsers()
	var order []string
	for i := 0; i < 6; i++ {
		order = append(order, "B")
	}
	for i := 0; i < 6; i++ {
		order = append(order, "A")
	}
	for i := 0; i < 6; i++ {
		order = append(order, "C")
	}
	byName := map[string]*user{}
	for _, u := range fifo {
		byName[u.name] = u
	}
	var got res
	for _, name := range order {
		u := byName[name]
		if got.plus(u.task).fits(capRes) {
			got = got.plus(u.task)
			u.used = u.used.plus(u.task)
			u.n++
		}
	}

	fmt.Println("\n== 三策略对比（同一批 18 个任务）==")
	print := func(name string, us []*user) {
		t := usedAll(us)
		fmt.Printf("  %-4s 服务 %2d 个 | CPU %2g/12 (%3.0f%%) | 内存 %2g/24 (%3.0f%%) | 主导份额 %s/%s/%s\n",
			name, served(us), t.cpu, 100*t.cpu/capRes.cpu, t.mem, 100*t.mem/capRes.mem,
			f2(us[0].used.share()), f2(us[1].used.share()), f2(us[2].used.share()))
	}
	print("DRF", us)
	print("均分", eq)
	print("FIFO", fifo)
}

func dominant(cpuIsDominant bool) string {
	if cpuIsDominant {
		return "CPU"
	}
	return "内存"
}

func f2(v float64) string { return fmt.Sprintf("%.2f", v) }
