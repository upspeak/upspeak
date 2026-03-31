package core

import "time"

// ListOptions provides offset-based pagination for list operations.
type ListOptions struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	SortBy string `json:"sort_by"` // e.g. "created_at", "updated_at", "short_id"
	Order  string `json:"order"`   // "asc" or "desc"
}

// DefaultListOptions returns sensible defaults for list operations.
func DefaultListOptions() ListOptions {
	return ListOptions{
		Limit:  20,
		Offset: 0,
		SortBy: "created_at",
		Order:  "desc",
	}
}

// EdgeQueryOptions filters edges by direction and type.
type EdgeQueryOptions struct {
	Direction string // "outgoing", "incoming", "both"
	Type      string // edge type filter; empty means all types
	ListOptions
}

// AnnotationQueryOptions filters annotations, optionally by motivation.
type AnnotationQueryOptions struct {
	Motivation string // e.g. "commenting", "highlighting"; empty means all
	ListOptions
}

// NodeListOptions filters nodes in list operations.
type NodeListOptions struct {
	Type string // filter by node type; empty means all types
	ListOptions
}

// EdgeListOptions filters edges in list operations.
type EdgeListOptions struct {
	Source string // filter by source node ref; empty means all
	Target string // filter by target node ref; empty means all
	Type   string // filter by edge type; empty means all
	ListOptions
}

// FilterListOptions filters filters in list operations.
type FilterListOptions struct {
	ListOptions
}

// JobListOptions filters jobs in list operations.
type JobListOptions struct {
	Status string    // filter by job status; empty means all
	Type   string    // filter by job type; empty means all
	RepoID string    // filter by repo ref; empty means all
	ListOptions
}

// SearchOptions provides structured search filters for nodes.
type SearchOptions struct {
	Type          []string          `json:"type"`
	CreatedAfter  *time.Time        `json:"created_after"`
	CreatedBefore *time.Time        `json:"created_before"`
	HasEdgeType   string            `json:"has_edge_type"`
	Metadata      map[string]string `json:"metadata"`
	Limit         int               `json:"limit"`
	Offset        int               `json:"offset"`
}

// GraphOptions configures graph traversal behaviour.
type GraphOptions struct {
	EdgeType  string // filter traversal to this edge type; empty means all
	Direction string // "outgoing", "incoming", "both"
}

// GraphResult holds the result of a graph traversal.
type GraphResult struct {
	Root  *Node  `json:"root"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
