package httpmw

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig configures the per-IP token-bucket limiter. Two tiers exist:
// a lenient global tier for ordinary traffic and a strict tier for
// unauthenticated or resource-creating routes (auth, room/table/SNG creation),
// which are the expensive and abusable ones.
type RateLimitConfig struct {
	GlobalRPS      float64
	GlobalBurst    int
	StrictRPS      float64
	StrictBurst    int
	StrictPrefixes []string      // path prefixes that use the strict tier
	TrustProxy     bool          // honor X-Forwarded-For (see ClientIP)
	IdleTTL        time.Duration // evict a per-IP bucket idle for this long
}

// ipBucket is one IP's limiter plus the last time it was used (for eviction).
type ipBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter is a two-tier, per-IP token-bucket rate limiter. It bounds its
// own memory by evicting buckets that have been idle past IdleTTL.
type IPRateLimiter struct {
	cfg    RateLimitConfig
	mu     sync.Mutex
	global map[string]*ipBucket
	strict map[string]*ipBucket
	stop   chan struct{}
	now    func() time.Time
}

// NewIPRateLimiter builds a limiter from cfg and starts a background janitor
// that evicts idle per-IP buckets. Call Close to stop the janitor (tests);
// long-running servers may simply let it run for the process lifetime.
func NewIPRateLimiter(cfg RateLimitConfig) *IPRateLimiter {
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 10 * time.Minute
	}
	l := &IPRateLimiter{
		cfg:    cfg,
		global: map[string]*ipBucket{},
		strict: map[string]*ipBucket{},
		stop:   make(chan struct{}),
		now:    time.Now,
	}
	go l.janitor()
	return l
}

// Close stops the background janitor. Safe to call once.
func (l *IPRateLimiter) Close() { close(l.stop) }

// janitor periodically drops per-IP buckets that have been idle past IdleTTL,
// keeping the two maps from growing without bound under many distinct IPs.
func (l *IPRateLimiter) janitor() {
	t := time.NewTicker(l.cfg.IdleTTL)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			l.evictIdle()
		}
	}
}

func (l *IPRateLimiter) evictIdle() {
	cutoff := l.now().Add(-l.cfg.IdleTTL)
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range []map[string]*ipBucket{l.global, l.strict} {
		for ip, b := range m {
			if b.lastSeen.Before(cutoff) {
				delete(m, ip)
			}
		}
	}
}

// bucketFor returns (creating if needed) the limiter for ip in tier m.
func (l *IPRateLimiter) bucketFor(m map[string]*ipBucket, ip string, rps float64, burst int) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := m[ip]
	if !ok {
		b = &ipBucket{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
		m[ip] = b
	}
	b.lastSeen = l.now()
	return b.limiter
}

// isStrict reports whether path falls under a strict-tier prefix.
func (l *IPRateLimiter) isStrict(path string) bool {
	for _, p := range l.cfg.StrictPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// Middleware wraps next with per-IP rate limiting, choosing the strict tier for
// configured prefixes and the global tier otherwise. Over-limit requests get a
// 429 with Retry-After.
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r, l.cfg.TrustProxy)
		var lim *rate.Limiter
		if l.isStrict(r.URL.Path) {
			lim = l.bucketFor(l.strict, ip, l.cfg.StrictRPS, l.cfg.StrictBurst)
		} else {
			lim = l.bucketFor(l.global, ip, l.cfg.GlobalRPS, l.cfg.GlobalBurst)
		}
		if !lim.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
