// 15.12 Go 语义陷阱的 11 个最小复现。
//
// 每个 demo 只做一件事：把「编译通过、vet 通过、但语义不对」的现场摆出来。
// 数字与地址都来自当前进程，跑 `go run .` 即可复现。
package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// ① 包级变量先于 init()，init() 先于 main()。
var packageVar = func() int {
	fmt.Println("启动顺序 ① 包级变量初始化")
	return 0
}()

func init() {
	fmt.Println("启动顺序 ② init()")
}

func main() {
	fmt.Println("启动顺序 ③ main()")
	_ = packageVar

	shadowDemo()
	initDemo()
	embedDemo()
	sliceDemo()
	rangeDemo()
	deferDemo()
	stringDemo()
	typedNilDemo()
	stringerDeadlockDemo()
	layoutDemo()
	zeroCopyDemo()
}

func section(id, title string) {
	fmt.Printf("\n== %s %s ==\n", id, title)
}

// ---------------------------------------------------------------- 1 shadow

type httpClient struct {
	timeout time.Duration
	label   string
}

func newTracingClient() (*httpClient, error) { return &httpClient{time.Second, "tracing"}, nil }
func newDefaultClient() (*httpClient, error) { return &httpClient{0, "default"}, nil }

// 外层 client 会被内层的 := 遮住，函数最终返回 nil。
func shadowBug(tracing bool) *httpClient {
	var client *httpClient
	if tracing {
		client, err := newTracingClient() // := 同时声明了新的 client 和 err
		if err != nil {
			return nil
		}
		fmt.Printf("  内层 client = %q\n", client.label)
	} else {
		client, err := newDefaultClient()
		if err != nil {
			return nil
		}
		fmt.Printf("  内层 client = %q\n", client.label)
	}
	return client
}

// 修法一：先声明 err，内层改用 =。
func shadowFixed(tracing bool) *httpClient {
	var client *httpClient
	var err error
	if tracing {
		client, err = newTracingClient()
	} else {
		client, err = newDefaultClient()
	}
	if err != nil {
		return nil
	}
	return client
}

func shadowDemo() {
	section("1", "shadow：:= 造出新变量，外层永远是 nil")
	fmt.Printf("  有 bug 的返回值 = %v\n", shadowBug(true))
	fmt.Printf("  修好后的返回值 = %q\n", shadowFixed(true).label)
}

// ---------------------------------------------------------------- 2 init

// 复用同一个进程，演示 init 与包级变量的依赖关系。
var initOnce sync.Once

func initDemo() {
	section("2", "init：不是最先执行，也不是显式依赖")
	initOnce.Do(func() {
		fmt.Println("  init 在包级变量之后、main 之前；它不接收参数、不返回错误、不能被调用")
	})
	fmt.Println("  同一个包里的多个 init 按文件名顺序执行；跨包按 import 依赖的拓扑序")
}

// ---------------------------------------------------------------- 3 embed

// InMem 嵌入 sync.Mutex，本意是内部使用，结果 Lock/Unlock 成了公开 API。
type InMem struct {
	sync.Mutex
	m map[string]int
}

func newInMem() *InMem { return &InMem{m: make(map[string]int)} }

func (i *InMem) Get(key string) (int, bool) {
	i.Lock()
	defer i.Unlock()
	v, ok := i.m[key]
	return v, ok
}

func embedDemo() {
	section("3", "嵌入类型：省下的转发代码，变成了公开的 API")
	in := newInMem()
	in.m["k"] = 1

	// 外部调用者拿到的是 *InMem，但方法集里已经晋升了 Lock/Unlock。
	var locker sync.Locker = in
	locker.Lock() // 封装被破坏：外部可以绕过 Get 直接改内部状态
	in.m["k"] = 2
	locker.Unlock()

	v, _ := in.Get("k")
	fmt.Printf("  外部直接 Lock/Unlock 后读到 k = %d\n", v)
	fmt.Println("  嵌入 sync.Mutex 让 *InMem 直接满足 sync.Locker：能力外泄，不是 bug 是语言规则")
}

// ---------------------------------------------------------------- 4 slice

type Foo struct{ v []byte }

func keepFirstTwo(foos []Foo) []Foo { return foos[:2] }

func keepFirstTwoCopy(foos []Foo) []Foo {
	res := make([]Foo, 2)
	copy(res, foos)
	return res
}

func allocKB() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024
}

