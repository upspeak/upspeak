package core

type Annotation struct {
	Node       Node   `json:"node"`       // A Node is the aggregate root of the Annotation
	Edge       Edge   `json:"edge"`       // The edge linking the annotation to the target node, with type="annotation"
	Motivation string `json:"motivation"` // Reason for the annotation (e.g., "commenting", "highlighting")
}
