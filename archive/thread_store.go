package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveThread persists a thread to the database.
// If Version == 0, this is a create: generates a thread short ID, saves the root
// node (via saveNode), inserts the thread row, saves edges, and links thread_edges.
// If Version > 0, this is an update: updates thread metadata only (root node and
// edges are updated separately through their own save methods).
func (a *LocalArchive) saveThread(thread *core.Thread) error {
	if thread == nil {
		return fmt.Errorf("thread is nil")
	}

	now := time.Now().UTC()

	metadataJSON, err := json.Marshal(thread.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal thread metadata: %w", err)
	}

	if thread.Version == 0 {
		// Create: generate short ID, save root node, insert thread, save edges, link edges.
		seq, err := nextRepoSequence(a.db, thread.RepoID, "thread")
		if err != nil {
			return fmt.Errorf("failed to generate thread short ID: %w", err)
		}
		thread.ShortID = core.FormatShortID(core.PrefixThread, seq)
		thread.Version = 1
		thread.CreatedAt = now
		thread.UpdatedAt = now

		// Save the root node.
		if err := a.saveNode(&thread.Node); err != nil {
			return fmt.Errorf("failed to save thread root node: %w", err)
		}

		// Insert thread row.
		_, err = a.db.Exec(`
			INSERT INTO threads (id, short_id, repo_id, node_id, metadata, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, thread.ID.String(), thread.ShortID, thread.RepoID.String(),
			thread.Node.ID.String(), string(metadataJSON), thread.CreatedBy.String(),
			thread.Version, thread.CreatedAt.Format(time.RFC3339), thread.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert thread: %w", err)
		}

		// Save edges and link them to the thread.
		for i := range thread.Edges {
			if err := a.saveEdge(&thread.Edges[i]); err != nil {
				return fmt.Errorf("failed to save thread edge: %w", err)
			}
			_, err = a.db.Exec(`
				INSERT INTO thread_edges (thread_id, edge_id) VALUES (?, ?)
			`, thread.ID.String(), thread.Edges[i].ID.String())
			if err != nil {
				return fmt.Errorf("failed to link thread edge: %w", err)
			}
		}

		return nil
	}

	// Update: optimistic concurrency check on thread metadata only.
	thread.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE threads
		SET metadata = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, string(metadataJSON), thread.UpdatedAt.Format(time.RFC3339),
		thread.ID.String(), thread.Version)
	if err != nil {
		return fmt.Errorf("failed to update thread: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "thread",
			EntityID:   thread.ID,
			Expected:   thread.Version,
		}
	}

	thread.Version++
	return nil
}

// getThread retrieves a thread by UUID, including its root node and edges.
func (a *LocalArchive) getThread(threadID uuid.UUID) (*core.Thread, error) {
	// Get thread row.
	var thread core.Thread
	var idStr, repoIDStr, nodeIDStr, createdByStr, createdAt, updatedAt string
	var metadataStr sql.NullString

	err := a.db.QueryRow(`
		SELECT id, short_id, repo_id, node_id, metadata, created_by, version, created_at, updated_at
		FROM threads WHERE id = ?
	`, threadID.String()).Scan(&idStr, &thread.ShortID, &repoIDStr, &nodeIDStr,
		&metadataStr, &createdByStr, &thread.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("thread", threadID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan thread: %w", err)
	}

	thread.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread ID: %w", err)
	}
	thread.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread repo ID: %w", err)
	}
	thread.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread created_by: %w", err)
	}

	if metadataStr.Valid && metadataStr.String != "" {
		if err := json.Unmarshal([]byte(metadataStr.String), &thread.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal thread metadata: %w", err)
		}
	}

	thread.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread created_at: %w", err)
	}
	thread.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread updated_at: %w", err)
	}

	// Get root node.
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse thread node ID: %w", err)
	}
	rootNode, err := a.getNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread root node: %w", err)
	}
	thread.Node = *rootNode

	// Get thread edges.
	edges, err := a.getThreadEdges(threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread edges: %w", err)
	}
	thread.Edges = edges

	return &thread, nil
}

// deleteThread deletes a thread and its thread_edge links, plus the root node.
// Does NOT delete contained nodes (except the root node).
func (a *LocalArchive) deleteThread(threadID uuid.UUID) error {
	// First, get the thread to find the root node ID and linked edges.
	var nodeIDStr string
	err := a.db.QueryRow(`SELECT node_id FROM threads WHERE id = ?`, threadID.String()).Scan(&nodeIDStr)
	if err == sql.ErrNoRows {
		return core.NewErrorNotFound("thread", threadID.String())
	}
	if err != nil {
		return fmt.Errorf("failed to find thread for deletion: %w", err)
	}

	// Get edge IDs linked to this thread so we can delete them.
	edgeRows, err := a.db.Query(`SELECT edge_id FROM thread_edges WHERE thread_id = ?`, threadID.String())
	if err != nil {
		return fmt.Errorf("failed to query thread edges: %w", err)
	}
	var edgeIDs []string
	for edgeRows.Next() {
		var edgeID string
		if err := edgeRows.Scan(&edgeID); err != nil {
			edgeRows.Close()
			return fmt.Errorf("failed to scan thread edge ID: %w", err)
		}
		edgeIDs = append(edgeIDs, edgeID)
	}
	edgeRows.Close()

	// Delete thread_edges links.
	_, err = a.db.Exec(`DELETE FROM thread_edges WHERE thread_id = ?`, threadID.String())
	if err != nil {
		return fmt.Errorf("failed to delete thread edges: %w", err)
	}

	// Delete the thread row.
	_, err = a.db.Exec(`DELETE FROM threads WHERE id = ?`, threadID.String())
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	// Delete linked edges.
	for _, edgeIDStr := range edgeIDs {
		edgeID, err := uuid.Parse(edgeIDStr)
		if err != nil {
			return fmt.Errorf("failed to parse edge ID for deletion: %w", err)
		}
		if err := a.deleteEdge(edgeID); err != nil {
			return fmt.Errorf("failed to delete thread edge: %w", err)
		}
	}

	// Delete root node.
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse root node ID for deletion: %w", err)
	}
	if err := a.deleteNode(nodeID); err != nil {
		return fmt.Errorf("failed to delete thread root node: %w", err)
	}

	return nil
}

// listThreads returns paginated threads for a repository (metadata only, without
// full structure).
func (a *LocalArchive) listThreads(repoID uuid.UUID, opts core.ListOptions) ([]core.Thread, int, error) {
	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM threads WHERE repo_id = ?`, repoID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count threads: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, node_id, metadata, created_by, version, created_at, updated_at
		 FROM threads WHERE repo_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		sortBy, order,
	)

	rows, err := a.db.Query(query, repoID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list threads: %w", err)
	}
	defer rows.Close()

	var threads []core.Thread
	for rows.Next() {
		thread, err := scanThreadFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		threads = append(threads, *thread)
	}

	return threads, total, nil
}

// addNodeToThread creates an edge from the thread's root node to the specified
// node and links it in the thread_edges table.
func (a *LocalArchive) addNodeToThread(threadID, nodeID uuid.UUID, edgeType string) error {
	// Get the thread's root node ID and repo ID.
	var rootNodeIDStr, repoIDStr, createdByStr string
	err := a.db.QueryRow(`SELECT node_id, repo_id, created_by FROM threads WHERE id = ?`, threadID.String()).
		Scan(&rootNodeIDStr, &repoIDStr, &createdByStr)
	if err == sql.ErrNoRows {
		return core.NewErrorNotFound("thread", threadID.String())
	}
	if err != nil {
		return fmt.Errorf("failed to find thread: %w", err)
	}

	rootNodeID, err := uuid.Parse(rootNodeIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse root node ID: %w", err)
	}
	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse repo ID: %w", err)
	}
	createdBy, err := uuid.Parse(createdByStr)
	if err != nil {
		return fmt.Errorf("failed to parse created_by: %w", err)
	}

	// Create edge from root node to target node.
	edge := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repoID,
		Type:      edgeType,
		Source:    rootNodeID,
		Target:    nodeID,
		Weight:    1.0,
		CreatedBy: createdBy,
	}

	if err := a.saveEdge(edge); err != nil {
		return fmt.Errorf("failed to create edge for thread node: %w", err)
	}

	// Link edge to thread.
	_, err = a.db.Exec(`INSERT INTO thread_edges (thread_id, edge_id) VALUES (?, ?)`,
		threadID.String(), edge.ID.String())
	if err != nil {
		return fmt.Errorf("failed to link edge to thread: %w", err)
	}

	return nil
}

