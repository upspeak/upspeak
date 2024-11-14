package core

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Common function to test Node JSON marshaling and unmarshaling
func testNodeJSON[M, B any](t *testing.T, node *Node[M, B], expectedMetadata M, expectedBody B) {
	// Marshal the Node to JSON
	jsonData, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("Failed to marshal Node to JSON: %v", err)
	}

	// Ensure the JSON is as expected
	var unmarshaledNode map[string]interface{}
	if err := json.Unmarshal(jsonData, &unmarshaledNode); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Handle expected metadata comparison
	expectedMetadataJSON, err := json.Marshal(expectedMetadata)
	if err != nil {
		t.Fatalf("Failed to marshal expected metadata to JSON: %v", err)
	}
	var expectedMetadataValue interface{}
	if err := json.Unmarshal(expectedMetadataJSON, &expectedMetadataValue); err != nil {
		t.Fatalf("Failed to unmarshal expected metadata JSON: %v", err)
	}
	if !compareValues(unmarshaledNode["metadata"], expectedMetadataValue) {
		t.Errorf("Expected metadata %v, got %v", expectedMetadataValue, unmarshaledNode["metadata"])
	}

	// Handle expected body comparison
	expectedBodyJSON, err := json.Marshal(expectedBody)
	if err != nil {
		t.Fatalf("Failed to marshal expected body to JSON: %v", err)
	}
	var expectedBodyValue interface{}
	if err := json.Unmarshal(expectedBodyJSON, &expectedBodyValue); err != nil {
		t.Fatalf("Failed to unmarshal expected body JSON: %v", err)
	}
	if !compareValues(unmarshaledNode["body"], expectedBodyValue) {
		t.Errorf("Expected body %v, got %v", expectedBodyValue, unmarshaledNode["body"])
	}
}

// Helper function to compare two values
func compareValues(a, b interface{}) bool {
	switch a := a.(type) {
	case map[string]interface{}:
		b, ok := b.(map[string]interface{})
		if !ok {
			return false
		}
		return compareMaps(a, b)
	default:
		return a == b
	}
}

// Helper function to compare two maps
func compareMaps(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestNodeWithStringMetadataAndBody(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		metadata string
		body     string
	}{
		{
			name:     "example test",
			kind:     "exampleKind",
			metadata: "example metadata",
			body:     "example body",
		},
		{
			name:     "empty metadata and body",
			kind:     "emptyKind",
			metadata: "",
			body:     "",
		},
		{
			name:     "whitespace metadata and body",
			kind:     "whitespaceKind",
			metadata: "   ",
			body:     "   ",
		},
		{
			name:     "special characters",
			kind:     "specialKind",
			metadata: "!@#$%^&*()",
			body:     "<>{}[]",
		},
		{
			name:     "long string",
			kind:     "longStringKind",
			metadata: "a very long string that exceeds normal length",
			body:     "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode(Kind(tt.kind), tt.metadata, tt.body)
			testNodeJSON[string, string](t, &node, tt.metadata, tt.body)
		})
	}
}

type TextNodeMetadata struct {
	Version int    `json:"version"`
	Author  string `json:"author"`
}

type TextNodeBody struct {
	Text      string `json:"text"`
	WordCount int    `json:"word_count"`
}

func TestNodeWithCustomStructMetadataAndBody(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		metadata TextNodeMetadata
		body     TextNodeBody
	}{
		{
			name:     "example test",
			kind:     "customKind",
			metadata: TextNodeMetadata{Version: 1, Author: "John Doe"},
			body:     TextNodeBody{Text: "example text", WordCount: 2},
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode(Kind(tt.kind), tt.metadata, tt.body)
			testNodeJSON[TextNodeMetadata, TextNodeBody](t, &node, tt.metadata, tt.body)
		})
	}
}

func TestKindJSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected string
	}{
		{
			name:     "simple kind",
			kind:     "exampleKind",
			expected: `"exampleKind"`,
		},
		// Question: Should an empty Kind be allowed? Allowing for now.
		{
			name:     "empty kind",
			kind:     "",
			expected: `""`,
		},
		{
			name:     "special characters",
			kind:     "!@#$%^&*()",
			expected: `"!@#$%^\u0026*()"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			var buf bytes.Buffer
			encoder := json.NewEncoder(&buf)
			if err := encoder.Encode(tt.kind); err != nil {
				t.Fatalf("Failed to marshal Kind to JSON: %v", err)
			}
			jsonData := buf.Bytes()
			if string(jsonData) != tt.expected+"\n" {
				t.Errorf("Expected JSON %v, got %v", tt.expected, string(jsonData))
			}

			// Test unmarshaling
			var unmarshaledKind Kind
			if err := json.Unmarshal(jsonData, &unmarshaledKind); err != nil {
				t.Fatalf("Failed to unmarshal JSON to Kind: %v", err)
			}
			if unmarshaledKind != tt.kind {
				t.Errorf("Expected Kind %v, got %v", tt.kind, unmarshaledKind)
			}
		})
	}
}
