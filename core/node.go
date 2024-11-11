package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// Node represents a generic Node whose Metadata and Body fields vary based on the Datatype.
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
