// Package httpmw holds small, composable net/http middlewares used by the
// pokerd edge: per-IP rate limiting, request-body size caps, and a per-IP cap
// on concurrent WebSocket connections. They are deliberately dependency-light
// (stdlib + golang.org/x/time/rate) so the latency hot path stays lean.
package httpmw

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP resolves the caller's IP for rate-limiting keys.
//
// By default it uses the transport-level RemoteAddr host, which cannot be
// spoofed by the client. When trustProxy is true it instead honors the
// left-most address of X-Forwarded-For - the original client per the de-facto
// standard. Trusting X-Forwarded-For is ONLY safe when pokerd sits directly
// behind a reverse proxy/load balancer that overwrites the header; otherwise a
// client can forge it to evade limits. Hence it is off unless explicitly
// enabled (POKERD_TRUST_PROXY=true).
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
