package core

import "github.com/rs/xid"

// Repo groups Nodes (core.Node) and Edges (core.Edge) in a context.
type Repo struct {
	Name       string
	Visibility string // "private" or "public"
}

// NewRepo creates a new Repo with the given Archive.
func NewRepo(name, visibility string) *Repo {
	return &Repo{
		Name:       name,
		Visibility: visibility,
	}
}

// CreateNode adds a new Node to the Repo and syncs with the Archive.
func (r *Repo) CreateNode(archive NodeCommand, node *Node) (*Event, error) {
	if err := archive.CreateNode(node); err != nil {
		return nil, err
	}
	return nodeCreatedEvent(node)
}

// UpdateNode updates an existing Node in the Repo and syncs with the Archive.
func (r *Repo) UpdateNode(archive NodeCommand, node *Node) (*Event, error) {
	if err := archive.UpdateNode(node); err != nil {
		return nil, err
	}
	return nodeUpdatedEvent(node)
}

// DeleteNode removes a Node from the Repo and syncs with the Archive.
func (r *Repo) DeleteNode(archive NodeCommand, id xid.ID) (*Event, error) {
	if err := archive.DeleteNode(id); err != nil {
		return nil, err
	}
	return nodeDeletedEvent(id)
}

// CreateEdge creates a new Edge to the Repo and syncs with the Archive.
func (r *Repo) CreateEdge(archive EdgeCommand, edge *Edge) (*Event, error) {
	if err := archive.CreateEdge(edge); err != nil {
		return nil, err
	}
	return edgeCreatedEvent(edge)
}

// UpdateEdge updates an existing Edge in the Repo and syncs with the Archive.
func (r *Repo) UpdateEdge(archive EdgeCommand, edge *Edge) (*Event, error) {
	if err := archive.UpdateEdge(edge); err != nil {
		return nil, err
	}
	return edgeUpdatedEvent(edge)
}

// DeleteEdge removes an Edge from the Repo and syncs with the Archive.
func (r *Repo) DeleteEdge(archive EdgeCommand, id xid.ID) (*Event, error) {
	if err := archive.DeleteEdge(id); err != nil {
		return nil, err
	}
	return edgeDeletedEvent(id)
}

// GetNode retrieves a Node by its ID from the Archive.
func (r *Repo) GetNode(archive NodeQuery, id xid.ID) (*Node, error) {
	return archive.GetNode(id)
}

// GetEdge retrieves an Edge by its ID from the Archive.
func (r *Repo) GetEdge(archive EdgeQuery, id xid.ID) (*Edge, error) {
	return archive.GetEdge(id)
}
