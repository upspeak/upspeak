package core

import (
	"time"

	"github.com/google/uuid"
)

// Annotation is a composite entity representing a note or highlight attached
// to a target node. The Annotation itself has a first-class identity (ID,
// ShortID) separate from the Node and Edge it contains. The embedded Node
// holds the annotation content; the embedded Edge links that content to the
// target node with type="annotation".
type Annotation struct {
	ID         uuid.UUID `json:"id"`
	ShortID    string    `json:"short_id"`
	RepoID     uuid.UUID `json:"repo_id"`
	Node       Node      `json:"node"`
	Edge       Edge      `json:"edge"`
	Motivation string    `json:"motivation"`
	CreatedBy  uuid.UUID `json:"created_by"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
