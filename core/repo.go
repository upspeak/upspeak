package core

import (
	"github.com/rs/xid"
)

// Repo groups Nodes (core.Node) and Edges (core.Edge) in a context.
type Repo struct {
	Name       string
	Visibility string // "private" or "public"
	NodeIDs    []xid.ID
	EdgeIDs    []xid.ID
	Archive    Archive
}

// NewRepo creates a new Repo with the given Archive.
func NewRepo(name, visibility string, archive Archive) *Repo {
	return &Repo{
		Name:       name,
		Visibility: visibility,
		NodeIDs:    []xid.ID{},
		EdgeIDs:    []xid.ID{},
		Archive:    archive,
	}
}

// AddNode adds a new Node to the Repo and syncs with the Archive.
func (r *Repo) AddNode(node Node) error {
	r.NodeIDs = append(r.NodeIDs, node.ID)
	return r.Archive.UpdateNode(&node)
}

// AddEdge adds a new Edge to the Repo and syncs with the Archive.
func (r *Repo) AddEdge(edge Edge) error {
	r.EdgeIDs = append(r.EdgeIDs, edge.ID)
	return r.Archive.UpdateEdge(&edge)
}

// GetNode retrieves a Node by its ID from the Archive.
func (r *Repo) GetNode(id xid.ID) (*Node, error) {
	return r.Archive.FetchNode(id.String())
}

// GetEdge retrieves an Edge by its ID from the Archive.
func (r *Repo) GetEdge(id xid.ID) (*Edge, error) {
	return r.Archive.FetchEdge(id.String())
}

// Sync syncs the Repo with the Archive.
func (r *Repo) Sync() error {
	_, err := r.Archive.Load()
	return err
}
