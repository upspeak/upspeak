package core

import "github.com/google/uuid"

// Archive defines the interface for persistent storage of domain entities.
// Implementations should handle storage of all entity types with pagination,
// optimistic concurrency control, and reference resolution.
type Archive interface {
	// Repository operations.
	SaveRepository(repo *Repository) error
	GetRepository(repoID uuid.UUID) (*Repository, error)
	GetRepositoryBySlug(ownerID uuid.UUID, slug string) (*Repository, error)
	ListRepositories(ownerID uuid.UUID, opts ListOptions) ([]Repository, int, error)
	DeleteRepository(repoID uuid.UUID) error

	// Slug management.
	SaveSlugRedirect(ownerID uuid.UUID, oldSlug string, repoID uuid.UUID) error
	GetSlugRedirect(ownerID uuid.UUID, slug string) (uuid.UUID, string, error)

	// Ref resolution.
	// ResolveRepoRef resolves a UUID, short ID, slug, or old slug to a Repository.
	// For old slugs, returns ErrorSlugRedirect so the caller can issue a 301.
	ResolveRepoRef(ownerID uuid.UUID, ref string) (*Repository, error)

	// Node operations.
	SaveNode(node *Node) error
	SaveBatchNodes(repoID uuid.UUID, nodes []*Node) error
	GetNode(nodeID uuid.UUID) (*Node, error)
	DeleteNode(nodeID uuid.UUID) error
	ListNodes(repoID uuid.UUID, nodeType string, opts ListOptions) ([]Node, int, error)
	GetNodeEdges(nodeID uuid.UUID, opts EdgeQueryOptions) ([]Edge, int, error)
	GetNodeAnnotations(nodeID uuid.UUID, opts AnnotationQueryOptions) ([]Annotation, int, error)

	// Edge operations.
	SaveEdge(edge *Edge) error
	SaveBatchEdges(repoID uuid.UUID, edges []*Edge) error
	GetEdge(edgeID uuid.UUID) (*Edge, error)
	DeleteEdge(edgeID uuid.UUID) error
	ListEdges(repoID uuid.UUID, source, target, edgeType string, opts ListOptions) ([]Edge, int, error)

	// Thread operations.
	SaveThread(thread *Thread) error
	GetThread(threadID uuid.UUID) (*Thread, error)
	DeleteThread(threadID uuid.UUID) error
	ListThreads(repoID uuid.UUID, opts ListOptions) ([]Thread, int, error)
	AddNodeToThread(threadID, nodeID uuid.UUID, edgeType string) error
	RemoveNodeFromThread(threadID, nodeID uuid.UUID) error

	// Annotation operations.
	SaveAnnotation(annotation *Annotation) error
	GetAnnotation(annotationID uuid.UUID) (*Annotation, error)
	DeleteAnnotation(annotationID uuid.UUID) error
	ListAnnotations(repoID uuid.UUID, opts ListOptions) ([]Annotation, int, error)

	// Entity ref resolution.
	// ResolveRef resolves a short ID (e.g. "NODE-42") or UUID string to the
	// canonical UUID and entity type within a repository. Returns
	// (uuid, entityType, error) where entityType is "node", "edge", etc.
	ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error)

	// Sequence operations for short ID generation.
	NextRepoSequence(repoID uuid.UUID, entity string) (int, error)
	NextUserSequence(ownerID uuid.UUID, entity string) (int, error)
	NextGlobalSequence(entity string) (int, error)
}
