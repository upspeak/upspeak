package api

import "time"

// Response is the standard API response envelope.
// Every API response uses this structure for consistency.
type Response struct {
	Data  any        `json:"data,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
	Meta  *Meta      `json:"meta,omitempty"`
}

// ErrorBody carries error details in the response envelope.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Meta carries request metadata in the response envelope.
type Meta struct {
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`
	Total     *int      `json:"total,omitempty"`
	Limit     *int      `json:"limit,omitempty"`
	Offset    *int      `json:"offset,omitempty"`
}
