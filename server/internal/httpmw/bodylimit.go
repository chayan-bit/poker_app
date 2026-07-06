package httpmw

import "net/http"

// BodyLimit returns middleware that caps every request body at maxBytes using
// http.MaxBytesReader. A handler that reads the body then cannot be forced to
// buffer an unbounded payload: reads past the cap fail with a 413-style error
// and the connection is closed. maxBytes <= 0 disables the cap.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maxBytes > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
