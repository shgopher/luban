package main

import (
	"testing"
	"unsafe"
)

// ---- slice 预分配：同一件事，只差一个 cap

const appendN = 200_000

func BenchmarkAppendNoCap(b *testing.B) {
	for b.Loop() {
		var s []int
		for i := 0; i < appendN; i++ {
			s = append(s, i)
		}
		_ = s
	}
}

func BenchmarkAppendWithCap(b *testing.B) {
	for b.Loop() {
		s := make([]int, 0, appendN)
		for i := 0; i < appendN; i++ {
			s = append(s, i)
		}
		_ = s
	}
}

// ---- string ↔ []byte：分配次数是唯一的区别

var (
	strSink  string
	byteSink []byte
)

const payload = "the quick brown fox jumps over the lazy dog"

func BenchmarkStringToBytes(b *testing.B) {
	s := payload
	for b.Loop() {
		byteSink = []byte(s)
	}
	_ = byteSink
}

func BenchmarkStringToBytesUnsafe(b *testing.B) {
	s := payload
	for b.Loop() {
		byteSink = unsafe.Slice(unsafe.StringData(s), len(s))
	}
	_ = byteSink
}

func BenchmarkBytesToString(b *testing.B) {
	bs := []byte(payload)
	for b.Loop() {
		strSink = string(bs)
	}
	_ = strSink
}

func BenchmarkBytesToStringUnsafe(b *testing.B) {
	bs := []byte(payload)
	for b.Loop() {
		strSink = unsafe.String(unsafe.SliceData(bs), len(bs))
	}
	_ = strSink
}
