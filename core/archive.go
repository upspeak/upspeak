package core

import "github.com/google/uuid"

// RepositoryStore handles repository persistence and slug management.
type RepositoryStore interface {
	SaveRepository(repo *Repository) error
	GetRepository(repoID uuid.UUID) (*Repository, error)
	GetRepositoryBySlug(ownerID uuid.UUID, slug string) (*Repository, error)
	ListRepositories(ownerID uuid.UUID, opts ListOptions) ([]Repository, int, error)
	DeleteRepository(repoID uuid.UUID) error

	// Slug management.
	SaveSlugRedirect(ownerID uuid.UUID, oldSlug string, repoID uuid.UUID) error
	GetSlugRedirect(ownerID uuid.UUID, slug string) (uuid.UUID, string, error)

	// ResolveRepoRef resolves a UUID, short ID, slug, or old slug to a Repository.
	// For old slugs, returns ErrorSlugRedirect so the caller can issue a 301.
	ResolveRepoRef(ownerID uuid.UUID, ref string) (*Repository, error)
}

// NodeStore handles node persistence.
type NodeStore interface {
	SaveNode(node *Node) error
	SaveBatchNodes(nodes []*Node) error
	GetNode(nodeID uuid.UUID) (*Node, error)
	DeleteNode(nodeID uuid.UUID) error
	ListNodes(repoID uuid.UUID, opts NodeListOptions) ([]Node, int, error)
	GetNodeEdges(nodeID uuid.UUID, opts EdgeQueryOptions) ([]Edge, int, error)
	GetNodeAnnotations(nodeID uuid.UUID, opts AnnotationQueryOptions) ([]Annotation, int, error)
}

// EdgeStore handles edge persistence.
type EdgeStore interface {
	SaveEdge(edge *Edge) error
	SaveBatchEdges(edges []*Edge) error
	GetEdge(edgeID uuid.UUID) (*Edge, error)
	DeleteEdge(edgeID uuid.UUID) error
	ListEdges(repoID uuid.UUID, opts EdgeListOptions) ([]Edge, int, error)
}

// ThreadStore handles thread persistence.
type ThreadStore interface {
	SaveThread(thread *Thread) error
	GetThread(threadID uuid.UUID) (*Thread, error)
	DeleteThread(threadID uuid.UUID) error
	ListThreads(repoID uuid.UUID, opts ListOptions) ([]Thread, int, error)
	AddNodeToThread(threadID, nodeID uuid.UUID, edgeType string) error
	RemoveNodeFromThread(threadID, nodeID uuid.UUID) error
}

// AnnotationStore handles annotation persistence.
type AnnotationStore interface {
	SaveAnnotation(annotation *Annotation) error
	GetAnnotation(annotationID uuid.UUID) (*Annotation, error)
	DeleteAnnotation(annotationID uuid.UUID) error
	ListAnnotations(repoID uuid.UUID, opts ListOptions) ([]Annotation, int, error)
}

// RefResolver resolves entity references within a repository.
type RefResolver interface {
	// ResolveRef resolves a short ID (e.g. "NODE-42") or UUID string to the
	// canonical UUID and entity type within a repository. Returns
	// (uuid, entityType, error) where entityType is "node", "edge", etc.
	ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error)
}

// Archive is the composed interface for the complete storage layer.
// Both local (SQLite + filesystem) and remote (Postgres + object storage)
// implementations satisfy this interface.
type Archive interface {
	RepositoryStore
	NodeStore
	EdgeStore
	ThreadStore
	AnnotationStore
	RefResolver
}
