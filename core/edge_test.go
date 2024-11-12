package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/xid"
)

// TestEdgeMarshalJSON tests the JSON marshaling of an Edge object.
// It creates an Edge object, marshals it to JSON, and then checks if the JSON
// representation matches the expected values.
func TestEdgeMarshalJSON(t *testing.T) {
	source := xid.New()
	target := xid.New()
	edge := NewEdge("friend", source, target, "knows", 0.75)
	edge.CreatedAt = time.Now()

	data, err := json.Marshal(edge)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result["id"] != edge.ID.String() {
		t.Errorf("expected id %v, got %v", edge.ID.String(), result["id"])
	}
	if result["type"] != "friend" {
		t.Errorf("expected type 'friend', got %v", result["type"])
	}
	if result["source"] != source.String() {
		t.Errorf("expected source %v, got %v", source.String(), result["source"])
	}
	if result["target"] != target.String() {
		t.Errorf("expected target %v, got %v", target.String(), result["target"])
	}
	if result["label"] != "knows" {
		t.Errorf("expected label 'knows', got %v", result["label"])
	}
	if result["weight"] != edge.Weight {
		t.Errorf("expected weight %v, got %v", edge.Weight, result["weight"])
	}
	if result["created_at"] != edge.CreatedAt.Format(time.RFC3339) {
		t.Errorf("expected created_at %v, got %v", edge.CreatedAt.Format(time.RFC3339), result["created_at"])
	}
}

// TestEdgeUnmarshalJSON tests the JSON unmarshaling of an Edge object.
// It creates a JSON representation of an Edge object, unmarshals it, and then
// checks if the resulting Edge object matches the expected values.
func TestEdgeUnmarshalJSON(t *testing.T) {
	id := xid.New()
	source := xid.New()
	target := xid.New()
	createdAt := time.Now().Format(time.RFC3339)

	data := []byte(`{
		"id": "` + id.String() + `",
		"type": "friend",
		"source": "` + source.String() + `",
		"target": "` + target.String() + `",
		"label": "knows",
		"weight": 0.75,
		"created_at": "` + createdAt + `"
	}`)

	var edge Edge
	err := json.Unmarshal(data, &edge)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if edge.ID != id {
		t.Errorf("expected id %v, got %v", id, edge.ID)
	}
	if edge.Type != "friend" {
		t.Errorf("expected type 'friend', got %v", edge.Type)
	}
	if edge.Source != source {
		t.Errorf("expected source %v, got %v", source, edge.Source)
	}
	if edge.Target != target {
		t.Errorf("expected target %v, got %v", target, edge.Target)
	}
	if edge.Label != "knows" {
		t.Errorf("expected label 'knows', got %v", edge.Label)
	}
	if edge.Weight != 0.75 {
		t.Errorf("expected weight 0.75, got %v", edge.Weight)
	}
	if edge.CreatedAt.Format(time.RFC3339) != createdAt {
		t.Errorf("expected created_at %v, got %v", createdAt, edge.CreatedAt.Format(time.RFC3339))
	}
}

// TestEdgeUnmarshalJSONInvalidID tests the JSON unmarshaling of an Edge object with invalid IDs.
// It creates a JSON representation with invalid IDs, attempts to unmarshal it, and expects an error.
func TestEdgeUnmarshalJSONInvalidID(t *testing.T) {
	data := []byte(`{
		"id": "invalid",
		"type": "friend",
		"source": "invalid",
		"target": "invalid",
		"label": "knows",
		"weight": 0.75,
		"created_at": "2021-04-20T12:00:00Z"
	}`)

	var edge Edge
	err := json.Unmarshal(data, &edge)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
