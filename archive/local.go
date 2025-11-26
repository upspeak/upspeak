package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
	"github.com/upspeak/upspeak/core"
)

// LocalArchive implements the core.Archive interface using local file system storage.
// Metadata is stored in SQLite, and node content is stored as files.
type LocalArchive struct {
	path string  // Root path for the archive
	db   *sql.DB // SQLite database for metadata
}

// NewLocalArchive creates a new LocalArchive at the specified path.
// Directory structure:
//
//	{path}/
//	  .upspeak/
//	    metadata.db
//	  {node-id}  (node content files)
//	  {node-id}  (node content files)
//	  ...
func NewLocalArchive(path string) (*LocalArchive, error) {
	// Create directory structure
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archive directory: %w", err)
	}

	upspeakDir := filepath.Join(path, ".upspeak")
	if err := os.MkdirAll(upspeakDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .upspeak directory: %w", err)
	}

	// Open SQLite database
	dbPath := filepath.Join(upspeakDir, "metadata.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	archive := &LocalArchive{
		path: path,
		db:   db,
	}

	// Initialise database schema
	if err := archive.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialise schema: %w", err)
	}

	return archive, nil
}

// initSchema creates the database tables if they don't exist.
func (a *LocalArchive) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		subject TEXT NOT NULL,
		content_type TEXT NOT NULL,
		metadata TEXT,
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS edges (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		target TEXT NOT NULL,
		label TEXT NOT NULL,
		weight REAL NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (source) REFERENCES nodes(id),
		FOREIGN KEY (target) REFERENCES nodes(id)
	);

	CREATE TABLE IF NOT EXISTS threads (
		node_id TEXT PRIMARY KEY,
		metadata TEXT,
		FOREIGN KEY (node_id) REFERENCES nodes(id)
	);

	CREATE TABLE IF NOT EXISTS thread_edges (
		thread_node_id TEXT NOT NULL,
		edge_id TEXT NOT NULL,
		PRIMARY KEY (thread_node_id, edge_id),
		FOREIGN KEY (thread_node_id) REFERENCES threads(node_id),
		FOREIGN KEY (edge_id) REFERENCES edges(id)
	);

	CREATE TABLE IF NOT EXISTS annotations (
		node_id TEXT PRIMARY KEY,
		edge_id TEXT NOT NULL,
		motivation TEXT NOT NULL,
		FOREIGN KEY (node_id) REFERENCES nodes(id),
		FOREIGN KEY (edge_id) REFERENCES edges(id)
	);

	-- Indices for performance
	CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source);
	CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target);
	CREATE INDEX IF NOT EXISTS idx_thread_edges_thread_node_id ON thread_edges(thread_node_id);
	CREATE INDEX IF NOT EXISTS idx_thread_edges_edge_id ON thread_edges(edge_id);
	CREATE INDEX IF NOT EXISTS idx_annotations_edge_id ON annotations(edge_id);
	`

	_, err := a.db.Exec(schema)
	return err
}

// SaveNode saves a node to the archive.
func (a *LocalArchive) SaveNode(node *core.Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}

	// Marshal metadata
	metadataJSON, err := json.Marshal(node.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Save node metadata to database
	_, err = a.db.Exec(`
		INSERT OR REPLACE INTO nodes (id, type, subject, content_type, metadata, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, node.ID.String(), node.Type, node.Subject, node.ContentType, string(metadataJSON),
		node.CreatedBy.String(), node.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return fmt.Errorf("failed to save node metadata: %w", err)
	}

	// Save node content to file
	if len(node.Body) > 0 {
		contentPath := filepath.Join(a.path, node.ID.String())
		if err := os.WriteFile(contentPath, node.Body, 0644); err != nil {
			return fmt.Errorf("failed to save node content: %w", err)
		}
	}

	return nil
}

