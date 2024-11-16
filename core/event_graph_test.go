package core

import (
	"encoding/json"
	"testing"

	"github.com/rs/xid"
)

// TestNewNodeCreatedEvent verifies that a new node created event is correctly initialized.
func TestNewNodeCreatedEvent(t *testing.T) {
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeCreatedEvent(node)

	// Check if the event type is correctly set to NodeCreated.
	if event.Type != NodeCreated {
		t.Errorf("expected %v, got %v", NodeCreated, event.Type)
	}
	// Check if the payload contains the correct node.
	if event.Payload.(NodeCreatedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeCreatedPayload[string, string]).Node)
	}
}

// TestNewNodeUpdatedEvent verifies that a node updated event is correctly initialized.
func TestNewNodeUpdatedEvent(t *testing.T) {
	oldNode := Node[string, string]{ID: xid.New()}
	newNode := Node[string, string]{ID: xid.New()}
	event := NewNodeUpdatedEvent(oldNode, newNode)

	// Check if the event type is correctly set to NodeUpdated.
	if event.Type != NodeUpdated {
		t.Errorf("expected %v, got %v", NodeUpdated, event.Type)
	}
	// Check if the payload contains the correct old node.
	if event.Payload.(NodeUpdatedPayload[string, string]).OldNode != oldNode {
		t.Errorf("expected %v, got %v", oldNode, event.Payload.(NodeUpdatedPayload[string, string]).OldNode)
	}
	// Check if the payload contains the correct new node.
	if event.Payload.(NodeUpdatedPayload[string, string]).NewNode != newNode {
		t.Errorf("expected %v, got %v", newNode, event.Payload.(NodeUpdatedPayload[string, string]).NewNode)
	}
}

// TestNewNodeDeletedEvent verifies that a node deleted event is correctly initialized.
func TestNewNodeDeletedEvent(t *testing.T) {
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeDeletedEvent(node)

	// Check if the event type is correctly set to NodeDeleted.
	if event.Type != NodeDeleted {
		t.Errorf("expected %v, got %v", NodeDeleted, event.Type)
	}
	// Check if the payload contains the correct node.
	if event.Payload.(NodeDeletedPayload[string, string]).Node != node {
		t.Errorf("expected %v, got %v", node, event.Payload.(NodeDeletedPayload[string, string]).Node)
	}
}

// TestNewEdgeCreatedEvent verifies that a new edge created event is correctly initialized.
func TestNewEdgeCreatedEvent(t *testing.T) {
	edge := Edge{ID: xid.New()}
	event := NewEdgeCreatedEvent(edge)

	// Check if the event type is correctly set to EdgeCreated.
	if event.Type != EdgeCreated {
		t.Errorf("expected %v, got %v", EdgeCreated, event.Type)
	}
	// Check if the payload contains the correct edge.
	if event.Payload.(EdgeCreatedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeCreatedPayload).Edge)
	}
}

// TestNewEdgeUpdatedEvent verifies that an edge updated event is correctly initialized.
func TestNewEdgeUpdatedEvent(t *testing.T) {
	oldEdge := Edge{ID: xid.New()}
	newEdge := Edge{ID: xid.New()}
	event := NewEdgeUpdatedEvent(oldEdge, newEdge)

	// Check if the event type is correctly set to EdgeUpdated.
	if event.Type != EdgeUpdated {
		t.Errorf("expected %v, got %v", EdgeUpdated, event.Type)
	}
	// Check if the payload contains the correct old edge.
	if event.Payload.(EdgeUpdatedPayload).OldEdge != oldEdge {
		t.Errorf("expected %v, got %v", oldEdge, event.Payload.(EdgeUpdatedPayload).OldEdge)
	}
	// Check if the payload contains the correct new edge.
	if event.Payload.(EdgeUpdatedPayload).NewEdge != newEdge {
		t.Errorf("expected %v, got %v", newEdge, event.Payload.(EdgeUpdatedPayload).NewEdge)
	}
}

// TestNewEdgeDeletedEvent verifies that an edge deleted event is correctly initialized.
func TestNewEdgeDeletedEvent(t *testing.T) {
	edge := Edge{ID: xid.New()}
	event := NewEdgeDeletedEvent(edge)

	// Check if the event type is correctly set to EdgeDeleted.
	if event.Type != EdgeDeleted {
		t.Errorf("expected %v, got %v", EdgeDeleted, event.Type)
	}
	// Check if the payload contains the correct edge.
	if event.Payload.(EdgeDeletedPayload).Edge != edge {
		t.Errorf("expected %v, got %v", edge, event.Payload.(EdgeDeletedPayload).Edge)
	}
}

// TestEventLog_AddEvent verifies that an event can be added to the event log.
func TestEventLog_AddEvent(t *testing.T) {
	log := &EventLog[string, string]{}
	event := NewNodeCreatedEvent(Node[string, string]{ID: xid.New()})

	log.AddEvent(event)

	// Check if the event log contains exactly one event.
	if len(log.Events) != 1 {
		t.Errorf("expected %v, got %v", 1, len(log.Events))
	}
	// Check if the event log contains the correct event.
	if log.Events[0] != event {
		t.Errorf("expected %v, got %v", event, log.Events[0])
	}
}

// TestEventLog_FromJSON verifies that an event log can be correctly initialized from JSON data.
func TestEventLog_FromJSON(t *testing.T) {
	log := &EventLog[string, string]{}
	event := NewNodeCreatedEvent(Node[string, string]{ID: xid.New()})
	data, _ := json.Marshal([]Event[string, string]{event})

	err := log.FromJSON(data)

	// Check if there was no error during JSON unmarshalling.
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	// Check if the event log contains exactly one event.
	if len(log.Events) != 1 {
		t.Errorf("expected %v, got %v", 1, len(log.Events))
	}
	// Check if the event log contains the correct event.
	if log.Events[0] != event {
		t.Errorf("expected %v, got %v", event, log.Events[0])
	}
}

// TestEventLog_Replay verifies that the event log can correctly replay events to reconstruct the graph state.
func TestEventLog_Replay(t *testing.T) {
	log := &EventLog[string, string]{}
	node := Node[string, string]{ID: xid.New()}
	event := NewNodeCreatedEvent(node)
	log.AddEvent(event)

	graph := log.Replay()

	// Check if the graph contains exactly one node.
	if len(graph.Nodes) != 1 {
		t.Errorf("expected %v, got %v", 1, len(graph.Nodes))
	}
	// Check if the graph contains the correct node.
	if graph.Nodes[node.ID] != node {
		t.Errorf("expected %v, got %v", node, graph.Nodes[node.ID])
	}
}
