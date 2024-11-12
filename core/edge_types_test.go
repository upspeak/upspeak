package core

import (
	"testing"

	"github.com/rs/xid"
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
	node1 := NewNode("text", map[string]interface{}{"author": "Alice"}, map[string]interface{}{"content": "Hello"})
	node2 := NewNode("text", map[string]interface{}{"author": "Bob"}, map[string]interface{}{"content": "Hi"})

	// Create a ReplyEdge marking the second node as a reply to the first
	edge := ReplyEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Reply", node1.ID, node2.ID, 0.5, "")
}

func TestAnnotationEdge(t *testing.T) {
	// Create a Text Node
	node1 := NewNode("text", map[string]interface{}{"author": "Alice"}, map[string]interface{}{"content": "Hello"})

	// Create a URL Node
	node2 := NewNode("url", map[string]interface{}{"author": "Bob"}, map[string]interface{}{"url": "http://example.com"})

	// Create an AnnotationEdge marking the second node as an annotation of the first
	edge := AnnotationEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Annotation", node1.ID, node2.ID, 0.25, "")
}

func TestForkEdge(t *testing.T) {
	// Create a Text Node
	node1 := NewNode("text", map[string]interface{}{"author": "Alice"}, map[string]interface{}{"content": "Hello"})

	// Create another Text Node with the same content, but a different ID
	node2 := NewNode("text", map[string]interface{}{"author": "Alice"}, map[string]interface{}{"content": "Hello"})

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
	node1 := NewNode("text", map[string]interface{}{"author": "Alice"}, map[string]interface{}{"content": "Hello"})

	// Create an Image Node with a URL field in its Body
	node2 := NewNode("image", map[string]interface{}{"author": "Bob"}, map[string]interface{}{"url": "http://example.com/image.jpg"})

	// Create an AttachmentEdge marking the second node as an attachment of the first
	edge := AttachmentEdge(node1.ID, node2.ID)

	// Ensure the resulting Edge is valid and matches expectations
	validateEdge(t, edge, "Attachment", node1.ID, node2.ID, 0.3, "")
}