// 返回的切片与 foos 共用底层数组，998 个 Foo.v 因此无法回收。
func pinKB(keep func([]Foo) []Foo) uint64 {
	foos := make([]Foo, 1000)
	for i := range foos {
		foos[i] = Foo{v: make([]byte, 128*1024)}
	}
	two := keep(foos)
	foos = nil
	kb := allocKB()
	runtime.KeepAlive(two) // 让 two 活到测量之后
	return kb
}

func sliceDemo() {
	section("4", "slice：len/cap、别名、copy、底层数组钉住")

	s1 := []int{1, 2, 3}
	s2 := s1[1:2]
	s3 := append(s2, 10) // cap(s2) 还够，于是写进 s1 的第 3 个槽位
	fmt.Printf("  append 别名：s1=%v s2=%v s3=%v\n", s1, s2, s3)

	src := []int{0, 1, 2}
	var dst []int
	n := copy(dst, src) // dst 的 len 是 0，copy 一个字节都不搬
	fmt.Printf("  copy 到 nil slice：n=%d dst=%v\n", n, dst)

	fmt.Printf("  预分配：make([]int,3,6) 的 len=%d cap=%d\n", len(make([]int, 3, 6)), cap(make([]int, 3, 6)))
	fmt.Printf("  钉住版本 Alloc=%d KB（1000×128KB 全部存活）\n", pinKB(keepFirstTwo))
	fmt.Printf("  复制版本 Alloc=%d KB（只留 2 个元素）\n", pinKB(keepFirstTwoCopy))
}

// ---------------------------------------------------------------- 5 range

type account struct{ balance float32 }

type customer struct {
	ID      string
	Balance float64
}

func rangeDemo() {
	section("5", "range：拿到的是副本，不是元素本身")

	accounts := []account{{100}, {200}, {300}}
	for _, a := range accounts {
		a.balance += 1000 // 改的是副本
	}
	fmt.Printf("  改副本后：%v\n", accounts)

	test := []customer{{"1", 10}, {"2", -10}, {"3", 0}}
	m := make(map[string]*customer, len(test))
	for _, c := range test {
		m[c.ID] = &c // 1.22 起每轮 c 都是新变量
	}
	distinct := map[*customer]bool{}
	for _, p := range m {
		distinct[p] = true
	}
	fmt.Printf("  go.mod >= 1.22：3 个指针互不相同 = %v；go.mod = 1.21 时见 oldloop/\n", len(distinct) == len(test))

	for i := range test {
		test[i].Balance = 0
	}
	fmt.Printf("  按下标写回后：%v\n", test)
}

// ---------------------------------------------------------------- 6 defer

func deferArg() {
	i := 0
	defer fmt.Printf("  defer 参数在声明时求值：i=%d\n", i) // 这里就固定成 0
	i++
}

func deferNamed() (n int) {
	n = 3
	defer func() { n *= 2 }() // 闭包改的是命名返回值
	return n                  // 先写回 n=3，再执行 defer，最终返回 6
}

func deferPlain() int {
	n := 3
	defer func() { n *= 2 }() // 改的是局部变量，返回槽已经定好
	return n                  // 返回 3
}

func deferClosure() int {
	i := 0
	defer func() { fmt.Printf("  闭包读到的是最终值：i+1=%d\n", i+1) }()
	i++
	return i + 10
}

func loopDefer(n int) {
	for i := 0; i < n; i++ {
		f := i
		defer fmt.Printf("  第 %d 个文件在函数返回时才关闭\n", f)
	}
	fmt.Println("  循环结束，还没关闭任何一个")
}

func deferDemo() {
	section("6", "defer：声明时求值、返回前执行、循环里堆积")
	deferArg()
	fmt.Printf("  匿名返回值函数返回 %d\n", deferPlain())
	fmt.Printf("  命名返回值函数返回 %d\n", deferNamed())
	fmt.Printf("  带闭包的函数返回 %d\n", deferClosure())
	loopDefer(3)
}

// ---------------------------------------------------------------- 7 string

func stringDemo() {
	section("7", "string：字节序列，不是字符序列")

	s := "hêllo"
	fmt.Printf("  len=%d bytes=% x\n", len(s), []byte(s))
	for i := 0; i < len(s); i++ {
		fmt.Printf("  下标 %d → %c\n", i, s[i])
	}
	for i, r := range s {
		fmt.Printf("  range %d → %c\n", i, r)
	}

	big := strings.Repeat("uuid-0000-0000-0000-00000000000-", 1<<20) // 32 字节 × 1M = 32MB
	prefix := big[:36]
	big = ""
	fmt.Printf("  截断后 Alloc=%d KB（36 字节钉住 32MB 底层数组）\n", allocKB())
	_ = prefix
	runtime.KeepAlive(prefix)

	big2 := strings.Repeat("uuid-0000-0000-0000-00000000000-", 1<<20)
	cloned := strings.Clone(big2[:36])
	big2 = ""
	fmt.Printf("  strings.Clone 后 Alloc=%d KB\n", allocKB())
	runtime.KeepAlive(cloned)
}

