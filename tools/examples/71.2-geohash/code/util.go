package main

import (
	"math/rand"
	"runtime"
	"time"
)

func nowNanos() int64 { return time.Now().UnixNano() }

func elapsedNanos(t0 int64) int64 { return time.Now().UnixNano() - t0 }

func newRNG(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func goVer() string { return runtime.Version() }

// qf 把索引的 Query(lat,lng,r) 适配成 evalIndex 需要的 Query 闭包。
func qf(f func(lat, lng, r float64) ([]int32, int)) func(q Query) ([]int32, int) {
	return func(q Query) ([]int32, int) { return f(q.Lat, q.Lng, q.R) }
}

func timeSince(t0 int64) time.Duration { return time.Duration(time.Now().UnixNano() - t0) }
