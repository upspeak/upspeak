package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// Node represents a unit of information. It is a low-level struct intended to be used as a building block for more complex, higher-order
// structs that encapsulate specific types of data and behavior. End users are likely to interact with these higher-order structs
// rather than directly with the Node struct.
//
// Node provides common serialization and deserialization to all those higher-order types to ensure consistent format as JSON on the wire.
//
// - The ID field is a unique identifier for the Node, generated using the xid package so that it is unique and sortable.
// - A Node's Datatype defines how its Metadata and Body are interpreted.
// - Metadata and Body fields can be of any type that can be serialized to JSON.
// - The CreatedAt field records the time when the Node was created.
// - Metadata should include any information that describes the Node. Higher order Node implementations should put any author or version info here.
// - Body should include all the content relevant for the given Node type.
//
// Example usage:
//
//	type MyMetadata struct {
//	    Author string `json:"author"`
//	    Version int   `json:"version"`
//	}
//
//	type MyBody struct {
//	    Content string `json:"content"`
//	}
//
//	metadata := MyMetadata{Author: "John Doe", Version: 1}
//	body := MyBody{Content: "Hello, world!"}
//	node := NewNode("example_type", metadata, body)
//
// The above example creates a new Node with custom metadata and body types.
type Node[M any, B any] struct {
	ID        xid.ID    `json:"id"`
	Datatype  string    `json:"datatype"`
	Metadata  M         `json:"metadata"`
	Body      B         `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// NewNode creates a new Node with the given Datatype, Metadata, and Body.
func NewNode[M, B any](datatype string, metadata M, body B) Node[M, B] {
	return Node[M, B]{
		ID:        xid.New(),
		Datatype:  datatype,
		Metadata:  metadata,
		Body:      body,
		CreatedAt: time.Now(),
	}
}

// MarshalJSON marshals the Node to JSON.
// The ID field is converted to a string to ensure it is properly represented in JSON format.
func (n Node[M, B]) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID        string    `json:"id"`
		Datatype  string    `json:"datatype"`
		Metadata  M         `json:"metadata"`
		Body      B         `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}{
		ID:        n.ID.String(), // Convert ID to string for JSON representation
		Datatype:  n.Datatype,
		Metadata:  n.Metadata,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
	})
}

// UnmarshalJSON unmarshals the Node from JSON.
func (n *Node[M, B]) UnmarshalJSON(data []byte) error {
	// aux is used as an intermediary struct to unmarshal the JSON data into,
	// allowing us to handle the ID field as a string and then convert it to xid.ID.
	var aux struct {
		ID        string          `json:"id"`
		Datatype  string          `json:"datatype"`
		Metadata  json.RawMessage `json:"metadata"`
		Body      json.RawMessage `json:"body"`
		CreatedAt time.Time       `json:"created_at"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to parse ID from string: %w", err)
	}
	id, err := xid.FromString(aux.ID)
	if err != nil {
		return err
	}
	n.ID = id
	n.Datatype = aux.Datatype
	if err := json.Unmarshal(aux.Metadata, &n.Metadata); err != nil {
		return err
	}
	if err := json.Unmarshal(aux.Body, &n.Body); err != nil {
		return err
	}
	n.CreatedAt = aux.CreatedAt
	return nil
}
