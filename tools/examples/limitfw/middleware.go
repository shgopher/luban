package limitfw

import (
	"net/http"
	"strconv"
)

// Middleware 把限流器接进 HTTP：拒绝时回 429 + Retry-After；
// 限流器自身故障时按 failOpen 决定放行还是 503。
func Middleware(l Limiter, keyFn func(*http.Request) string, failOpen bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d, err := l.Allow(r.Context(), keyFn(r))
			if err != nil {
				if failOpen {
					w.Header().Set("X-RateLimit-Degraded", "1")
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "limiter unavailable", http.StatusServiceUnavailable)
				return
			}
			if !d.Allowed {
				w.Header().Set("Retry-After", RetryAfterSeconds(d.RetryAfter))
				w.Header().Set("X-RateLimit-Remaining", "0")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			next.ServeHTTP(w, r)
		})
	}
}
