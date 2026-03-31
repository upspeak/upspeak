package archive

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// LocalArchive implements the core.Archive interface using local file system storage.
// Metadata is stored in SQLite, and node body content is stored as files in the
// content/ directory. This separation supports the local/remote archive split
// defined in the high-level architecture.
type LocalArchive struct {
	path       string
	contentDir string
	db         *sql.DB
}

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
	return err
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

// --- core.RefResolver implementation ---

func (a *LocalArchive) ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error) {
	return a.resolveRef(repoID, ref)
}
