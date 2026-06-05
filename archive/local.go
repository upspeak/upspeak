package archive

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/upspeak/upspeak/core"
)

// LocalArchive implements the core.Archive interface using local file system storage.
// Metadata is stored in SQLite, and node body content is stored as files in the
// content/ directory. This separation supports the local/remote archive split
// defined in the high-level architecture.
type LocalArchive struct {
	path         string
	contentDir   string
	db           *sql.DB
	ftsAvailable bool
	cipher       core.SecretCipher
}

// Compile-time assertion that LocalArchive satisfies the full core.Archive
// interface. A half-wired sub-interface fails here, at the package boundary,
// rather than later at the call site in main.
var _ core.Archive = (*LocalArchive)(nil)

// SetSecretCipher injects the cipher used to encrypt connection credentials at
// rest. Until set, writes that carry credentials fail with core.ErrSecretKeyMissing.
func (a *LocalArchive) SetSecretCipher(c core.SecretCipher) { a.cipher = c }

// NewLocalArchive creates a new LocalArchive at the specified path.
func NewLocalArchive(path string) (*LocalArchive, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archive directory: %w", err)
	}

	upspeakDir := filepath.Join(path, ".upspeak")
	if err := os.MkdirAll(upspeakDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .upspeak directory: %w", err)
	}

	contentDir := filepath.Join(path, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create content directory: %w", err)
	}

	dbPath := filepath.Join(upspeakDir, "metadata.db")
	// Enable foreign keys, set a 5-second busy timeout to avoid SQLITE_BUSY under
	// contention, and enable secure_delete to overwrite deleted data with zeroes.
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000&_secure_delete=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	archive := &LocalArchive{path: path, contentDir: contentDir, db: db}

	if err := archive.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialise schema: %w", err)
	}

	return archive, nil
}

// initSchema creates the database tables if they don't exist.
func (a *LocalArchive) initSchema() error {
	_, err := a.db.Exec(schemaSQL)
	if err != nil {
		return err
	}

	// Apply FTS5 schema separately — it requires SQLite compiled with FTS5
	// support (the sqlite_fts5 build tag). If the module is genuinely absent,
	// degrade to empty search results rather than breaking the whole archive.
	// Any other error (typo, disk, lock) is a real failure and is returned.
	_, err = a.db.Exec(ftsSchemaSQL)
	if err == nil {
		a.ftsAvailable = true
		return nil
	}

	if isFTS5UnavailableError(err) {
		a.ftsAvailable = false
		slog.Warn("FTS5 not available; full-text search is disabled. "+
			"Rebuild with -tags sqlite_fts5 to enable search.", "error", err)
		return nil
	}

	return fmt.Errorf("failed to create FTS5 schema: %w", err)
}

// isFTS5UnavailableError reports whether err indicates that the SQLite driver
// was built without FTS5 support, as opposed to a genuine SQL failure.
func isFTS5UnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such module: fts5")
}

// FTSAvailable reports whether full-text search is operational. It is false when
// the SQLite driver was built without FTS5 support, in which case SearchNodes
// returns empty results rather than an error.
func (a *LocalArchive) FTSAvailable() bool {
	return a.ftsAvailable
}

// Close closes the database connection.
func (a *LocalArchive) Close() error {
	return a.db.Close()
}

// contentPath returns the filesystem path for a node's body content.
func (a *LocalArchive) contentPath(nodeID uuid.UUID) string {
	return filepath.Join(a.contentDir, nodeID.String())
}

// --- core.RepositoryStore implementation ---

func (a *LocalArchive) SaveRepository(repo *core.Repository) error {
	return a.saveRepository(repo)
}

func (a *LocalArchive) GetRepository(repoID uuid.UUID) (*core.Repository, error) {
	return a.getRepository(repoID)
}

func (a *LocalArchive) GetRepositoryBySlug(ownerID uuid.UUID, slug string) (*core.Repository, error) {
	return a.getRepositoryBySlug(ownerID, slug)
}

func (a *LocalArchive) ListRepositories(ownerID uuid.UUID, opts core.ListOptions) ([]core.Repository, int, error) {
	return a.listRepositories(ownerID, opts)
}