// removeNodeFromThread removes the thread_edge link and the corresponding edge
// for a given node within a thread.
func (a *LocalArchive) removeNodeFromThread(threadID, nodeID uuid.UUID) error {
	// Find the edge that connects the thread's root node to the given node.
	var rootNodeIDStr string
	err := a.db.QueryRow(`SELECT node_id FROM threads WHERE id = ?`, threadID.String()).
		Scan(&rootNodeIDStr)
	if err == sql.ErrNoRows {
		return core.NewErrorNotFound("thread", threadID.String())
	}
	if err != nil {
		return fmt.Errorf("failed to find thread: %w", err)
	}

	// Find the edge linked to this thread that targets the given node.
	var edgeIDStr string
	err = a.db.QueryRow(`
		SELECT e.id FROM edges e
		JOIN thread_edges te ON te.edge_id = e.id
		WHERE te.thread_id = ? AND e.source = ? AND e.target = ?
	`, threadID.String(), rootNodeIDStr, nodeID.String()).Scan(&edgeIDStr)
	if err == sql.ErrNoRows {
		return core.NewErrorNotFound("thread edge", nodeID.String())
	}
	if err != nil {
		return fmt.Errorf("failed to find thread edge: %w", err)
	}

	// Delete the thread_edge link.
	_, err = a.db.Exec(`DELETE FROM thread_edges WHERE thread_id = ? AND edge_id = ?`,
		threadID.String(), edgeIDStr)
	if err != nil {
		return fmt.Errorf("failed to delete thread_edge link: %w", err)
	}

	// Delete the edge itself.
	edgeID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse edge ID: %w", err)
	}
	if err := a.deleteEdge(edgeID); err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}

	return nil
}

