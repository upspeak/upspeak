package core

import (
	"encoding/json"
	"time"

	"github.com/rs/xid"
)

// Kind represents the structural type of a Node (e.g., thread, annotation).
// This type is used to differentiate between different kinds of Nodes.
// Each higher-order Node type should define its own Kind.
// Don't add HTML to the value of this type.
type Kind string

// MarshalJSON marshals the Kind to JSON.
func (k Kind) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(k))
}

// UnmarshalJSON unmarshals the Kind from JSON.
func (k *Kind) UnmarshalJSON(data []byte) error {
	var kindStr string
	if err := json.Unmarshal(data, &kindStr); err != nil {
		return err
	}
	*k = Kind(kindStr)
	return nil
}

// Node represents a unit of information. It is a low-level struct intended to be used as a building block for more complex, higher-order
// data structures that encapsulate specific types of data. e.g. a Discourse post, a GitHub issue, a Matrix message etc.
//
// Node provides common serialization and deserialization to all those higher-order types to ensure consistent format as JSON on the wire.
//
// - The ID field is a unique identifier for the Node, generated using the xid package so that it is unique and sortable.
// - The Kind field represents the structural type of the Node (e.g., thread, annotation).
// - The ContentType field represents the MIME type of the content in the Body.
// - Metadata and Body fields can be of any type that can be serialized to JSON.
// - The CreatedAt field records the time when the Node was created.
// - Metadata should include any information that describes the Node. Higher order Node implementations (specific Node types) should put any author or version info here.
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
//	metadata, _ := json.Marshal(MyMetadata{Author: "John Doe", Version: 1})
//	body, _ := json.Marshal(MyBody{Content: "Hello, world!"})
//	node := NewNode("example_kind", "text/plain", metadata, body)
//
// The above example creates a new Node with custom metadata and body types.
type Node struct {
	ID          xid.ID          `json:"id"`
	Kind        Kind            `json:"kind"` // Structural type (e.g., thread, annotation)
	ContentType string          `json:"content_type"`
	Metadata    json.RawMessage `json:"metadata"`
	Body        json.RawMessage `json:"body"`
	CreatedAt   time.Time       `json:"created_at"`
}

// NewNode creates a new Node with the given Metadata and Body.
func NewNode(kind Kind, contentType string, metadata json.RawMessage, body json.RawMessage) Node {
	return Node{
		ID:          xid.New(),
		Kind:        kind,
		ContentType: contentType,
		Metadata:    metadata,
		Body:        body,
		CreatedAt:   time.Now(),
	}
}

// MarshalJSON marshals the Node to JSON.
// The ID field is converted to a string to ensure it is properly represented in JSON format.
func (n Node) MarshalJSON() ([]byte, error) {
	type Alias Node
	return json.Marshal(&struct {
		ID string `json:"id"`
		*Alias
	}{
		ID:    n.ID.String(),
		Alias: (*Alias)(&n),
	})
}

// UnmarshalJSON unmarshals the Node from JSON.
func (n *Node) UnmarshalJSON(data []byte) error {
	type Alias Node
	aux := &struct {
		ID string `json:"id"`
		*Alias
	}{
		Alias: (*Alias)(n),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	id, err := xid.FromString(aux.ID)
	if err != nil {
		return err
	}
	n.ID = id
	return nil
}
