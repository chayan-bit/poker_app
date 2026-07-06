package httpmw

import (
	"net/http"
	"sync"
)

// WSConnLimiter caps the number of concurrent WebSocket connections a single IP
// may hold open at once, bounding per-IP goroutine growth (each live socket
// pins a reader + writer + pinger). It is applied only to the /ws route, where
// the wrapped handler blocks for the whole connection lifetime, so the deferred
// release fires exactly when the socket closes.
type WSConnLimiter struct {
	max        int
	trustProxy bool
	mu         sync.Mutex
	counts     map[string]int
}

// NewWSConnLimiter builds a limiter allowing at most max concurrent connections
// per IP. max <= 0 disables the cap.
func NewWSConnLimiter(max int, trustProxy bool) *WSConnLimiter {
	return &WSConnLimiter{max: max, trustProxy: trustProxy, counts: map[string]int{}}
}

// acquire reserves a slot for ip, returning false when the cap is reached.
func (l *WSConnLimiter) acquire(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[ip] >= l.max {
		return false
	}
	l.counts[ip]++
	return true
}

// release frees a previously acquired slot for ip.
func (l *WSConnLimiter) release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := l.counts[ip]; n > 1 {
		l.counts[ip] = n - 1
	} else {
		delete(l.counts, ip)
	}
}

// Middleware wraps next (the WS handler) with the per-IP concurrent-connection
// cap. Over-cap connect attempts are rejected with a 429 before the upgrade.
func (l *WSConnLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.max <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ip := ClientIP(r, l.trustProxy)
		if !l.acquire(ip) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "too many concurrent connections", http.StatusTooManyRequests)
			return
		}
		defer l.release(ip)
		next.ServeHTTP(w, r)
	})
}
