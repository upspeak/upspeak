package core

import (
	"encoding/json"
	"testing"

	"github.com/rs/xid"
)

// TestNewNodeCreatedEvent verifies initialization of a new node created event.
func TestNewNodeCreatedEvent(t *testing.T) {
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeCreatedEvent(node)

	if event.Type != NodeCreated {
		t.Errorf("expected %v, got %v", NodeCreated, event.Type)
	}
	if event.Payload.(NodeCreatedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeCreatedPayload[string, string]).Node)
	}
}

// TestNewNodeUpdatedEvent verifies initialization of a node updated event.
func TestNewNodeUpdatedEvent(t *testing.T) {
	oldNode := Node[string, string]{ID: xid.New()}
	newNode := Node[string, string]{ID: xid.New()}
	event := NewNodeUpdatedEvent(oldNode, newNode)

	if event.Type != NodeUpdated {
		t.Errorf("expected %v, got %v", NodeUpdated, event.Type)
	}
	if event.Payload.(NodeUpdatedPayload[string, string]).OldNode != oldNode {
		t.Errorf("expected %v, got %v", oldNode, event.Payload.(NodeUpdatedPayload[string, string]).OldNode)
	}
	if event.Payload.(NodeUpdatedPayload[string, string]).NewNode != newNode {
		t.Errorf("expected %v, got %v", newNode, event.Payload.(NodeUpdatedPayload[string, string]).NewNode)
	}
}

// TestNewNodeDeletedEvent verifies initialization of a node deleted event.
func TestNewNodeDeletedEvent(t *testing.T) {
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeDeletedEvent(node)

	if event.Type != NodeDeleted {
		t.Errorf("expected %v, got %v", NodeDeleted, event.Type)
	}
	if event.Payload.(NodeDeletedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeDeletedPayload[string, string]).Node)
	}
}

// TestNewEdgeCreatedEvent verifies initialization of a new edge created event.
func TestNewEdgeCreatedEvent(t *testing.T) {
	edge := Edge{ID: xid.New()}
	event := NewEdgeCreatedEvent(edge)

	if event.Type != EdgeCreated {
		t.Errorf("expected %v, got %v", EdgeCreated, event.Type)
	}
	if event.Payload.(EdgeCreatedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeCreatedPayload).Edge)
	}
}

// TestNewEdgeUpdatedEvent verifies initialization of an edge updated event.
func TestNewEdgeUpdatedEvent(t *testing.T) {
	oldEdge := Edge{ID: xid.New()}
	newEdge := Edge{ID: xid.New()}
	event := NewEdgeUpdatedEvent(oldEdge, newEdge)

	if event.Type != EdgeUpdated {
		t.Errorf("expected %v, got %v", EdgeUpdated, event.Type)
	}
	if event.Payload.(EdgeUpdatedPayload).OldEdge != oldEdge {
		t.Errorf("expected %v, got %v", oldEdge, event.Payload.(EdgeUpdatedPayload).OldEdge)
	}
	if event.Payload.(EdgeUpdatedPayload).NewEdge != newEdge {
		t.Errorf("expected %v, got %v", newEdge, event.Payload.(EdgeUpdatedPayload).NewEdge)
	}
}

// TestNewEdgeDeletedEvent verifies initialization of an edge deleted event.
func TestNewEdgeDeletedEvent(t *testing.T) {
	edge := Edge{ID: xid.New()}
	event := NewEdgeDeletedEvent(edge)

	if event.Type != EdgeDeleted {
		t.Errorf("expected %v, got %v", EdgeDeleted, event.Type)
	}
	if event.Payload.(EdgeDeletedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeDeletedPayload).Edge)
	}
}

// TestEventLog_AddEvent verifies adding an event to the event log.
func TestEventLog_AddEvent(t *testing.T) {
	log := &EventLog[string, string]{}
	event := NewNodeCreatedEvent(Node[string, string]{ID: xid.New()})

	log.AddEvent(event)

	if len(log.Events) != 1 {
		t.Errorf("expected %v, got %v", 1, len(log.Events))
	}
	if log.Events[0] != event {
		t.Errorf("expected %v, got %v", event, log.Events[0])
	}
}

// TestEventLog_FromJSON verifies initialization of an event log from JSON data.
func TestEventLog_FromJSON(t *testing.T) {
	log := &EventLog[string, string]{}
	event := NewNodeCreatedEvent(Node[string, string]{ID: xid.New()})
	data, _ := json.Marshal([]Event[string, string]{event})

	err := log.FromJSON(data)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(log.Events) != 1 {
		t.Errorf("expected %v, got %v", 1, len(log.Events))
	}
	if log.Events[0] != event {
		t.Errorf("expected %v, got %v", event, log.Events[0])
	}
}

// TestEventLog_Replay verifies replaying events to reconstruct the graph state.
func TestEventLog_Replay(t *testing.T) {
	log := &EventLog[string, string]{}
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeCreatedEvent(node)
	log.AddEvent(event)

	graph := log.Replay()

	if len(graph.Nodes) != 1 {
		t.Errorf("expected %v, got %v", 1, len(graph.Nodes))
	}
	if graph.Nodes[node.ID] != node {
		t.Errorf("expected %v, got %v", node, graph.Nodes[node.ID])
	}
}

// TestGraph_AddNode verifies adding a node to the graph and returning the correct event.
func TestGraph_AddNode(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	node := Node[string, string]{ID: xid.New()}

	event, err := graph.AddNode(node)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != NodeCreated {
		t.Errorf("expected %v, got %v", NodeCreated, event.Type)
	}
	if event.Payload.(NodeCreatedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeCreatedPayload[string, string]).Node)
	}
	if graph.Nodes[node.ID] != node {
		t.Errorf("expected %v, got %v", node, graph.Nodes[node.ID])
	}
}

