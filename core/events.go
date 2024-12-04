package core

import (
	"encoding/json"

	"github.com/rs/xid"
)

// EventType represents the type of an event in the graph.
type EventType string

const (
	// Input events
	EventCreateNode EventType = "CreateNode"
	EventUpdateNode EventType = "UpdateNode"
	EventDeleteNode EventType = "DeleteNode"
	EventCreateEdge EventType = "CreateEdge"
	EventUpdateEdge EventType = "UpdateEdge"
	EventDeleteEdge EventType = "DeleteEdge"

	// Node and Edge event types post-processing
	EventNodeCreated EventType = "NodeCreated"
	EventNodeUpdated EventType = "NodeUpdated"
	EventNodeDeleted EventType = "NodeDeleted"
	EventEdgeCreated EventType = "EdgeCreated"
	EventEdgeUpdated EventType = "EdgeUpdated"
	EventEdgeDeleted EventType = "EdgeDeleted"
)

// Event represents an event in the graph, which can be related to either Nodes or Edges.
type Event struct {
	ID      xid.ID          `json:"id"`      // Unique identifier for the event
	Type    EventType       `json:"type"`    // Type of the event
	Payload json.RawMessage `json:"payload"` // Payload of the event, varies based on the event type
}

type EventNodeCreatePayload struct {
	Node *Node `json:"node"` // The created node
}

type EventNodeUpdatePayload struct {
	NodeId      xid.ID `json:"node_id"`      // The id of the Node being updated
	UpdatedNode *Node  `json:"updated_node"` // The new state of the node
}

type EventNodeDeletePayload struct {
	NodeId xid.ID `json:"node_id"` // The deleted node
}

type EventEdgeCreatePayload struct {
	Edge *Edge `json:"edge"` // The created edge
}

type EventEdgeUpdatePayload struct {
	EdgeId      xid.ID `json:"edge_id"`      // The id for the Edge being updated
	UpdatedEdge *Edge  `json:"updated_edge"` // The new state of the edge
}

type EventEdgeDeletePayload struct {
	EdgeId xid.ID `json:"edge_id"` // The id for the deleted Edge
}

func NewEvent(eventType EventType, payload any) (*Event, error) {
	jsonpayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:      xid.New(),
		Type:    eventType,
		Payload: jsonpayload,
	}, nil
}
