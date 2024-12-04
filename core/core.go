package core

import (
	"encoding/json"
	"time"

	"github.com/rs/xid"
)

type Metadata struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// Node is an entity that repesents a basic unit of information in a knowledge graph.
type Node struct {
	ID          xid.ID          `json:"id"`
	Type        string          `json:"type"`
	Subject     string          `json:"subject"`
	ContentType string          `json:"content_type"`
	Body        json.RawMessage `json:"body"`
	Metadata    []Metadata      `json:"metadata"`
	CreatedBy   xid.ID          `json:"created_by"` // ID of the user who created the node
	CreatedAt   time.Time       `json:"created_at"`
}

// Edge represents a relationship between two nodes in a knowledge graph.
// e.g., replies, sub-threads, annotations, etc.
type Edge struct {
	ID        xid.ID    `json:"id"`
	Type      string    `json:"type"`   // Type of relationship (e.g., "reply", "sub-thread")
	Source    xid.ID    `json:"source"` // ID of the source node (parent node)
	Target    xid.ID    `json:"target"` // ID of the target node (child node)
	Label     string    `json:"label"`  // Label for the relationship
	Weight    float64   `json:"weight"` // Weight or importance of the relation
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID          xid.ID `json:"id"`           // Unique local identifier for the user
	Username    string `json:"username"`     // Username of the user
	Hostname    string `json:"hostname"`     // Hostname of the user
	DisplayName string `json:"display_name"` // Display name of the user
	Source      string `json:"source"`       // Platform identifier (e.g., "matrix", "discourse", "mastodon", "github", "bluesky")
}