func (a *LocalArchive) DeleteRepository(repoID uuid.UUID) error {
	return a.deleteRepository(repoID)
}

func (a *LocalArchive) SaveSlugRedirect(ownerID uuid.UUID, oldSlug string, repoID uuid.UUID) error {
	return a.saveSlugRedirect(ownerID, oldSlug, repoID)
}

func (a *LocalArchive) GetSlugRedirect(ownerID uuid.UUID, slug string) (uuid.UUID, string, error) {
	return a.getSlugRedirect(ownerID, slug)
}

func (a *LocalArchive) ResolveRepoRef(ownerID uuid.UUID, ref string) (*core.Repository, error) {
	return a.resolveRepoRef(ownerID, ref)
}

// --- core.NodeStore implementation ---

func (a *LocalArchive) SaveNode(node *core.Node) error {
	return a.saveNode(node)
}

func (a *LocalArchive) SaveBatchNodes(nodes []*core.Node) error {
	return a.saveBatchNodes(nodes)
}

func (a *LocalArchive) GetNode(nodeID uuid.UUID) (*core.Node, error) {
	return a.getNode(nodeID)
}

func (a *LocalArchive) DeleteNode(nodeID uuid.UUID) error {
	return a.deleteNode(nodeID)
}

func (a *LocalArchive) ListNodes(repoID uuid.UUID, opts core.NodeListOptions) ([]core.Node, int, error) {
	return a.listNodes(repoID, opts)
}

func (a *LocalArchive) GetNodeEdges(nodeID uuid.UUID, opts core.EdgeQueryOptions) ([]core.Edge, int, error) {
	return a.getNodeEdges(nodeID, opts)
}

func (a *LocalArchive) GetNodeAnnotations(nodeID uuid.UUID, opts core.AnnotationQueryOptions) ([]core.Annotation, int, error) {
	return a.getNodeAnnotations(nodeID, opts)
}

func (a *LocalArchive) GetNodeBySourceExternalID(sourceID uuid.UUID, externalID string) (*core.Node, error) {
	return a.getNodeBySourceExternalID(sourceID, externalID)
}

// --- core.EdgeStore implementation ---

func (a *LocalArchive) SaveEdge(edge *core.Edge) error {
	return a.saveEdge(edge)
}

func (a *LocalArchive) SaveBatchEdges(edges []*core.Edge) error {
	return a.saveBatchEdges(edges)
}

func (a *LocalArchive) GetEdge(edgeID uuid.UUID) (*core.Edge, error) {
	return a.getEdge(edgeID)
}

func (a *LocalArchive) DeleteEdge(edgeID uuid.UUID) error {
	return a.deleteEdge(edgeID)
}

func (a *LocalArchive) ListEdges(repoID uuid.UUID, opts core.EdgeListOptions) ([]core.Edge, int, error) {
	return a.listEdges(repoID, opts)
}

// --- core.ThreadStore implementation ---

func (a *LocalArchive) SaveThread(thread *core.Thread) error {
	return a.saveThread(thread)
}

func (a *LocalArchive) GetThread(threadID uuid.UUID) (*core.Thread, error) {
	return a.getThread(threadID)
}

func (a *LocalArchive) DeleteThread(threadID uuid.UUID) error {
	return a.deleteThread(threadID)
}

func (a *LocalArchive) ListThreads(repoID uuid.UUID, opts core.ListOptions) ([]core.Thread, int, error) {
	return a.listThreads(repoID, opts)
}

func (a *LocalArchive) AddNodeToThread(threadID, nodeID uuid.UUID, edgeType string) error {
	return a.addNodeToThread(threadID, nodeID, edgeType)
}

func (a *LocalArchive) RemoveNodeFromThread(threadID, nodeID uuid.UUID) error {
	return a.removeNodeFromThread(threadID, nodeID)
}

// --- core.AnnotationStore implementation ---

func (a *LocalArchive) SaveAnnotation(annotation *core.Annotation) error {
	return a.saveAnnotation(annotation)
}

