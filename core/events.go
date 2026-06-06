package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaxEventHops is the maximum number of reaction hops an event may carry before
// consumers drop it. It bounds both rule-engine action cascades and repo→repo
// ingest chains: any consumer that emits a downstream event increments Hops by
// one, and any consumer that receives an event with Hops >= MaxEventHops must
// discard it without processing.
const MaxEventHops = 5

// Event represents a domain event published to JetStream.
type Event struct {
	ID        uuid.UUID       `json:"id"`
	Type      EventType       `json:"type"`
	RepoID    uuid.UUID       `json:"repo_id"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	// Hops counts how many reaction steps produced this event. The originating
	// (user-initiated) event has Hops 0; an event emitted as a consequence of a
	// rule firing or repo→repo ingest carries Hops+1. Consumers drop events at
	// MaxEventHops, bounding action cascades.
	Hops int `json:"hops,omitempty"`
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

// Filter event payloads.

// EventFilterPayload carries filter data for filter events. Like the other
// entity payloads it nests the entity under a stable key ("filter") so event
// consumers (the rules engine's dot-path filters, the realtime fan-out) read a
// consistent shape across create, update, and delete.
type EventFilterPayload struct {
	Filter *Filter `json:"filter"`
}

// Source and sink event payloads.

// EventSourcePayload carries source data for source events.
type EventSourcePayload struct {
	Source *Source `json:"source"`
}

// EventSinkPayload carries sink data for sink events.
type EventSinkPayload struct {
	Sink *Sink `json:"sink"`
}

// Schedule event payloads.

// EventSchedulePayload carries schedule data for schedule events.
type EventSchedulePayload struct {
	Schedule *Schedule `json:"schedule"`
}

// Rule event payloads.

// EventRulePayload carries rule data for rule events.
type EventRulePayload struct {
	Rule *Rule `json:"rule"`
}
