package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// IngestBatch is what an adapter emits from a collect or stream call. Adapters
// speak external IDs only; they never touch internal UUIDs or the archive. The
// ingest pipeline owns identity resolution and persistence.
type IngestBatch struct {
	Threads     []IngestThread     `json:"threads,omitempty"`     // conversations → Threads
	Items       []IngestItem       `json:"items,omitempty"`       // messages/posts → Nodes (+ reply Edges)
	Edges       []IngestEdge       `json:"edges,omitempty"`       // explicit relationships → Edges
	Annotations []IngestAnnotation `json:"annotations,omitempty"` // reactions/likes → Annotations
	Tombstones  []string           `json:"tombstones,omitempty"`  // external IDs to redact/delete
	Cursor      *IngestCursor      `json:"cursor,omitempty"`      // advanced cursor (nil = unchanged)
}

// IngestThread describes a conversation (Discourse topic, Matrix m.thread) that
// the pipeline resolves to a Thread by (SourceID, ExternalThreadID).
type IngestThread struct {
	ExternalThreadID string     `json:"external_thread_id"`
	Subject          string     `json:"subject"`
	Metadata         []Metadata `json:"metadata,omitempty"`
}

// IngestEdge is one explicit relationship between two external entities. The
// pipeline resolves SourceExternalID/TargetExternalID to the destination repo's
// node UUIDs via node provenance and skips the edge when either is absent.
type IngestEdge struct {
	ExternalID       string  `json:"external_id"`
	SourceExternalID string  `json:"source_external_id"`
	TargetExternalID string  `json:"target_external_id"`
	Type             string  `json:"type"`
	Label            string  `json:"label,omitempty"`
	Weight           float64 `json:"weight,omitempty"`
}

// IngestItem is one external message/post projected toward the graph. The
// adapter fills content + external identity; the pipeline assigns IDs, applies
// filters, resolves the thread/parent, and persists.
type IngestItem struct {
	ExternalID       string         `json:"external_id"`
	ThreadExternalID string         `json:"thread_external_id,omitempty"` // empty → attach at Source level
	ParentExternalID string         `json:"parent_external_id,omitempty"` // → reply Edge
	Node             *Node          `json:"node"`                         // content only (no ID/RepoID/ShortID)
	Author           *ExternalActor `json:"author,omitempty"`
}

// IngestAnnotation is a reaction/like/highlight targeting a node.
type IngestAnnotation struct {
	ExternalID       string          `json:"external_id"`
	TargetExternalID string          `json:"target_external_id"`
	Motivation       string          `json:"motivation"`
	Body             json.RawMessage `json:"body,omitempty"`
	Author           *ExternalActor  `json:"author,omitempty"`
}

// ExternalActor is an external author resolved to a User (username@hostname).
type ExternalActor struct {
	ExternalID  string `json:"external_id"`
	Username    string `json:"username"`
	Hostname    string `json:"hostname"`
	DisplayName string `json:"display_name"`
}

// IngestCursor persists an adapter-defined resumption point per Source. The
// Cursor payload is opaque to core (RSS: etag+last-guid; Discourse: high-water
// post id; Matrix: since-token). Distinct from Phase 6b multi-device sync.
type IngestCursor struct {
	SourceID  uuid.UUID       `json:"source_id"`
	Cursor    json.RawMessage `json:"cursor"`
	UpdatedAt time.Time       `json:"updated_at"`
}