func (a *LocalArchive) GetAnnotation(annotationID uuid.UUID) (*core.Annotation, error) {
	return a.getAnnotation(annotationID)
}

func (a *LocalArchive) DeleteAnnotation(annotationID uuid.UUID) error {
	return a.deleteAnnotation(annotationID)
}

func (a *LocalArchive) ListAnnotations(repoID uuid.UUID, opts core.ListOptions) ([]core.Annotation, int, error) {
	return a.listAnnotations(repoID, opts)
}

// --- core.FilterStore implementation ---

func (a *LocalArchive) SaveFilter(filter *core.Filter) error {
	return a.saveFilter(filter)
}

func (a *LocalArchive) GetFilter(filterID uuid.UUID) (*core.Filter, error) {
	return a.getFilter(filterID)
}

func (a *LocalArchive) DeleteFilter(filterID uuid.UUID) error {
	return a.deleteFilter(filterID)
}

func (a *LocalArchive) ListFilters(repoID uuid.UUID, opts core.FilterListOptions) ([]core.Filter, int, error) {
	return a.listFilters(repoID, opts)
}

func (a *LocalArchive) GetFilterReferences(filterID uuid.UUID) ([]core.FilterReference, error) {
	return a.getFilterReferences(filterID)
}

// --- core.JobStore implementation ---

func (a *LocalArchive) SaveJob(job *core.Job) error {
	return a.saveJob(job)
}

func (a *LocalArchive) GetJob(jobID uuid.UUID) (*core.Job, error) {
	return a.getJob(jobID)
}

func (a *LocalArchive) GetJobByShortID(shortID string) (*core.Job, error) {
	return a.getJobByShortID(shortID)
}

func (a *LocalArchive) ListJobs(opts core.JobListOptions) ([]core.Job, int, error) {
	return a.listJobs(opts)
}

// --- core.ScheduleStore implementation ---

func (a *LocalArchive) SaveSchedule(schedule *core.Schedule) error {
	return a.saveSchedule(schedule)
}

func (a *LocalArchive) GetSchedule(scheduleID uuid.UUID) (*core.Schedule, error) {
	return a.getSchedule(scheduleID)
}

func (a *LocalArchive) GetScheduleByShortID(shortID string) (*core.Schedule, error) {
	return a.getScheduleByShortID(shortID)
}

func (a *LocalArchive) DeleteSchedule(scheduleID uuid.UUID) error {
	return a.deleteSchedule(scheduleID)
}

func (a *LocalArchive) ListSchedules(opts core.ScheduleListOptions) ([]core.Schedule, int, error) {
	return a.listSchedules(opts)
}

func (a *LocalArchive) GetEnabledSchedules() ([]core.Schedule, error) {
	return a.getEnabledSchedules()
}

// --- core.SourceStore implementation ---

func (a *LocalArchive) SaveSource(source *core.Source) error {
	return a.saveSource(source)
}

func (a *LocalArchive) GetSource(sourceID uuid.UUID) (*core.Source, error) {
	return a.getSource(sourceID)
}

func (a *LocalArchive) DeleteSource(sourceID uuid.UUID) error {
	return a.deleteSource(sourceID)
}

func (a *LocalArchive) ListSources(repoID uuid.UUID, opts core.SourceListOptions) ([]core.Source, int, error) {
	return a.listSources(repoID, opts)
}

// --- core.SinkStore implementation ---

func (a *LocalArchive) SaveSink(sink *core.Sink) error {
	return a.saveSink(sink)
}

func (a *LocalArchive) GetSink(sinkID uuid.UUID) (*core.Sink, error) {
	return a.getSink(sinkID)
}

func (a *LocalArchive) DeleteSink(sinkID uuid.UUID) error {
	return a.deleteSink(sinkID)
}

func (a *LocalArchive) ListSinks(repoID uuid.UUID, opts core.SinkListOptions) ([]core.Sink, int, error) {
	return a.listSinks(repoID, opts)
}

// --- core.ConnectionStore implementation ---

func (a *LocalArchive) SaveConnection(conn *core.Connection) error {
	return a.saveConnection(conn)
}

