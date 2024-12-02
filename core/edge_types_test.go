package core

import (
	"encoding/json"
	"testing"

	"github.com/rs/xid"
)

const (
	KindText  = Kind("text")
	KindImage = Kind("image")
)

func validateEdge(t *testing.T, edge Edge, expectedType string, expectedSource, expectedTarget xid.ID, expectedWeight float64, expectedLabel string) {
	if edge.Type != expectedType {
		t.Errorf("expected edge type to be '%s', got '%s'", expectedType, edge.Type)
	}
	if edge.Source != expectedSource {
		t.Errorf("expected source ID to be '%s', got '%s'", expectedSource, edge.Source)
	}
	if edge.Target != expectedTarget {
		t.Errorf("expected target ID to be '%s', got '%s'", expectedTarget, edge.Target)
	}
	if edge.Weight != expectedWeight {
		t.Errorf("expected weight to be %f, got %f", expectedWeight, edge.Weight)
	}
	if expectedLabel != "" && edge.Label != expectedLabel {
		t.Errorf("expected label to be '%s', got '%s'", expectedLabel, edge.Label)
	}
	if edge.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set, but it was zero")
	}
}

func TestReplyEdge(t *testing.T) {
	// Create two Text Nodes
	metadata1, _ := json.Marshal(map[string]interface{}{"author": "Alice"})
	body1, _ := json.Marshal(map[string]interface{}{"content": "Hello"})
	node1 := NewNode(KindText, "text/plain", metadata1, body1)

	metadata2, _ := json.Marshal(map[string]interface{}{"author": "Bob"})
	body2, _ := json.Marshal(map[string]interface{}{"content": "Hi"})
	node2 := NewNode(KindText, "text/plain", metadata2, body2)

	// Create a ReplyEdge marking the second node as a reply to the first
	edge := ReplyEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Reply", node1.ID, node2.ID, 0.5, "")
}

func TestAnnotationEdge(t *testing.T) {
	// Create a Text Node
	metadata1, _ := json.Marshal(map[string]interface{}{"author": "Alice"})
	body1, _ := json.Marshal(map[string]interface{}{"content": "Hello"})
	node1 := NewNode(KindText, "text/plain", metadata1, body1)

	// Create a URL Node
	metadata2, _ := json.Marshal(map[string]interface{}{"author": "Bob"})
	body2, _ := json.Marshal(map[string]interface{}{"url": "http://example.com"})
	node2 := NewNode("url", "text/plain", metadata2, body2)

	// Create an AnnotationEdge marking the second node as an annotation of the first
	edge := AnnotationEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Annotation", node1.ID, node2.ID, 0.25, "")
}

func TestForkEdge(t *testing.T) {
	// Create a Text Node
	metadata1, _ := json.Marshal(map[string]interface{}{"author": "Alice"})
	body1, _ := json.Marshal(map[string]interface{}{"content": "Hello"})
	node1 := NewNode(KindText, "text/plain", metadata1, body1)

	// Create another Text Node with the same content, but a different ID
	metadata2, _ := json.Marshal(map[string]interface{}{"author": "Alice"})
	body2, _ := json.Marshal(map[string]interface{}{"content": "Hello"})
	node2 := NewNode(KindText, "text/plain", metadata2, body2)

	// Create a ForkEdge marking the second node as a fork of the first
	edge := ForkEdge(node1.ID, node2.ID)

	// Change the label and weight of the new edge
	edge.Label = "First fork"
	edge.Weight = 0.5

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Fork", node1.ID, node2.ID, 0.5, "First fork")
}

func TestAttachmentEdge(t *testing.T) {
	// Create a Text Node
	metadata1, _ := json.Marshal(map[string]interface{}{"author": "Alice"})
	body1, _ := json.Marshal(map[string]interface{}{"content": "Hello"})
	node1 := NewNode(KindText, "text/plain", metadata1, body1)

	// Create an Image Node with a URL field in its Body
	metadata2, _ := json.Marshal(map[string]interface{}{"author": "Bob"})
	body2, _ := json.Marshal(map[string]interface{}{"url": "http://example.com/image.jpg"})
	node2 := NewNode(KindImage, "image/jpeg", metadata2, body2)

	// Create an AttachmentEdge marking the second node as an attachment of the first
	edge := AttachmentEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Attachment", node1.ID, node2.ID, 0.5, "")
}
