package core

import "github.com/rs/xid"

// NodeCommand is an interface that defines the command methods to manage nodes
type NodeCommand interface {
	CreateNode(node *Node) error
	UpdateNode(node *Node) error
	DeleteNode(id xid.ID) error
}

// NodeQuery is an interface that defines the query methods to fetch nodes
type NodeQuery interface {
	GetNode(id xid.ID) (*Node, error)
	GetNodes(query NodeQueryParams) ([]*Node, error)
}

// EdgeCommand is an interface that defines the command methods to manage edges
type EdgeCommand interface {
	CreateEdge(edge *Edge) error
	UpdateEdge(edge *Edge) error
	DeleteEdge(id xid.ID) error
}

// EdgeQuery is an interface that defines the query methods to fetch edges
type EdgeQuery interface {
	GetEdge(id xid.ID) (*Edge, error)
	GetEdges(query EdgeQueryParams) ([]*Edge, error)
}

// NodeQueryParams defines the query parameters for fetching nodes
type NodeQueryParams struct {
	Page     int
	PageSize int
}

// EdgeQueryParams defines the query parameters for fetching edges
type EdgeQueryParams struct {
	SourceID *xid.ID // Optional field to filter edges by source node ID
	TargetID *xid.ID // Optional field to filter edges by target node ID
	// ...fields for edge query...
}
