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

// --- Stub implementations for entities not yet fully implemented (Phase 2) ---

func (a *LocalArchive) SaveNode(node *core.Node) error {
	return fmt.Errorf("not implemented: SaveNode (Phase 2)")
}

func (a *LocalArchive) GetNode(nodeID uuid.UUID) (*core.Node, error) {
	return nil, fmt.Errorf("not implemented: GetNode (Phase 2)")
}

func (a *LocalArchive) DeleteNode(nodeID uuid.UUID) error {
	return fmt.Errorf("not implemented: DeleteNode (Phase 2)")
}

func (a *LocalArchive) ListNodes(repoID uuid.UUID, opts core.ListOptions) ([]core.Node, int, error) {
	return nil, 0, fmt.Errorf("not implemented: ListNodes (Phase 2)")
}

func (a *LocalArchive) SaveEdge(edge *core.Edge) error {
	return fmt.Errorf("not implemented: SaveEdge (Phase 2)")
}

func (a *LocalArchive) GetEdge(edgeID uuid.UUID) (*core.Edge, error) {
	return nil, fmt.Errorf("not implemented: GetEdge (Phase 2)")
}

func (a *LocalArchive) DeleteEdge(edgeID uuid.UUID) error {
	return fmt.Errorf("not implemented: DeleteEdge (Phase 2)")
}

func (a *LocalArchive) ListEdges(repoID uuid.UUID, opts core.ListOptions) ([]core.Edge, int, error) {
	return nil, 0, fmt.Errorf("not implemented: ListEdges (Phase 2)")
}

func (a *LocalArchive) SaveThread(thread *core.Thread) error {
	return fmt.Errorf("not implemented: SaveThread (Phase 2)")
}

func (a *LocalArchive) GetThread(threadID uuid.UUID) (*core.Thread, error) {
	return nil, fmt.Errorf("not implemented: GetThread (Phase 2)")
}

func (a *LocalArchive) DeleteThread(threadID uuid.UUID) error {
	return fmt.Errorf("not implemented: DeleteThread (Phase 2)")
}

func (a *LocalArchive) ListThreads(repoID uuid.UUID, opts core.ListOptions) ([]core.Thread, int, error) {
	return nil, 0, fmt.Errorf("not implemented: ListThreads (Phase 2)")
}

func (a *LocalArchive) SaveAnnotation(annotation *core.Annotation) error {
	return fmt.Errorf("not implemented: SaveAnnotation (Phase 2)")
}

func (a *LocalArchive) GetAnnotation(annotationID uuid.UUID) (*core.Annotation, error) {
	return nil, fmt.Errorf("not implemented: GetAnnotation (Phase 2)")
}

func (a *LocalArchive) DeleteAnnotation(annotationID uuid.UUID) error {
	return fmt.Errorf("not implemented: DeleteAnnotation (Phase 2)")
}

func (a *LocalArchive) ListAnnotations(repoID uuid.UUID, opts core.ListOptions) ([]core.Annotation, int, error) {
	return nil, 0, fmt.Errorf("not implemented: ListAnnotations (Phase 2)")
}
