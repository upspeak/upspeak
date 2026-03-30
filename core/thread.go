package core

import (
	"time"

	"github.com/google/uuid"
)

// Thread is a composite entity representing an ordered collection of nodes
// linked by edges. The Thread itself has a first-class identity (ID, ShortID)
// distinct from its root Node. The root Node is the initial content of the
// thread; additional nodes are attached via AddNodeToThread/RemoveNodeFromThread.
type Thread struct {
	ID        uuid.UUID  `json:"id"`
	ShortID   string     `json:"short_id"`
	RepoID    uuid.UUID  `json:"repo_id"`
	Node      Node       `json:"node"`
	Edges     []Edge     `json:"edges"`
	Metadata  []Metadata `json:"metadata"`
	CreatedBy uuid.UUID  `json:"created_by"`
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
