package core

// Thread is an aggregate; a composite Node using Edges for relationships.
type Thread struct {
	Node     Node       `json:"node"`     // A Node is the aggregate root of the thread
	Edges    []Edge     `json:"edges"`    // List of Edges representing relations between nodes in the thread
	Metadata []Metadata `json:"metadata"` // Additional custom metadata for the thread
}
