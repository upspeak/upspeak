package connector

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/core"
)

// RateLimiter provides per-resource rate limiting with in-memory tracking.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[uuid.UUID]*window
}

// window tracks the request count and reset time for a single resource.
type window struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter creates a new in-memory rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		windows: make(map[uuid.UUID]*window),
	}
}

// Allow checks whether an operation is allowed for the given resource.
// Returns true if within limits, false if rate limited.
func (rl *RateLimiter) Allow(resourceID uuid.UUID, limit *core.RateLimit) bool {
	if limit == nil || limit.MaxRequests <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	w, exists := rl.windows[resourceID]

	if !exists || now.After(w.resetAt) {
		// Start a new window.
		rl.windows[resourceID] = &window{
			count:   1,
			resetAt: now.Add(time.Duration(limit.WindowSeconds) * time.Second),
		}
		return true
	}

	if w.count >= limit.MaxRequests {
		return false
	}

	w.count++
	return true
}

// cleanup removes expired windows to prevent memory leaks.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for id, w := range rl.windows {
		if now.After(w.resetAt) {
			delete(rl.windows, id)
		}
	}
}

// StartCleanup starts a background goroutine that cleans up expired windows
// every 5 minutes. It stops when the context is cancelled.
func (rl *RateLimiter) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.cleanup()
			}
		}
	}()
}
