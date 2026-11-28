package main

import (
	"fmt"
	"time"
)

// ---------- 装饰器：不改函数体，改函数值 ----------

// fibNaive 是教科书版：朴素递归，调用次数按黄金比例指数增长。
func fibNaive(n int) int {
	if n < 2 {
		return n
	}
	return fibNaive(n-1) + fibNaive(n-2)
}

// memoize 是"装饰器"的最小形态：接收一个函数，返回一个同签名的新函数。
// 它没有修改 fibNaive 一行代码——这正是装饰器的本质：改的是函数值，不是函数体。
func memoize(f func(int) int) func(int) int {
	cache := map[int]int{}
	var calls int
	return func(n int) int {
		calls++
		if v, ok := cache[n]; ok {
			return v
		}
		v := f(n)
		cache[n] = v
		return v
	}
}

// ---------- 委托：Go 用嵌入做编译期委托 ----------

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }
func (c *Counter) N() int { return c.n }

// LoggingCounter 嵌入 Counter：Inc/N 被"提升"，调用方看到的是同一组方法。
// 它不是继承——没有虚函数表，也没有运行时多态；要改行为只能定义同名方法覆盖。
type LoggingCounter struct {
	Counter
	ops int
}

func (c *LoggingCounter) Inc() { c.ops++; c.Counter.Inc() }

// ---------- 撤销栈：nil 占位是"委托模式"落地时的经典坑 ----------

type UndoStack struct {
	ops []func()
}

func (s *UndoStack) Do(op func()) { s.ops = append(s.ops, op) }

// Undo 把已经撤销的位置置 nil 占位而不是删除，否则后续下标全部错位。
func (s *UndoStack) Undo() bool {
	for i := len(s.ops) - 1; i >= 0; i-- {
		if s.ops[i] == nil {
			continue
		}
		s.ops[i]()
		s.ops[i] = nil // 释放闭包，避免内存泄漏
		return true
	}
	return false
}

func runParadigm() {
	const n = 32
	start := time.Now()
	v := fibNaive(n)
	naive := time.Since(start)

	// 坑：直接 memoize(fibNaive) 没用——fibNaive 内部调用的是它自己，
	// 不是被装饰后的函数，每次递归仍然绕过了 cache。
	wrong := memoize(fibNaive)
	start = time.Now()
	v2 := wrong(n)
	wrongCost := time.Since(start)

	// 正确写法：让递归调用也走被装饰后的函数（自引用），cache 才命中。
	var fib func(int) int
	fib = memoize(func(k int) int {
		if k < 2 {
			return k
		}
		return fib(k-1) + fib(k-2)
	})
	start = time.Now()
	v3 := fib(n)
	rightCost := time.Since(start)

	fmt.Printf("fib(%d) 朴素递归              = %-8d 耗时 %v\n", n, v, naive)
	fmt.Printf("fib(%d) memoize(fibNaive) 套壳 = %-8d 耗时 %v  ← 白套，递归绕过了 cache\n", n, v2, wrongCost)
	fmt.Printf("fib(%d) 自引用 memoize         = %-8d 耗时 %v  ← 线性\n", n, v3, rightCost)
}
