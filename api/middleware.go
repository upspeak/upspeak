package api

import (
	"net/http"

	"github.com/google/uuid"
)

// RequestID is middleware that adds a unique request ID to each request.
// The ID is set as the X-Request-ID header on both the request and response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.Must(uuid.NewV7()).String()
		w.Header().Set("X-Request-ID", id)
		r.Header.Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders is middleware that sets standard HTTP security headers on
// every response. These mitigate common web vulnerabilities such as MIME
// sniffing, click-jacking, and cross-site scripting via embedding.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
