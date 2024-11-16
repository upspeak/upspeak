package core

import (
	"encoding/json"
	"fmt"

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
type Event[M, B any] struct {
	ID      xid.ID    `json:"id"`      // Unique identifier for the event
	Type    EventType `json:"type"`    // Type of the event
	Payload any       `json:"payload"` // Payload of the event, varies based on the event type
}

// UnmarshalJSON unmarshals the Event from JSON, choosing the correct payload type based on the Event.Type.
func (e *Event[M, B]) UnmarshalJSON(data []byte) error {
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

	var err error
	switch e.Type {
	case NodeCreated:
		var payload NodeCreatedPayload[M, B]
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	case NodeUpdated:
		var payload NodeUpdatedPayload[M, B]
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	case NodeDeleted:
		var payload NodeDeletedPayload[M, B]
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	case EdgeCreated:
		var payload EdgeCreatedPayload
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	case EdgeUpdated:
		var payload EdgeUpdatedPayload
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	case EdgeDeleted:
		var payload EdgeDeletedPayload
		err = json.Unmarshal(raw.Payload, &payload)
		e.Payload = payload
	default:
		err = json.Unmarshal(raw.Payload, &e.Payload)
	}
	return err
}

// MarshalJSON marshals the Event into JSON, choosing the correct payload type based on the Event.Type.
func (e Event[M, B]) MarshalJSON() ([]byte, error) {
	var payload interface{}
	switch e.Type {
	case NodeCreated:
		payload = e.Payload.(NodeCreatedPayload[M, B])
	case NodeUpdated:
		payload = e.Payload.(NodeUpdatedPayload[M, B])
	case NodeDeleted:
		payload = e.Payload.(NodeDeletedPayload[M, B])
	case EdgeCreated:
		payload = e.Payload.(EdgeCreatedPayload)
	case EdgeUpdated:
		payload = e.Payload.(EdgeUpdatedPayload)
	case EdgeDeleted:
		payload = e.Payload.(EdgeDeletedPayload)
	default:
		payload = e.Payload
	}

	return json.Marshal(struct {
		ID      xid.ID      `json:"id"`
		Type    EventType   `json:"type"`
		Payload interface{} `json:"payload"`
	}{
		ID:      e.ID,
		Type:    e.Type,
		Payload: payload,
	})
}

// NodeCreatedPayload represents the payload for a NodeCreated event.
type NodeCreatedPayload[M, B any] struct {
	Node Node[M, B] `json:"node"` // The created node
}

// NodeUpdatedPayload represents the payload for a NodeUpdated event.
type NodeUpdatedPayload[M, B any] struct {
	OldNode Node[M, B] `json:"old_node"` // The old state of the node
	NewNode Node[M, B] `json:"new_node"` // The new state of the node
}

// NodeDeletedPayload represents the payload for a NodeDeleted event.
type NodeDeletedPayload[M, B any] struct {
	Node Node[M, B] `json:"node"` // The deleted node
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

// Graph represents a graph structure consisting of Nodes and Edges.
type Graph[M, B any] struct {
	ID    xid.ID                `json:"id"`    // Unique identifier for the graph
	Nodes map[xid.ID]Node[M, B] `json:"nodes"` // Map of node IDs to nodes
	Edges map[xid.ID]Edge       `json:"edges"` // Map of edge IDs to edges
}

// AddNode adds a node to the graph and returns a NodeCreated event.
func (g *Graph[M, B]) AddNode(node Node[M, B]) (Event[M, B], error) {
	if _, exists := g.Nodes[node.ID]; exists {
		return Event[M, B]{}, fmt.Errorf("node with ID %s already exists", node.ID)
	}
	g.Nodes[node.ID] = node
	return NewNodeCreatedEvent(node), nil
}

// UpdateNode updates a node in the graph and returns a NodeUpdated event.
func (g *Graph[M, B]) UpdateNode(node Node[M, B]) (Event[M, B], error) {
	oldNode, exists := g.Nodes[node.ID]
	if !exists {
		return Event[M, B]{}, fmt.Errorf("node with ID %s does not exist", node.ID)
	}
	g.Nodes[node.ID] = node
	return NewNodeUpdatedEvent(oldNode, node), nil
}

// DeleteNode deletes a node from the graph and returns a NodeDeleted event.
func (g *Graph[M, B]) DeleteNode(nodeID xid.ID) (Event[M, B], error) {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return Event[M, B]{}, fmt.Errorf("node with ID %s does not exist", nodeID)
	}
	delete(g.Nodes, nodeID)
	return NewNodeDeletedEvent(node), nil
}

// AddEdge adds an edge to the graph and returns an EdgeCreated event.
func (g *Graph[M, B]) AddEdge(edge Edge) (Event[any, any], error) {
	if _, exists := g.Edges[edge.ID]; exists {
		return Event[any, any]{}, fmt.Errorf("edge with ID %s already exists", edge.ID)
	}
	g.Edges[edge.ID] = edge
	return NewEdgeCreatedEvent(edge), nil
}

// UpdateEdge updates an edge in the graph and returns an EdgeUpdated event.
func (g *Graph[M, B]) UpdateEdge(edge Edge) (Event[any, any], error) {
	oldEdge, exists := g.Edges[edge.ID]
	if !exists {
		return Event[any, any]{}, fmt.Errorf("edge with ID %s does not exist", edge.ID)
	}
	g.Edges[edge.ID] = edge
	return NewEdgeUpdatedEvent(oldEdge, edge), nil
}

// DeleteEdge deletes an edge from the graph and returns an EdgeDeleted event.
func (g *Graph[M, B]) DeleteEdge(edgeID xid.ID) (Event[any, any], error) {
	edge, exists := g.Edges[edgeID]
	if !exists {
		return Event[any, any]{}, fmt.Errorf("edge with ID %s does not exist", edgeID)
	}
	delete(g.Edges, edgeID)
	return NewEdgeDeletedEvent(edge), nil
}

// EventLog holds an append-only log of events.
type EventLog[M, B any] struct {
	Events []Event[M, B] `json:"events"` // List of events
}

// AddEvent appends an event to the event log.
func (log *EventLog[M, B]) AddEvent(event Event[M, B]) {
	log.Events = append(log.Events, event)
}

// FromJSON creates an EventLog by parsing a JSON array of events.
func (log *EventLog[M, B]) FromJSON(data []byte) error {
	var rawEvents []json.RawMessage
	if err := json.Unmarshal(data, &rawEvents); err != nil {
		return err
	}

	for _, rawEvent := range rawEvents {
		var event Event[M, B]
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			return err
		}
		log.Events = append(log.Events, event)
	}

	return nil
}

