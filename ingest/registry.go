// Package ingest holds the adapter registry and the ingest pipeline. The
// pipeline turns adapter-emitted IngestBatches into persisted graph entities by
// reusing the canonical write+event path (Archive + Publisher), so ingested
// data flows through rules, realtime, and search with no extra wiring.
package ingest

import (
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Registry is the concrete app.AdapterRegistry. It is populated once at startup
// (main.go) and read concurrently thereafter; registration is not expected
// after the server starts, so it carries no lock.
type Registry struct {
	adapters map[core.ConnectorType]core.Adapter
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[core.ConnectorType]core.Adapter)}
}

// Register adds (or replaces) the adapter for its connector type.
func (r *Registry) Register(a core.Adapter) {
	r.adapters[a.Type()] = a
}

// AdapterFor returns the adapter for a connector type, and false when absent.
func (r *Registry) AdapterFor(connector core.ConnectorType) (core.Adapter, bool) {
	a, ok := r.adapters[connector]
	return a, ok
}

// Ensure Registry satisfies the app port.
var _ app.AdapterRegistry = (*Registry)(nil)
