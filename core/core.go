package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Metadata represents a key-value pair with a JSON-encoded value.
type Metadata struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// Node is an entity that represents a basic unit of information in a knowledge graph.
type Node struct {
	ID          uuid.UUID       `json:"id"`
	ShortID     string          `json:"short_id"`
	RepoID      uuid.UUID       `json:"repo_id"`
	Type        string          `json:"type"`
	Subject     string          `json:"subject"`
	ContentType string          `json:"content_type"`
	Body        json.RawMessage `json:"body"`
	Metadata    []Metadata      `json:"metadata"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Edge represents a relationship between two nodes in a knowledge graph.
type Edge struct {
	ID        uuid.UUID `json:"id"`
	ShortID   string    `json:"short_id"`
	RepoID    uuid.UUID `json:"repo_id"`
	Type      string    `json:"type"`
	Source    uuid.UUID `json:"source"`
	Target    uuid.UUID `json:"target"`
	Label     string    `json:"label"`
	Weight    float64   `json:"weight"`
	CreatedBy uuid.UUID `json:"created_by"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents an identity in the system. Users are global — they are not
// scoped to a repository.
type User struct {
	ID          uuid.UUID `json:"id"`
	ShortID     string    `json:"short_id"`
	Username    string    `json:"username"`
	Hostname    string    `json:"hostname"`
	DisplayName string    `json:"display_name"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
