package core

import (
	"encoding/json"
	"time"

	"github.com/rs/xid"
)

// Edge is the smallest unit of relation. An Edge relates two Nodes, a Source and a Target, with a given Label.
// An Edge has an ID, defined using xid.ID such that it is unique and sortable.
// Both Source and Target Nodes are identified by their ID.
// Each Edge has a Type that determines the relationship between the Source and Target Nodes.
// The Weight field is a float64 that can be used to represent the strength of the relationship.
// An Edge is unidirectional, meaning that the Source Node is the origin of the Edge and the Target Node is the destination.
// The Label field is a string that describes the relationship between the Source and Target Nodes.
type Edge struct {
	ID        xid.ID    `json:"id"`
	Type      string    `json:"type"`
	Source    xid.ID    `json:"source"`
	Target    xid.ID    `json:"target"`
	Label     string    `json:"label"`
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"created_at"`
}

// MarshalJSON marshals the Edge to JSON.
// The ID, Source, and Target fields are converted to strings to ensure they are properly represented in JSON format.
func (e Edge) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID        string  `json:"id"`
		Type      string  `json:"type"`
		Source    string  `json:"source"`
		Target    string  `json:"target"`
		Label     string  `json:"label"`
		Weight    float64 `json:"weight"`
		CreatedAt string  `json:"created_at"`
	}{
		ID:        e.ID.String(), // Convert ID to string for JSON representation
		Type:      e.Type,
		Source:    e.Source.String(), // Convert Source to string for JSON representation
		Target:    e.Target.String(), // Convert Target to string for JSON representation
		Label:     e.Label,
		Weight:    e.Weight,
		CreatedAt: e.CreatedAt.Format(time.RFC3339),
	})
}

// UnmarshalJSON unmarshals the Edge from JSON.
func (e *Edge) UnmarshalJSON(data []byte) error {
	// aux is used as an intermediary struct to unmarshal the JSON data into,
	// allowing us to handle the ID, Source, and Target fields as strings and then convert them to xid.ID.
	var aux struct {
		ID        string    `json:"id"`
		Type      string    `json:"type"`
		Source    string    `json:"source"`
		Target    string    `json:"target"`
		Label     string    `json:"label"`
		Weight    float64   `json:"weight"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	id, err := xid.FromString(aux.ID)
	if err != nil {
		return err
	}
	source, err := xid.FromString(aux.Source)
	if err != nil {
		return err
	}
	target, err := xid.FromString(aux.Target)
	if err != nil {
		return err
	}
	e.ID = id
	e.Type = aux.Type
	e.Source = source
	e.Target = target
	e.Label = aux.Label
	e.Weight = aux.Weight
	createdAt, err := time.Parse(time.RFC3339, aux.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return err
	}
	e.CreatedAt = createdAt
	return nil
}

// newEdge creates a new Edge with the given parameters.
//
// Parameters:
//   - edgeType: A string representing the type of the edge.
//   - source: The source node ID of type xid.ID.
//   - target: The target node ID of type xid.ID.
//   - label: A string representing the label of the edge.
//   - weight: A float64 representing the weight of the edge.
//
// Returns an Edge struct initialized with the provided parameters and a new ID and CreatedAt timestamp.
func newEdge(edgeType string, source, target xid.ID, label string, weight float64) Edge {
	return Edge{
		ID:        xid.New(),
		Type:      edgeType,
		Source:    source,
		Target:    target,
		Label:     label,
		Weight:    weight,
		CreatedAt: time.Now(),
	}
}