func (a *LocalArchive) GetConnection(connID uuid.UUID) (*core.Connection, error) {
	return a.getConnection(connID)
}

func (a *LocalArchive) ListConnections(ownerID uuid.UUID, opts core.ConnectionListOptions) ([]core.Connection, int, error) {
	return a.listConnections(ownerID, opts)
}

func (a *LocalArchive) DeleteConnection(connID uuid.UUID) error {
	return a.deleteConnection(connID)
}

func (a *LocalArchive) GetConnectionReferences(connID uuid.UUID) ([]core.FilterReference, error) {
	return a.getConnectionReferences(connID)
}

// --- core.IngestCursorStore implementation ---

func (a *LocalArchive) SaveIngestCursor(c *core.IngestCursor) error {
	return a.saveIngestCursor(c)
}

func (a *LocalArchive) GetIngestCursor(sourceID uuid.UUID) (*core.IngestCursor, error) {
	return a.getIngestCursor(sourceID)
}

// --- core.ConnectorHistoryStore implementation ---

func (a *LocalArchive) RecordCollectionAttempt(record *core.CollectionRecord) error {
	return a.recordCollectionAttempt(record)
}

func (a *LocalArchive) RecordPublishAttempt(record *core.PublishRecord) error {
	return a.recordPublishAttempt(record)
}

func (a *LocalArchive) GetSourceHistory(sourceID uuid.UUID, opts core.ListOptions) ([]core.CollectionRecord, int, error) {
	return a.getSourceHistory(sourceID, opts)
}

func (a *LocalArchive) GetSinkHistory(sinkID uuid.UUID, opts core.ListOptions) ([]core.PublishRecord, int, error) {
	return a.getSinkHistory(sinkID, opts)
}

// --- core.RuleStore implementation ---

func (a *LocalArchive) SaveRule(rule *core.Rule) error {
	return a.saveRule(rule)
}

func (a *LocalArchive) GetRule(ruleID uuid.UUID) (*core.Rule, error) {
	return a.getRule(ruleID)
}

func (a *LocalArchive) DeleteRule(ruleID uuid.UUID) error {
	return a.deleteRule(ruleID)
}

func (a *LocalArchive) ListRules(repoID uuid.UUID, opts core.RuleListOptions) ([]core.Rule, int, error) {
	return a.listRules(repoID, opts)
}

func (a *LocalArchive) GetActiveRulesForEvent(repoID uuid.UUID, eventType core.EventType) ([]core.Rule, error) {
	return a.getActiveRulesForEvent(repoID, eventType)
}

func (a *LocalArchive) SaveRuleExecution(exec *core.RuleExecution) error {
	return a.saveRuleExecution(exec)
}

func (a *LocalArchive) ListRuleExecutions(ruleID uuid.UUID, opts core.ListOptions) ([]core.RuleExecution, int, error) {
	return a.listRuleExecutions(ruleID, opts)
}

// --- core.SearchStore implementation ---

func (a *LocalArchive) IndexNode(nodeID uuid.UUID, repoID uuid.UUID, subject string, bodyText string) error {
	return a.indexNode(nodeID, repoID, subject, bodyText)
}

func (a *LocalArchive) RemoveNodeIndex(nodeID uuid.UUID) error {
	return a.removeNodeIndex(nodeID)
}

func (a *LocalArchive) SearchNodes(repoID uuid.UUID, query string, opts core.SearchOptions) ([]core.SearchResult, int, error) {
	return a.searchNodes(repoID, query, opts)
}

func (a *LocalArchive) BrowseNodes(repoID uuid.UUID, opts core.BrowseOptions) ([]core.Node, int, error) {
	return a.browseNodes(repoID, opts)
}

func (a *LocalArchive) TraverseGraph(repoID uuid.UUID, startNodeID uuid.UUID, depth int, opts core.GraphOptions) (*core.GraphResult, error) {
	return a.traverseGraph(repoID, startNodeID, depth, opts)
}

// --- core.RefResolver implementation ---

func (a *LocalArchive) ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error) {
	return a.resolveRef(repoID, ref)
}
