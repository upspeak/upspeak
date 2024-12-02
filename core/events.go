package core

import (
	"encoding/json"

	"github.com/rs/xid"
)

// EventType represents the type of an event in the graph.
type EventType string

const (
	// Node and Edge event types.
	NodeCreated EventType = "NodeCreated"
	NodeUpdated EventType = "NodeUpdated"
	NodeDeleted EventType = "NodeDeleted"
	EdgeCreated EventType = "EdgeCreated"
	EdgeUpdated EventType = "EdgeUpdated"
	EdgeDeleted EventType = "EdgeDeleted"
)

// Event represents an event in the graph, which can be related to either Nodes or Edges.
type Event struct {
	ID      xid.ID          `json:"id"`      // Unique identifier for the event
	Type    EventType       `json:"type"`    // Type of the event
	Payload json.RawMessage `json:"payload"` // Payload of the event, varies based on the event type
}

// UnmarshalJSON unmarshals the Event from JSON, choosing the correct payload type based on the Event.Type.
func (e *Event) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID      xid.ID          `json:"id"`
		Type    EventType       `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	e.ID = raw.ID
	e.Type = raw.Type
	e.Payload = raw.Payload

	return nil
}

// MarshalJSON marshals the Event into JSON.
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID      xid.ID          `json:"id"`
		Type    EventType       `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}{
		ID:      e.ID,
		Type:    e.Type,
		Payload: e.Payload,
	})
}

// NodeCreatedPayload represents the payload for a NodeCreated event.
type NodeCreatedPayload struct {
	Node Node `json:"node"` // The created node
}

// NodeUpdatedPayload represents the payload for a NodeUpdated event.
type NodeUpdatedPayload struct {
	OldNode Node `json:"old_node"` // The old state of the node
	NewNode Node `json:"new_node"` // The new state of the node
}

// NodeDeletedPayload represents the payload for a NodeDeleted event.
type NodeDeletedPayload struct {
	Node Node `json:"node"` // The deleted node
}

// EdgeCreatedPayload represents the payload for an EdgeCreated event.
type EdgeCreatedPayload struct {
	Edge Edge `json:"edge"` // The created edge
}

// EdgeUpdatedPayload represents the payload for an EdgeUpdated event.
type EdgeUpdatedPayload struct {
	OldEdge Edge `json:"old_edge"` // The old state of the edge
	NewEdge Edge `json:"new_edge"` // The new state of the edge
}

// EdgeDeletedPayload represents the payload for an EdgeDeleted event.
type EdgeDeletedPayload struct {
	Edge Edge `json:"edge"` // The deleted edge
}

// EventLog holds an append-only log of events.
type EventLog struct {
	Events []Event `json:"events"` // List of events
}

// AddEvent appends an event to the event log.
func (log *EventLog) AddEvent(event Event) {
	log.Events = append(log.Events, event)
}

// FromJSON creates an EventLog by parsing a JSON array of events.
func (log *EventLog) FromJSON(data []byte) error {
	var rawEvents []json.RawMessage
	if err := json.Unmarshal(data, &rawEvents); err != nil {
		return err
	}

	for _, rawEvent := range rawEvents {
		var event Event
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			return err
		}
		log.Events = append(log.Events, event)
	}

	return nil
}

// NewNodeCreatedEvent creates a new NodeCreated event.
func NewNodeCreatedEvent(node Node) (Event, error) {
	payload, err := json.Marshal(NodeCreatedPayload{Node: node})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    NodeCreated,
		Payload: payload,
	}, nil
}

// NewNodeUpdatedEvent creates a new NodeUpdated event.
func NewNodeUpdatedEvent(oldNode, newNode Node) (Event, error) {
	payload, err := json.Marshal(NodeUpdatedPayload{OldNode: oldNode, NewNode: newNode})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    NodeUpdated,
		Payload: payload,
	}, nil
}

// NewNodeDeletedEvent creates a new NodeDeleted event.
func NewNodeDeletedEvent(node Node) (Event, error) {
	payload, err := json.Marshal(NodeDeletedPayload{Node: node})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    NodeDeleted,
		Payload: payload,
	}, nil
}

// NewEdgeCreatedEvent creates a new EdgeCreated event.
func NewEdgeCreatedEvent(edge Edge) (Event, error) {
	payload, err := json.Marshal(EdgeCreatedPayload{Edge: edge})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    EdgeCreated,
		Payload: payload,
	}, nil
}

// NewEdgeUpdatedEvent creates a new EdgeUpdated event.
func NewEdgeUpdatedEvent(oldEdge, newEdge Edge) (Event, error) {
	payload, err := json.Marshal(EdgeUpdatedPayload{OldEdge: oldEdge, NewEdge: newEdge})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    EdgeUpdated,
		Payload: payload,
	}, nil
}

// NewEdgeDeletedEvent creates a new EdgeDeleted event.
func NewEdgeDeletedEvent(edge Edge) (Event, error) {
	payload, err := json.Marshal(EdgeDeletedPayload{Edge: edge})
	if err != nil {
		return Event{}, err
	}
	return Event{
		ID:      xid.New(),
		Type:    EdgeDeleted,
		Payload: payload,
	}, nil
}