// GetNode retrieves a node from the archive.
func (a *LocalArchive) GetNode(nodeID xid.ID) (*core.Node, error) {
	var node core.Node
	var metadataJSON string
	var createdAt string
	var createdByStr string

	err := a.db.QueryRow(`
		SELECT id, type, subject, content_type, metadata, created_by, created_at
		FROM nodes WHERE id = ?
	`, nodeID.String()).Scan(&node.ID, &node.Type, &node.Subject, &node.ContentType,
		&metadataJSON, &createdByStr, &createdAt)

	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("node", nodeID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query node: %w", err)
	}

	// Unmarshal metadata
	if err := json.Unmarshal([]byte(metadataJSON), &node.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Parse created_by
	createdBy, err := xid.FromString(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_by: %w", err)
	}
	node.CreatedBy = createdBy

	// Parse created_at
	createdAtTime, err := time.Parse("2006-01-02T15:04:05Z07:00", createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	node.CreatedAt = createdAtTime

	// Load node content from file
	contentPath := filepath.Join(a.path, nodeID.String())
	if _, err := os.Stat(contentPath); err == nil {
		body, err := os.ReadFile(contentPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read node content: %w", err)
		}
		node.Body = body
	}

	return &node, nil
}

// DeleteNode deletes a node from the archive.
func (a *LocalArchive) DeleteNode(nodeID xid.ID) error {
	// Delete node metadata from database
	_, err := a.db.Exec("DELETE FROM nodes WHERE id = ?", nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete node metadata: %w", err)
	}

	// Delete node content file
	contentPath := filepath.Join(a.path, nodeID.String())
	if err := os.Remove(contentPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete node content: %w", err)
	}

	return nil
}

// SaveEdge saves an edge to the archive.
func (a *LocalArchive) SaveEdge(edge *core.Edge) error {
	if edge == nil {
		return fmt.Errorf("edge is nil")
	}

	_, err := a.db.Exec(`
		INSERT OR REPLACE INTO edges (id, type, source, target, label, weight, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, edge.ID.String(), edge.Type, edge.Source.String(), edge.Target.String(),
		edge.Label, edge.Weight, edge.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	if err != nil {
		return fmt.Errorf("failed to save edge: %w", err)
	}

	return nil
}

// GetEdge retrieves an edge from the archive.
func (a *LocalArchive) GetEdge(edgeID xid.ID) (*core.Edge, error) {
	var edge core.Edge
	var sourceStr, targetStr, createdAt string

	err := a.db.QueryRow(`
		SELECT id, type, source, target, label, weight, created_at
		FROM edges WHERE id = ?
	`, edgeID.String()).Scan(&edge.ID, &edge.Type, &sourceStr, &targetStr,
		&edge.Label, &edge.Weight, &createdAt)

	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("edge", edgeID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query edge: %w", err)
	}

	// Parse source and target
	source, err := xid.FromString(sourceStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source: %w", err)
	}
	edge.Source = source

	target, err := xid.FromString(targetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target: %w", err)
	}
	edge.Target = target

	return &edge, nil
}

// DeleteEdge deletes an edge from the archive.
func (a *LocalArchive) DeleteEdge(edgeID xid.ID) error {
	_, err := a.db.Exec("DELETE FROM edges WHERE id = ?", edgeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}
	return nil
}

// GetEdgesByNode retrieves all edges where the given node is either source or target.
func (a *LocalArchive) GetEdgesByNode(nodeID xid.ID) ([]core.Edge, error) {
	rows, err := a.db.Query(`
		SELECT id, type, source, target, label, weight, created_at
		FROM edges WHERE source = ? OR target = ?
	`, nodeID.String(), nodeID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	var edges []core.Edge
	for rows.Next() {
		var edge core.Edge
		var sourceStr, targetStr, createdAt string

		if err := rows.Scan(&edge.ID, &edge.Type, &sourceStr, &targetStr,
			&edge.Label, &edge.Weight, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan edge: %w", err)
		}

		// Parse source and target
		source, err := xid.FromString(sourceStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse source: %w", err)
		}
		edge.Source = source

		target, err := xid.FromString(targetStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse target: %w", err)
		}
		edge.Target = target

		edges = append(edges, edge)
	}

	return edges, nil
}

// DeleteEdgesByNode deletes all edges where the given node is either source or target.
func (a *LocalArchive) DeleteEdgesByNode(nodeID xid.ID) error {
	_, err := a.db.Exec("DELETE FROM edges WHERE source = ? OR target = ?",
		nodeID.String(), nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete edges by node: %w", err)
	}
	return nil
}

// SaveThread saves a thread to the archive.
func (a *LocalArchive) SaveThread(thread *core.Thread) error {
	if thread == nil {
		return fmt.Errorf("thread is nil")
	}

	// Create directory for the thread
	threadDir := filepath.Join(a.path, thread.Node.ID.String())
	if err := os.MkdirAll(threadDir, 0755); err != nil {
		return fmt.Errorf("failed to create thread directory: %w", err)
	}

	// Start transaction
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Save the root node
	if err := a.SaveNode(&thread.Node); err != nil {
		return fmt.Errorf("failed to save thread node: %w", err)
	}

	// Save thread metadata
	metadataJSON, err := json.Marshal(thread.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal thread metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO threads (node_id, metadata)
		VALUES (?, ?)
	`, thread.Node.ID.String(), string(metadataJSON))
	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	// Delete existing thread edges
	_, err = tx.Exec("DELETE FROM thread_edges WHERE thread_node_id = ?", thread.Node.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete existing thread edges: %w", err)
	}

	// Save edges and collect node IDs from edges
	nodeIDs := make(map[xid.ID]bool)
	nodeIDs[thread.Node.ID] = true // Include root node

	for _, edge := range thread.Edges {
		if err := a.SaveEdge(&edge); err != nil {
			return fmt.Errorf("failed to save thread edge: %w", err)
		}

		// Link edge to thread
		_, err = tx.Exec(`
			INSERT INTO thread_edges (thread_node_id, edge_id)
			VALUES (?, ?)
		`, thread.Node.ID.String(), edge.ID.String())
		if err != nil {
			return fmt.Errorf("failed to link edge to thread: %w", err)
		}

		// Track nodes referenced by edges
		nodeIDs[edge.Source] = true
		nodeIDs[edge.Target] = true
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Move or copy node content files into the thread directory
	for nodeID := range nodeIDs {
		nodePath := filepath.Join(a.path, nodeID.String())
		threadNodePath := filepath.Join(threadDir, nodeID.String())

		// Check if node content exists
		if _, err := os.Stat(nodePath); err == nil {
			// Read node content
			content, err := os.ReadFile(nodePath)
			if err != nil {
				return fmt.Errorf("failed to read node content: %w", err)
			}

			// Write to thread directory
			if err := os.WriteFile(threadNodePath, content, 0644); err != nil {
				return fmt.Errorf("failed to write node to thread directory: %w", err)
			}

			// Remove original if it's not the root node (root stays at top level)
			if nodeID != thread.Node.ID {
				os.Remove(nodePath) // Ignore errors - file might not exist
			}
		}
	}

	return nil
}

// GetThread retrieves a thread from the archive.
func (a *LocalArchive) GetThread(nodeID xid.ID) (*core.Thread, error) {
	var thread core.Thread
	var metadataJSON string

	// Get thread metadata
	err := a.db.QueryRow(`
		SELECT node_id, metadata FROM threads WHERE node_id = ?
	`, nodeID.String()).Scan(&nodeID, &metadataJSON)

	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("thread", nodeID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query thread: %w", err)
	}

	// Unmarshal metadata
	if err := json.Unmarshal([]byte(metadataJSON), &thread.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread metadata: %w", err)
	}

	// Get the root node
	node, err := a.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread node: %w", err)
	}

	// Load root node content from thread directory if available
	threadDir := filepath.Join(a.path, nodeID.String())
	threadNodePath := filepath.Join(threadDir, nodeID.String())
	if _, err := os.Stat(threadNodePath); err == nil {
		body, err := os.ReadFile(threadNodePath)
		if err == nil {
			node.Body = body
		}
	}

	thread.Node = *node

	// Get thread edges
	rows, err := a.db.Query(`
		SELECT edge_id FROM thread_edges WHERE thread_node_id = ?
	`, nodeID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query thread edges: %w", err)
	}
	defer rows.Close()

	var edges []core.Edge
	for rows.Next() {
		var edgeIDStr string
		if err := rows.Scan(&edgeIDStr); err != nil {
			return nil, fmt.Errorf("failed to scan edge ID: %w", err)
		}

		edgeID, err := xid.FromString(edgeIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse edge ID: %w", err)
		}

		edge, err := a.GetEdge(edgeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get edge: %w", err)
		}
		edges = append(edges, *edge)
	}

	thread.Edges = edges
	return &thread, nil
}

// DeleteThread deletes a thread from the archive.
func (a *LocalArchive) DeleteThread(nodeID xid.ID) error {
	// Start transaction
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete thread edges links
	_, err = tx.Exec("DELETE FROM thread_edges WHERE thread_node_id = ?", nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete thread edges: %w", err)
	}

	// Delete thread metadata
	_, err = tx.Exec("DELETE FROM threads WHERE node_id = ?", nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Delete the thread directory and all its contents
	threadDir := filepath.Join(a.path, nodeID.String())
	if err := os.RemoveAll(threadDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete thread directory: %w", err)
	}

	// Delete the root node
	return a.DeleteNode(nodeID)
}

// SaveAnnotation saves an annotation to the archive.
func (a *LocalArchive) SaveAnnotation(annotation *core.Annotation) error {
	if annotation == nil {
		return fmt.Errorf("annotation is nil")
	}

	// Save the annotation node
	if err := a.SaveNode(&annotation.Node); err != nil {
		return fmt.Errorf("failed to save annotation node: %w", err)
	}

	// Save the edge
	if err := a.SaveEdge(&annotation.Edge); err != nil {
		return fmt.Errorf("failed to save annotation edge: %w", err)
	}

	// Save annotation metadata
	_, err := a.db.Exec(`
		INSERT OR REPLACE INTO annotations (node_id, edge_id, motivation)
		VALUES (?, ?, ?)
	`, annotation.Node.ID.String(), annotation.Edge.ID.String(), annotation.Motivation)

	if err != nil {
		return fmt.Errorf("failed to save annotation: %w", err)
	}

	return nil
}

// GetAnnotation retrieves an annotation from the archive.
func (a *LocalArchive) GetAnnotation(nodeID xid.ID) (*core.Annotation, error) {
	var annotation core.Annotation
	var edgeIDStr string

	// Get annotation metadata
	err := a.db.QueryRow(`
		SELECT node_id, edge_id, motivation FROM annotations WHERE node_id = ?
	`, nodeID.String()).Scan(&nodeID, &edgeIDStr, &annotation.Motivation)

	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("annotation", nodeID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query annotation: %w", err)
	}

	// Get the annotation node
	node, err := a.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation node: %w", err)
	}
	annotation.Node = *node

	// Get the edge
	edgeID, err := xid.FromString(edgeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge ID: %w", err)
	}

	edge, err := a.GetEdge(edgeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation edge: %w", err)
	}
	annotation.Edge = *edge

	return &annotation, nil
}

// DeleteAnnotation deletes an annotation from the archive.
func (a *LocalArchive) DeleteAnnotation(nodeID xid.ID) error {
	// Get annotation to find the edge ID
	annotation, err := a.GetAnnotation(nodeID)
	if err != nil {
		return err
	}

	// Delete annotation metadata
	_, err = a.db.Exec("DELETE FROM annotations WHERE node_id = ?", nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}

	// Delete the edge
	if err := a.DeleteEdge(annotation.Edge.ID); err != nil {
		return fmt.Errorf("failed to delete annotation edge: %w", err)
	}

	// Delete the node
	return a.DeleteNode(nodeID)
}

// Close closes the database connection.
func (a *LocalArchive) Close() error {
	return a.db.Close()
}
