package core

import "github.com/rs/xid"

type Archive interface {
	// Node operations
	SaveNode(node *Node) error
	GetNode(nodeID xid.ID) (*Node, error)
	DeleteNode(nodeID xid.ID) error

	// Edge operations
	SaveEdge(edge *Edge) error
	GetEdge(edgeID xid.ID) (*Edge, error)
	DeleteEdge(edgeID xid.ID) error

	// Thread operations
	SaveThread(thread *Thread) error
	GetThread(nodeID xid.ID) (*Thread, error)
	DeleteThread(nodeID xid.ID) error

	// Annotation operations
	SaveAnnotation(annotation *Annotation) error
	GetAnnotation(nodeID xid.ID) (*Annotation, error)
	DeleteAnnotation(nodeID xid.ID) error

	// Sync repository state
	ReplayEvents(events []Event) error
	LoadEvents() ([]Event, error)
}
