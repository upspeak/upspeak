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
	Node *Node `json:"node"` // The created node
}

// NodeUpdatedPayload represents the payload for a NodeUpdated event.
type NodeUpdatedPayload struct {
	NodeId      xid.ID `json:"node_id"`      // The id of the Node being updated
	UpdatedNode *Node  `json:"updated_node"` // The new state of the node
}

// NodeDeletedPayload represents the payload for a NodeDeleted event.
type NodeDeletedPayload struct {
	NodeId xid.ID `json:"node_id"` // The deleted node
}

// EdgeCreatedPayload represents the payload for an EdgeCreated event.
type EdgeCreatedPayload struct {
	Edge *Edge `json:"edge"` // The created edge
}

// EdgeUpdatedPayload represents the payload for an EdgeUpdated event.
type EdgeUpdatedPayload struct {
	EdgeId      xid.ID `json:"edge_id"`      // The id for the Edge being updated
	UpdatedEdge *Edge  `json:"updated_edge"` // The new state of the edge
}

// EdgeDeletedPayload represents the payload for an EdgeDeleted event.
type EdgeDeletedPayload struct {
	EdgeId xid.ID `json:"edge_id"` // The id for the deleted Edge
}

// nodeCreatedEvent creates a new NodeCreated event.
func nodeCreatedEvent(node *Node) (*Event, error) {
	payload, err := json.Marshal(NodeCreatedPayload{Node: node})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    NodeCreated,
		Payload: payload,
	}, nil
}

// nodeUpdatedEvent creates a new NodeUpdated event.
func nodeUpdatedEvent(node *Node) (*Event, error) {
	payload, err := json.Marshal(NodeUpdatedPayload{NodeId: node.ID, UpdatedNode: node})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    NodeUpdated,
		Payload: payload,
	}, nil
}

// nodeDeletedEvent creates a new NodeDeleted event.
func nodeDeletedEvent(nodeId xid.ID) (*Event, error) {
	payload, err := json.Marshal(NodeDeletedPayload{NodeId: nodeId})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    NodeDeleted,
		Payload: payload,
	}, nil
}

// edgeCreatedEvent creates a new EdgeCreated event.
func edgeCreatedEvent(edge *Edge) (*Event, error) {
	payload, err := json.Marshal(EdgeCreatedPayload{Edge: edge})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    EdgeCreated,
		Payload: payload,
	}, nil
}

// edgeUpdatedEvent creates a new EdgeUpdated event.
func edgeUpdatedEvent(edge *Edge) (*Event, error) {
	payload, err := json.Marshal(EdgeUpdatedPayload{EdgeId: edge.ID, UpdatedEdge: edge})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    EdgeUpdated,
		Payload: payload,
	}, nil
}

// edgeDeletedEvent creates a new EdgeDeleted event.
func edgeDeletedEvent(id xid.ID) (*Event, error) {
	payload, err := json.Marshal(EdgeDeletedPayload{EdgeId: id})
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    EdgeDeleted,
		Payload: payload,
	}, nil
}
