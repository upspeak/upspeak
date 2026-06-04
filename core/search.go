package core

import "github.com/google/uuid"

// SearchResult represents a single search hit with relevance score and highlights.
type SearchResult struct {
	Node       Node        `json:"node"`
	Score      float64     `json:"score"`
	Highlights []Highlight `json:"highlights,omitempty"`
}

// Highlight represents a highlighted snippet from a search match.
type Highlight struct {
	Field   string `json:"field"`   // "subject" or "body"
	Snippet string `json:"snippet"` // text with <em> markers
}

// BrowseOptions provides filtering for browsing nodes without full-text search.
type BrowseOptions struct {
	Type     string     `json:"type"`
	SourceID *uuid.UUID `json:"source_id"`
	ListOptions
}

// SearchStore handles full-text search indexing and graph traversal.
type SearchStore interface {
	IndexNode(nodeID uuid.UUID, repoID uuid.UUID, subject string, bodyText string) error
	RemoveNodeIndex(nodeID uuid.UUID) error
	SearchNodes(repoID uuid.UUID, query string, opts SearchOptions) ([]SearchResult, int, error)
	BrowseNodes(repoID uuid.UUID, opts BrowseOptions) ([]Node, int, error)
	TraverseGraph(repoID uuid.UUID, startNodeID uuid.UUID, depth int, opts GraphOptions) (*GraphResult, error)
}