// TestGraph_AddNode_Duplicate verifies that adding a duplicate node returns an error.
func TestGraph_AddNode_Duplicate(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	node := Node[string, string]{ID: xid.New()}
	graph.Nodes[node.ID] = node

	_, err := graph.AddNode(node)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestGraph_UpdateNode verifies updating a node in the graph and returning the correct event.
func TestGraph_UpdateNode(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	oldNode := Node[string, string]{ID: xid.New()}
	newNode := Node[string, string]{ID: oldNode.ID}
	graph.Nodes[oldNode.ID] = oldNode

	event, err := graph.UpdateNode(newNode)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != NodeUpdated {
		t.Errorf("expected %v, got %v", NodeUpdated, event.Type)
	}
	if event.Payload.(NodeUpdatedPayload[string, string]).OldNode != oldNode {
		t.Errorf("expected %v, got %v", oldNode, event.Payload.(NodeUpdatedPayload[string, string]).OldNode)
	}
	if event.Payload.(NodeUpdatedPayload[string, string]).NewNode != newNode {
		t.Errorf("expected %v, got %v", newNode, event.Payload.(NodeUpdatedPayload[string, string]).NewNode)
	}
	if graph.Nodes[newNode.ID] != newNode {
		t.Errorf("expected %v, got %v", newNode, graph.Nodes[newNode.ID])
	}
}

// TestGraph_UpdateNode_NotExist verifies that updating a non-existent node returns an error.
func TestGraph_UpdateNode_NotExist(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	node := Node[string, string]{ID: xid.New()}

	_, err := graph.UpdateNode(node)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestGraph_DeleteNode verifies deleting a node from the graph and returning the correct event.
func TestGraph_DeleteNode(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	node := Node[string, string]{ID: xid.New()}
	graph.Nodes[node.ID] = node

	event, err := graph.DeleteNode(node.ID)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != NodeDeleted {
		t.Errorf("expected %v, got %v", NodeDeleted, event.Type)
	}
	if event.Payload.(NodeDeletedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeDeletedPayload[string, string]).Node)
	}
	if _, exists := graph.Nodes[node.ID]; exists {
		t.Errorf("expected node to be deleted, but it still exists")
	}
}

// TestGraph_DeleteNode_NotExist verifies that deleting a non-existent node returns an error.
func TestGraph_DeleteNode_NotExist(t *testing.T) {
	graph := &Graph[string, string]{Nodes: make(map[xid.ID]Node[string, string])}
	nodeID := xid.New()

	_, err := graph.DeleteNode(nodeID)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestGraph_AddEdge verifies adding an edge to the graph and returning the correct event.
func TestGraph_AddEdge(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	edge := Edge{ID: xid.New()}

	event, err := graph.AddEdge(edge)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != EdgeCreated {
		t.Errorf("expected %v, got %v", EdgeCreated, event.Type)
	}
	if event.Payload.(EdgeCreatedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeCreatedPayload).Edge)
	}
	if graph.Edges[edge.ID] != edge {
		t.Errorf("expected %v, got %v", edge, graph.Edges[edge.ID])
	}
}

// TestGraph_AddEdge_Duplicate verifies that adding a duplicate edge returns an error.
func TestGraph_AddEdge_Duplicate(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	edge := Edge{ID: xid.New()}
	graph.Edges[edge.ID] = edge

	_, err := graph.AddEdge(edge)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestGraph_UpdateEdge verifies updating an edge in the graph and returning the correct event.
func TestGraph_UpdateEdge(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	oldEdge := Edge{ID: xid.New()}
	newEdge := Edge{ID: oldEdge.ID}
	graph.Edges[oldEdge.ID] = oldEdge

	event, err := graph.UpdateEdge(newEdge)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != EdgeUpdated {
		t.Errorf("expected %v, got %v", EdgeUpdated, event.Type)
	}
	if event.Payload.(EdgeUpdatedPayload).OldEdge != oldEdge {
		t.Errorf("expected %v, got %v", oldEdge, event.Payload.(EdgeUpdatedPayload).OldEdge)
	}
	if event.Payload.(EdgeUpdatedPayload).NewEdge != newEdge {
		t.Errorf("expected %v, got %v", newEdge, event.Payload.(EdgeUpdatedPayload).NewEdge)
	}
	if graph.Edges[newEdge.ID] != newEdge {
		t.Errorf("expected %v, got %v", newEdge, graph.Edges[newEdge.ID])
	}
}

// TestGraph_UpdateEdge_NotExist verifies that updating a non-existent edge returns an error.
func TestGraph_UpdateEdge_NotExist(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	edge := Edge{ID: xid.New()}

	_, err := graph.UpdateEdge(edge)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestGraph_DeleteEdge verifies deleting an edge from the graph and returning the correct event.
func TestGraph_DeleteEdge(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	edge := Edge{ID: xid.New()}
	graph.Edges[edge.ID] = edge

	event, err := graph.DeleteEdge(edge.ID)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if event.Type != EdgeDeleted {
		t.Errorf("expected %v, got %v", EdgeDeleted, event.Type)
	}
	if event.Payload.(EdgeDeletedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeDeletedPayload).Edge)
	}
	if _, exists := graph.Edges[edge.ID]; exists {
		t.Errorf("expected edge to be deleted, but it still exists")
	}
}

// TestGraph_DeleteEdge_NotExist verifies that deleting a non-existent edge returns an error.
func TestGraph_DeleteEdge_NotExist(t *testing.T) {
	graph := &Graph[string, string]{Edges: make(map[xid.ID]Edge)}
	edgeID := xid.New()

	_, err := graph.DeleteEdge(edgeID)

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}
