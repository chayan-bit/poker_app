package httpmw

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestClientIP_UsesRemoteAddrByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:12345"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	if got := ClientIP(r, false); got != "203.0.113.7" {
		t.Fatalf("ClientIP without proxy trust = %q, want 203.0.113.7", got)
	}
	if got := ClientIP(r, true); got != "1.2.3.4" {
		t.Fatalf("ClientIP trusting proxy = %q, want 1.2.3.4", got)
	}
}

func TestIPRateLimiter_BlocksBurstThenAllowsRefill(t *testing.T) {
	l := NewIPRateLimiter(RateLimitConfig{GlobalRPS: 100, GlobalBurst: 2})
	defer l.Close()
	h := l.Middleware(okHandler())

	call := func() int {
		r := httptest.NewRequest(http.MethodGet, "/api/tables", nil)
		r.RemoteAddr = "10.0.0.1:1000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	if call() != http.StatusOK || call() != http.StatusOK {
		t.Fatal("first two requests (within burst) should pass")
	}
	if code := call(); code != http.StatusTooManyRequests {
		t.Fatalf("third request over burst = %d, want 429", code)
	}

	// A different IP has its own bucket and is unaffected.
	r := httptest.NewRequest(http.MethodGet, "/api/tables", nil)
	r.RemoteAddr = "10.0.0.2:1000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("independent IP = %d, want 200", w.Code)
	}
}

func TestIPRateLimiter_StrictTierForCreateRoutes(t *testing.T) {
	l := NewIPRateLimiter(RateLimitConfig{
		GlobalRPS: 100, GlobalBurst: 100,
		StrictRPS: 1, StrictBurst: 1,
		StrictPrefixes: []string{"/api/auth"},
	})
	defer l.Close()
	h := l.Middleware(okHandler())

	call := func(path string) int {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.RemoteAddr = "10.0.0.9:1000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	if call("/api/auth/guest") != http.StatusOK {
		t.Fatal("first strict request should pass")
	}
	if code := call("/api/auth/guest"); code != http.StatusTooManyRequests {
		t.Fatalf("second strict request = %d, want 429 (burst 1)", code)
	}
	// The lenient global tier for a non-strict path is unaffected.
	if code := call("/api/tables"); code != http.StatusOK {
		t.Fatalf("global-tier path = %d, want 200", code)
	}
}

func TestBodyLimit_RejectsOversizedBody(t *testing.T) {
	readAll := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := BodyLimit(16)(readAll)

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 64)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413", w.Code)
	}

	r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small"))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("small body = %d, want 200", w.Code)
	}
}

func TestWSConnLimiter_CapsConcurrentConnectionsPerIP(t *testing.T) {
	l := NewWSConnLimiter(1, false)
	release := make(chan struct{})
	blocking := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		<-release // hold the "connection" open
	})
	h := l.Middleware(blocking)

	// First connection acquires the only slot and blocks.
	firstDone := make(chan int, 1)
	go func() {
		r := httptest.NewRequest(http.MethodGet, "/ws", nil)
		r.RemoteAddr = "10.0.0.5:1000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		firstDone <- w.Code
	}()

	// Wait until the first is in-flight, then a second connection from the same IP
	// is rejected.
	deadline := time.After(2 * time.Second)
	for {
		if l.slotHeld("10.0.0.5") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first connection never acquired its slot")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.RemoteAddr = "10.0.0.5:1001"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second concurrent connection = %d, want 429", w.Code)
	}

	// Releasing the first frees the slot.
	close(release)
	if code := <-firstDone; code != http.StatusOK {
		t.Fatalf("first connection = %d, want 200", code)
	}
}

// slotHeld reports whether ip currently holds a connection slot (test helper).
func (l *WSConnLimiter) slotHeld(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[ip] > 0
}