// ---------------------------------------------------------------- 8 typed nil

type MultiError struct{ errs []string }

func (m *MultiError) Add(err error) { m.errs = append(m.errs, err.Error()) }
func (m *MultiError) Error() string { return strings.Join(m.errs, "; ") }

func validateBug(age int, name string) error {
	var m *MultiError // 指针是 nil
	if age < 0 {
		m = &MultiError{}
		m.Add(fmt.Errorf("age is negative"))
	}
	if name == "" {
		if m == nil {
			m = &MultiError{}
		}
		m.Add(fmt.Errorf("name is empty"))
	}
	return m // 装进 error 接口后，接口类型是 *MultiError，值 nil
}

func validateFixed(age int, name string) error {
	var m *MultiError
	if age < 0 {
		m = &MultiError{}
		m.Add(fmt.Errorf("age is negative"))
	}
	if name == "" {
		if m == nil {
			m = &MultiError{}
		}
		m.Add(fmt.Errorf("name is empty"))
	}
	if m == nil {
		return nil // 显式返回无类型 nil
	}
	return m
}

func typedNilDemo() {
	section("8", "接口值不是 nil，只是它装的东西是 nil")
	err := validateBug(10, "a")
	fmt.Printf("  validateBug 返回 err == nil ? %v\n", err == nil)
	fmt.Printf("  err 的动态类型 = %T，动态值 = %v\n", err, err)
	fmt.Printf("  validateFixed 返回 err == nil ? %v\n", validateFixed(10, "a") == nil)
}

// ---------------------------------------------------------------- 9 Stringer

type Customer struct {
	mu  sync.RWMutex
	id  string
	age int
}

func (c *Customer) String() string {
	c.mu.RLock() // 已经持有写锁时进来，这里永久阻塞
	defer c.mu.RUnlock()
	return fmt.Sprintf("id %s, age %d", c.id, c.age)
}

func (c *Customer) UpdateAge(age int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if age < 0 {
		return fmt.Errorf("age should be positive for customer %v", c) // 触发 String()
	}
	c.age = age
	return nil
}

func stringerDeadlockDemo() {
	section("9", "打印即加锁：String() 与锁重入")
	c := &Customer{id: "c-1"}
	done := make(chan struct{})
	go func() {
		_ = c.UpdateAge(-1)
		close(done)
	}()
	select {
	case <-done:
		fmt.Println("  没有死锁")
	case <-time.After(300 * time.Millisecond):
		fmt.Println("  300ms 仍未返回：UpdateAge 持写锁 → fmt 调 String() → 抢读锁，自锁")
	}
}

// ---------------------------------------------------------------- 10 layout

type M struct {
	m int64
	x struct{}
}

type N struct {
	x struct{}
	n int64
}

type Bad struct {
	a bool
	b int64
	c bool
}

type Good struct {
	a bool
	c bool
	b int64
}

func layoutDemo() {
	section("10", "结构体布局：对齐、填充、零大小字段")
	fmt.Printf("  M{int64; struct{}} = %d bytes（零大小字段在尾部，补齐到 16）\n", unsafe.Sizeof(M{}))
	fmt.Printf("  N{struct{}; int64} = %d bytes\n", unsafe.Sizeof(N{}))
	fmt.Printf("  Bad{bool; int64; bool} = %d bytes（两个 7 字节填充）\n", unsafe.Sizeof(Bad{}))
	fmt.Printf("  Good{bool; bool; int64} = %d bytes（填充合并成 6 字节）\n", unsafe.Sizeof(Good{}))
}

// ---------------------------------------------------------------- 11 zero copy

func zeroCopyDemo() {
	section("11", "[]byte ↔ string：共享内存是结论，不是目的")
	b := []byte("hello")
	s := unsafe.String(unsafe.SliceData(b), len(b))
	b[0] = 'H'
	fmt.Printf("  改 []byte 之后 string = %q（共享同一块内存）\n", s)

	origin := "read-only"
	bs := unsafe.Slice(unsafe.StringData(origin), len(origin))
	fmt.Printf("  string → []byte：len=%d cap=%d，没有复制\n", len(bs), cap(bs))
	fmt.Println("  规则：转换结果不能写字符串底层（可能只读段），也不能比原值活得更久")
}