// Replay replays the event log and produces a Graph.
func (log *EventLog[M, B]) Replay() Graph[M, B] {
	graph := Graph[M, B]{
		ID:    xid.New(),
		Nodes: make(map[xid.ID]Node[M, B]),
		Edges: make(map[xid.ID]Edge),
	}

	for _, event := range log.Events {
		switch event.Type {
		case NodeCreated:
			payload := event.Payload.(NodeCreatedPayload[M, B])
			graph.Nodes[payload.Node.ID] = payload.Node
		case NodeUpdated:
			payload := event.Payload.(NodeUpdatedPayload[M, B])
			graph.Nodes[payload.NewNode.ID] = payload.NewNode
		case NodeDeleted:
			payload := event.Payload.(NodeDeletedPayload[M, B])
			delete(graph.Nodes, payload.Node.ID)
		case EdgeCreated:
			payload := event.Payload.(EdgeCreatedPayload)
			graph.Edges[payload.Edge.ID] = payload.Edge
		case EdgeUpdated:
			payload := event.Payload.(EdgeUpdatedPayload)
			graph.Edges[payload.NewEdge.ID] = payload.NewEdge
		case EdgeDeleted:
			payload := event.Payload.(EdgeDeletedPayload)
			delete(graph.Edges, payload.Edge.ID)
		}
	}

	return graph
}

// NewNodeCreatedEvent creates a new NodeCreated event.
func NewNodeCreatedEvent[M, B any](node Node[M, B]) Event[M, B] {
	return Event[M, B]{
		ID:   xid.New(),
		Type: NodeCreated,
		Payload: NodeCreatedPayload[M, B]{
			Node: node,
		},
	}
}

// NewNodeUpdatedEvent creates a new NodeUpdated event.
func NewNodeUpdatedEvent[M, B any](oldNode, newNode Node[M, B]) Event[M, B] {
	return Event[M, B]{
		ID:   xid.New(),
		Type: NodeUpdated,
		Payload: NodeUpdatedPayload[M, B]{
			OldNode: oldNode,
			NewNode: newNode,
		},
	}
}

// NewNodeDeletedEvent creates a new NodeDeleted event.
func NewNodeDeletedEvent[M, B any](node Node[M, B]) Event[M, B] {
	return Event[M, B]{
		ID:   xid.New(),
		Type: NodeDeleted,
		Payload: NodeDeletedPayload[M, B]{
			Node: node,
		},
	}
}

// NewEdgeCreatedEvent creates a new EdgeCreated event.
func NewEdgeCreatedEvent(edge Edge) Event[any, any] {
	return Event[any, any]{
		ID:   xid.New(),
		Type: EdgeCreated,
		Payload: EdgeCreatedPayload{
			Edge: edge,
		},
	}
}

// NewEdgeUpdatedEvent creates a new EdgeUpdated event.
func NewEdgeUpdatedEvent(oldEdge, newEdge Edge) Event[any, any] {
	return Event[any, any]{
		ID:   xid.New(),
		Type: EdgeUpdated,
		Payload: EdgeUpdatedPayload{
			OldEdge: oldEdge,
			NewEdge: newEdge,
		},
	}
}

// NewEdgeDeletedEvent creates a new EdgeDeleted event.
func NewEdgeDeletedEvent(edge Edge) Event[any, any] {
	return Event[any, any]{
		ID:   xid.New(),
		Type: EdgeDeleted,
		Payload: EdgeDeletedPayload{
			Edge: edge,
		},
	}
}
