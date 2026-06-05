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
	// GetNodeBySourceExternalID finds a node by ingestion provenance for
	// idempotent re-collection. Returns ErrorNotFound when absent.
	GetNodeBySourceExternalID(sourceID uuid.UUID, externalID string) (*Node, error)
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

// FilterStore handles filter persistence.
type FilterStore interface {
	SaveFilter(filter *Filter) error
	GetFilter(filterID uuid.UUID) (*Filter, error)
	DeleteFilter(filterID uuid.UUID) error
	ListFilters(repoID uuid.UUID, opts FilterListOptions) ([]Filter, int, error)
	// GetFilterReferences returns entity type/ID pairs that reference the
	// given filter (sources, sinks, rules). Used to enforce referential
	// integrity on delete.
	GetFilterReferences(filterID uuid.UUID) ([]FilterReference, error)
}

// FilterReference describes an entity that references a filter.
type FilterReference struct {
	EntityType string `json:"entity_type"` // "source", "sink", "rule"
	EntityID   string `json:"entity_id"`
	EntityName string `json:"entity_name"`
}

// JobStore handles job persistence.
type JobStore interface {
	SaveJob(job *Job) error
	GetJob(jobID uuid.UUID) (*Job, error)
	GetJobByShortID(shortID string) (*Job, error)
	ListJobs(opts JobListOptions) ([]Job, int, error)
}

// SourceStore handles source persistence.
type SourceStore interface {
	SaveSource(source *Source) error
	GetSource(sourceID uuid.UUID) (*Source, error)
	DeleteSource(sourceID uuid.UUID) error
	ListSources(repoID uuid.UUID, opts SourceListOptions) ([]Source, int, error)
}

// SinkStore handles sink persistence.
type SinkStore interface {
	SaveSink(sink *Sink) error
	GetSink(sinkID uuid.UUID) (*Sink, error)
	DeleteSink(sinkID uuid.UUID) error
	ListSinks(repoID uuid.UUID, opts SinkListOptions) ([]Sink, int, error)
}

// ConnectionStore handles owner-scoped connection persistence. Credentials are
// encrypted at rest by the implementation.
type ConnectionStore interface {
	// SaveConnection persists the full Connection state. When updating an
	// existing connection, callers must load it first (GetConnection populates
	// the decrypted Credentials), modify, then save: saving a Connection with
	// empty Credentials nulls the stored secret.
	SaveConnection(conn *Connection) error
	GetConnection(connID uuid.UUID) (*Connection, error)
	ListConnections(ownerID uuid.UUID, opts ConnectionListOptions) ([]Connection, int, error)
	DeleteConnection(connID uuid.UUID) error
	// GetConnectionReferences returns sources/sinks referencing the connection,
	// for delete integrity. Reuses FilterReference for the {type,id,name} shape.
	GetConnectionReferences(connID uuid.UUID) ([]FilterReference, error)
}

// IngestCursorStore handles per-source ingestion cursor persistence.
// The cursor payload is opaque — adapters define their own resumption format.
type IngestCursorStore interface {
	SaveIngestCursor(c *IngestCursor) error
	GetIngestCursor(sourceID uuid.UUID) (*IngestCursor, error)
}

// ConnectorHistoryStore handles collection and publish history persistence.
type ConnectorHistoryStore interface {
	RecordCollectionAttempt(record *CollectionRecord) error
	RecordPublishAttempt(record *PublishRecord) error
	GetSourceHistory(sourceID uuid.UUID, opts ListOptions) ([]CollectionRecord, int, error)
	GetSinkHistory(sinkID uuid.UUID, opts ListOptions) ([]PublishRecord, int, error)
}

// ScheduleStore handles schedule persistence.
type ScheduleStore interface {
	SaveSchedule(schedule *Schedule) error
	GetSchedule(scheduleID uuid.UUID) (*Schedule, error)
	GetScheduleByShortID(shortID string) (*Schedule, error)
	DeleteSchedule(scheduleID uuid.UUID) error
	ListSchedules(opts ScheduleListOptions) ([]Schedule, int, error)
	GetEnabledSchedules() ([]Schedule, error)
}

// RuleStore handles rule persistence and execution history.
type RuleStore interface {
	SaveRule(rule *Rule) error
	GetRule(ruleID uuid.UUID) (*Rule, error)
	DeleteRule(ruleID uuid.UUID) error
	ListRules(repoID uuid.UUID, opts RuleListOptions) ([]Rule, int, error)
	GetActiveRulesForEvent(repoID uuid.UUID, eventType EventType) ([]Rule, error)
	SaveRuleExecution(exec *RuleExecution) error
	ListRuleExecutions(ruleID uuid.UUID, opts ListOptions) ([]RuleExecution, int, error)
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
	FilterStore
	JobStore
	SourceStore
	SinkStore
	ConnectionStore
	IngestCursorStore
	ConnectorHistoryStore
	ScheduleStore
	RuleStore
	SearchStore
	RefResolver
}