// getThreadEdges retrieves all edges linked to a thread via the thread_edges table.
func (a *LocalArchive) getThreadEdges(threadID uuid.UUID) ([]core.Edge, error) {
	rows, err := a.db.Query(`
		SELECT e.id, e.short_id, e.repo_id, e.type, e.source, e.target, e.label, e.weight, e.created_by, e.version, e.created_at, e.updated_at
		FROM edges e
		JOIN thread_edges te ON te.edge_id = e.id
		WHERE te.thread_id = ?
	`, threadID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query thread edges: %w", err)
	}
	defer rows.Close()

	var edges []core.Edge
	for rows.Next() {
		edge, err := scanEdgeFromRow(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, *edge)
	}

	return edges, nil
}

// scanThreadFromRow scans a thread from a *sql.Rows iterator (metadata only,
// without root node or edges).
func scanThreadFromRow(rows *sql.Rows) (*core.Thread, error) {
	var thread core.Thread
	var idStr, repoIDStr, nodeIDStr, createdByStr, createdAt, updatedAt string
	var metadataStr sql.NullString

	err := rows.Scan(&idStr, &thread.ShortID, &repoIDStr, &nodeIDStr,
		&metadataStr, &createdByStr, &thread.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan thread row: %w", err)
	}

	thread.ID, _ = uuid.Parse(idStr)
	thread.RepoID, _ = uuid.Parse(repoIDStr)
	thread.CreatedBy, _ = uuid.Parse(createdByStr)
	thread.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	thread.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	// Set the root node ID so callers know which node to fetch if needed.
	nodeID, _ := uuid.Parse(nodeIDStr)
	thread.Node = core.Node{ID: nodeID}

	if metadataStr.Valid && metadataStr.String != "" {
		_ = json.Unmarshal([]byte(metadataStr.String), &thread.Metadata)
	}

	return &thread, nil
}
