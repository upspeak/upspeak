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
