package stackprobe

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

var sink byte

func mw16(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [16]byte
		buf[0] = 1
		next.ServeHTTP(w, r)
		sink = buf[0]
		runtime.KeepAlive(&buf)
	})
}

func mw256(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [256]byte
		buf[0] = 2
		next.ServeHTTP(w, r)
		sink = buf[0]
		runtime.KeepAlive(&buf)
	})
}

func mw512(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [512]byte
		buf[0] = 3
		next.ServeHTTP(w, r)
		sink = buf[0]
		runtime.KeepAlive(&buf)
	})
}

func mw1024(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [1024]byte
		buf[0] = 4
		next.ServeHTTP(w, r)
		sink = buf[0]
		runtime.KeepAlive(&buf)
	})
}

func chain(n int, mw []func(http.Handler) http.Handler) http.Handler {
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for i := 0; i < n; i++ {
		h = mw[i%len(mw)](h)
	}
	return h
}

func BenchmarkChain(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	cases := []struct {
		name string
		mws  []func(http.Handler) http.Handler
	}{
		{"frame16", []func(http.Handler) http.Handler{mw16}},
		{"frame256", []func(http.Handler) http.Handler{mw256}},
		{"frame512", []func(http.Handler) http.Handler{mw512}},
		{"frame1024", []func(http.Handler) http.Handler{mw1024}},
	}
	for _, c := range cases {
		for _, n := range []int{4,5,6,7,8,9,10,11,12,13,14,16,18,20,22,24} {
			b.Run(fmt.Sprintf("%s/layers%d", c.name, n), func(b *testing.B) {
				h := chain(n, c.mws)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					h.ServeHTTP(rec, req)
				}
			})
		}
	}
}
