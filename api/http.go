package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/upspeak/upspeak/core"
)

// MaxRequestBodySize is the maximum allowed request body size (1 MB).
// Requests exceeding this limit will receive a 413 Payload Too Large response.
const MaxRequestBodySize = 1 << 20 // 1 MB

// LimitedBody returns an http.MaxBytesReader-wrapped body that enforces
// MaxRequestBodySize. Handlers should call this before decoding JSON to
// prevent denial-of-service via oversized payloads.
func LimitedBody(w http.ResponseWriter, r *http.Request) *http.Request {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
	return r
}

// WriteJSON writes a single-resource success response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	resp := Response{
		Data: data,
		Meta: &Meta{
			RequestID: w.Header().Get("X-Request-ID"),
			Timestamp: time.Now().UTC(),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// WriteList writes a collection success response with pagination metadata.
func WriteList(w http.ResponseWriter, data any, total int, opts core.ListOptions) {
	resp := Response{
		Data: data,
		Meta: &Meta{
			RequestID: w.Header().Get("X-Request-ID"),
			Timestamp: time.Now().UTC(),
			Total:     &total,
			Limit:     &opts.Limit,
			Offset:    &opts.Offset,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// WriteError writes an error response with the given status code.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	resp := Response{
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
		Meta: &Meta{
			RequestID: w.Header().Get("X-Request-ID"),
			Timestamp: time.Now().UTC(),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// WriteErrorWithDetails writes an error response with additional detail data.
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message string, details any) {
	resp := Response{
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: &Meta{
			RequestID: w.Header().Get("X-Request-ID"),
			Timestamp: time.Now().UTC(),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// ParsePagination extracts pagination parameters from query string.
// Returns sensible defaults when parameters are missing or invalid.
func ParsePagination(r *http.Request) core.ListOptions {
	opts := core.DefaultListOptions()

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10000 {
			opts.Offset = n
		}
	}
	if v := r.URL.Query().Get("sort"); v != "" {
		opts.SortBy = v
	}
	if v := r.URL.Query().Get("order"); v != "" && (v == "asc" || v == "desc") {
		opts.Order = v
	}

	return opts
}

// SetETag sets the ETag response header from an entity version number.
func SetETag(w http.ResponseWriter, version int) {
	w.Header().Set("ETag", strconv.Quote(strconv.Itoa(version)))
}

// ParseIfMatch extracts the expected version from the If-Match request header.
// Returns 0 if the header is absent (no version check required).
// Returns -1 if the header is present but malformed.
func ParseIfMatch(r *http.Request) int {
	h := r.Header.Get("If-Match")
	if h == "" {
		return 0
	}
	// Strip surrounding quotes if present.
	if len(h) >= 2 && h[0] == '"' && h[len(h)-1] == '"' {
		h = h[1 : len(h)-1]
	}
	v, err := strconv.Atoi(h)
	if err != nil || v < 1 {
		return -1
	}
	return v
}
