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
// Metadata is stored in SQLite, and node content is stored as files.
type LocalArchive struct {
	path string
	db   *sql.DB
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

	dbPath := filepath.Join(upspeakDir, "metadata.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	archive := &LocalArchive{path: path, db: db}

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

// --- core.Archive interface implementation ---

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

// Sequence interface methods.

func (a *LocalArchive) NextRepoSequence(repoID uuid.UUID, entity string) (int, error) {
	return nextRepoSequence(a.db, repoID, entity)
}

func (a *LocalArchive) NextUserSequence(ownerID uuid.UUID, entity string) (int, error) {
	return nextUserSequence(a.db, ownerID, entity)
}

func (a *LocalArchive) NextGlobalSequence(entity string) (int, error) {
	return nextGlobalSequence(a.db, entity)
}

// --- Node operations ---

func (a *LocalArchive) SaveNode(node *core.Node) error {
	return a.saveNode(node)
}

func (a *LocalArchive) SaveBatchNodes(repoID uuid.UUID, nodes []*core.Node) error {
	return a.saveBatchNodes(repoID, nodes)
}

func (a *LocalArchive) GetNode(nodeID uuid.UUID) (*core.Node, error) {
	return a.getNode(nodeID)
}

func (a *LocalArchive) DeleteNode(nodeID uuid.UUID) error {
	return a.deleteNode(nodeID)
}

func (a *LocalArchive) ListNodes(repoID uuid.UUID, nodeType string, opts core.ListOptions) ([]core.Node, int, error) {
	return a.listNodes(repoID, nodeType, opts)
}

func (a *LocalArchive) GetNodeEdges(nodeID uuid.UUID, opts core.EdgeQueryOptions) ([]core.Edge, int, error) {
	return a.getNodeEdges(nodeID, opts)
}

func (a *LocalArchive) GetNodeAnnotations(nodeID uuid.UUID, opts core.AnnotationQueryOptions) ([]core.Annotation, int, error) {
	return a.getNodeAnnotations(nodeID, opts)
}

// --- Edge operations ---

func (a *LocalArchive) SaveEdge(edge *core.Edge) error {
	return a.saveEdge(edge)
}

func (a *LocalArchive) SaveBatchEdges(repoID uuid.UUID, edges []*core.Edge) error {
	return a.saveBatchEdges(repoID, edges)
}

func (a *LocalArchive) GetEdge(edgeID uuid.UUID) (*core.Edge, error) {
	return a.getEdge(edgeID)
}

func (a *LocalArchive) DeleteEdge(edgeID uuid.UUID) error {
	return a.deleteEdge(edgeID)
}

func (a *LocalArchive) ListEdges(repoID uuid.UUID, source, target, edgeType string, opts core.ListOptions) ([]core.Edge, int, error) {
	return a.listEdges(repoID, source, target, edgeType, opts)
}

// --- Thread operations ---

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

// --- Annotation operations ---

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

// --- Entity ref resolution ---

func (a *LocalArchive) ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error) {
	return a.resolveRef(repoID, ref)
}
