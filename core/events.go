package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event represents a domain event published to JetStream.
type Event struct {
	ID        uuid.UUID       `json:"id"`
	Type      EventType       `json:"type"`
	RepoID    uuid.UUID       `json:"repo_id"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// Subject returns the canonical JetStream subject for this event.
// Format: repo.{repo_id}.events.{EventType}
func (e *Event) Subject() string {
	return fmt.Sprintf("repo.%s.events.%s", e.RepoID.String(), e.Type)
}

// NewEvent creates a new Event with a generated UUID v7 and current timestamp.
func NewEvent(eventType EventType, repoID uuid.UUID, payload any) (*Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}
	return &Event{
		ID:        NewID(),
		Type:      eventType,
		RepoID:    repoID,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}

// Node event payloads.

// EventNodeCreatePayload carries the node to create.
type EventNodeCreatePayload struct {
	Node *Node `json:"node"`
}

// EventNodeUpdatePayload carries the full replacement for a node.
type EventNodeUpdatePayload struct {
	NodeID      uuid.UUID `json:"node_id"`
	UpdatedNode *Node     `json:"updated_node"`
}

// EventNodePatchPayload carries a partial update for a node.
type EventNodePatchPayload struct {
	NodeID   uuid.UUID       `json:"node_id"`
	Fields   json.RawMessage `json:"fields"`
	Metadata []Metadata      `json:"metadata,omitempty"`
}

// EventNodeDeletePayload carries the ID of the node to delete.
type EventNodeDeletePayload struct {
	NodeID uuid.UUID `json:"node_id"`
}

// Edge event payloads.

// EventEdgeCreatePayload carries the edge to create.
type EventEdgeCreatePayload struct {
	Edge *Edge `json:"edge"`
}

// EventEdgeUpdatePayload carries the full replacement for an edge.
type EventEdgeUpdatePayload struct {
	EdgeID      uuid.UUID `json:"edge_id"`
	UpdatedEdge *Edge     `json:"updated_edge"`
}

// EventEdgeDeletePayload carries the ID of the edge to delete.
type EventEdgeDeletePayload struct {
	EdgeID uuid.UUID `json:"edge_id"`
}

// Thread event payloads.

// EventThreadCreatePayload carries the thread to create.
type EventThreadCreatePayload struct {
	Thread *Thread `json:"thread"`
}

// EventThreadUpdatePayload carries the full replacement for a thread.
type EventThreadUpdatePayload struct {
	ThreadID      uuid.UUID `json:"thread_id"`
	UpdatedThread *Thread   `json:"updated_thread"`
}

// EventThreadDeletePayload carries the ID of the thread to delete.
type EventThreadDeletePayload struct {
	ThreadID uuid.UUID `json:"thread_id"`
}

// EventThreadNodePayload carries a node addition/removal from a thread.
type EventThreadNodePayload struct {
	ThreadID uuid.UUID `json:"thread_id"`
	NodeID   uuid.UUID `json:"node_id"`
	EdgeType string    `json:"edge_type,omitempty"`
}

// Annotation event payloads.

// EventAnnotationCreatePayload carries the annotation to create.
type EventAnnotationCreatePayload struct {
	Annotation *Annotation `json:"annotation"`
}

// EventAnnotationUpdatePayload carries the full replacement for an annotation.
type EventAnnotationUpdatePayload struct {
	AnnotationID      uuid.UUID   `json:"annotation_id"`
	UpdatedAnnotation *Annotation `json:"updated_annotation"`
}

// EventAnnotationDeletePayload carries the ID of the annotation to delete.
type EventAnnotationDeletePayload struct {
	AnnotationID uuid.UUID `json:"annotation_id"`
}

// Repository event payloads.

// EventRepoPayload carries repository data for repo events.
type EventRepoPayload struct {
	Repository *Repository `json:"repository"`
}

// EventRepoDeletePayload carries the ID of the repository to delete.
type EventRepoDeletePayload struct {
	RepoID uuid.UUID `json:"repo_id"`
}
