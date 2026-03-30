package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveNode persists a node to the database and writes body content to a file.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveNode(node *core.Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}

	now := time.Now().UTC()

	metadataJSON, err := json.Marshal(node.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal node metadata: %w", err)
	}

	if node.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, node.RepoID, "node")
		if err != nil {
			return fmt.Errorf("failed to generate node short ID: %w", err)
		}
		node.ShortID = core.FormatShortID(core.PrefixNode, seq)
		node.Version = 1
		node.CreatedAt = now
		node.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO nodes (id, short_id, repo_id, type, subject, content_type, metadata, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, node.ID.String(), node.ShortID, node.RepoID.String(), node.Type, node.Subject,
			node.ContentType, string(metadataJSON), node.CreatedBy.String(),
			node.Version, node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert node: %w", err)
		}

		// Write body content to file.
		if err := a.writeNodeBody(node.ID, node.Body); err != nil {
			return fmt.Errorf("failed to write node body: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	node.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE nodes
		SET type = ?, subject = ?, content_type = ?, metadata = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, node.Type, node.Subject, node.ContentType, string(metadataJSON),
		node.UpdatedAt.Format(time.RFC3339),
		node.ID.String(), node.Version)
	if err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "node",
			EntityID:   node.ID,
			Expected:   node.Version,
		}
	}

	node.Version++

	// Write body content to file.
	if err := a.writeNodeBody(node.ID, node.Body); err != nil {
		return fmt.Errorf("failed to write node body: %w", err)
	}

	return nil
}

// saveBatchNodes persists multiple nodes in a single atomic transaction.
// Short IDs are generated for each node within the transaction. Body content
// is written to files after the transaction commits successfully.
func (a *LocalArchive) saveBatchNodes(nodes []*core.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	for _, node := range nodes {
		if node == nil {
			return fmt.Errorf("nil node in batch")
		}

		metadataJSON, err := json.Marshal(node.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal node metadata: %w", err)
		}

		// Generate short ID within the transaction to avoid locking conflicts.
		seq, err := nextRepoSequence(tx, node.RepoID, "node")
		if err != nil {
			return fmt.Errorf("failed to generate node short ID: %w", err)
		}
		node.ShortID = core.FormatShortID(core.PrefixNode, seq)
		node.Version = 1
		node.CreatedAt = now
		node.UpdatedAt = now

		_, err = tx.Exec(`
			INSERT INTO nodes (id, short_id, repo_id, type, subject, content_type, metadata, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, node.ID.String(), node.ShortID, node.RepoID.String(), node.Type, node.Subject,
			node.ContentType, string(metadataJSON), node.CreatedBy.String(),
			node.Version, node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert node in batch: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Write body content files after successful commit.
	for _, node := range nodes {
		if err := a.writeNodeBody(node.ID, node.Body); err != nil {
			return fmt.Errorf("failed to write node body for %s: %w", node.ID, err)
		}
	}

	return nil
}

// getNode retrieves a node by UUID, including body content from file.
func (a *LocalArchive) getNode(nodeID uuid.UUID) (*core.Node, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, type, subject, content_type, metadata, created_by, version, created_at, updated_at
		FROM nodes WHERE id = ?
	`, nodeID.String())

	node, err := scanNodeFromSingleRow(row)
	if err != nil {
		return nil, err
	}

	// Read body content from file.
	body, err := a.readNodeBody(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to read node body: %w", err)
	}
	node.Body = body

	return node, nil
}

// deleteNode deletes a node by UUID, including its body content file.
func (a *LocalArchive) deleteNode(nodeID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM nodes WHERE id = ?`, nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("node", nodeID.String())
	}

	// Remove body content file (ignore error if file doesn't exist).
	_ = os.Remove(a.contentPath(nodeID))

	return nil
}

// listNodes returns paginated nodes for a repository, optionally filtered by type.
// Body content is NOT loaded for list operations (use GetNode for full content).
func (a *LocalArchive) listNodes(repoID uuid.UUID, opts core.NodeListOptions) ([]core.Node, int, error) {
	// Build WHERE clause.
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}
	if opts.Type != "" {
		where += ` AND type = ?`
		args = append(args, opts.Type)
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM nodes `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count nodes: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "type", "subject":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, type, subject, content_type, metadata, created_by, version, created_at, updated_at
		 FROM nodes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list nodes: %w", err)
	}
	defer rows.Close()

	var nodes []core.Node
	for rows.Next() {
		node, err := scanNodeFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, *node)
	}

	return nodes, total, nil
}

// getNodeEdges returns edges connected to a node, filtered by direction and type.
func (a *LocalArchive) getNodeEdges(nodeID uuid.UUID, opts core.EdgeQueryOptions) ([]core.Edge, int, error) {
	// Build WHERE clause based on direction.
	var where string
	var args []any

	switch opts.Direction {
	case "outgoing":
		where = `WHERE source = ?`
		args = []any{nodeID.String()}
	case "incoming":
		where = `WHERE target = ?`
		args = []any{nodeID.String()}
	default: // "both" or empty
		where = `WHERE (source = ? OR target = ?)`
		args = []any{nodeID.String(), nodeID.String()}
	}

	if opts.Type != "" {
		where += ` AND type = ?`
		args = append(args, opts.Type)
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM edges `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count node edges: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "type", "weight":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, type, source, target, label, weight, created_by, version, created_at, updated_at
		 FROM edges %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list node edges: %w", err)
	}
	defer rows.Close()

	var edges []core.Edge
	for rows.Next() {
		edge, err := scanEdgeFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		edges = append(edges, *edge)
	}

	return edges, total, nil
}

// getNodeAnnotations returns annotations targeting a node, optionally filtered by motivation.
func (a *LocalArchive) getNodeAnnotations(nodeID uuid.UUID, opts core.AnnotationQueryOptions) ([]core.Annotation, int, error) {
	// Annotations target a node via their embedded edge. We join annotations
	// with edges to find annotations whose edge targets the given node.
	where := `WHERE e.target = ?`
	args := []any{nodeID.String()}

	if opts.Motivation != "" {
		where += ` AND a.motivation = ?`
		args = append(args, opts.Motivation)
	}

	// Count total.
	var total int
	err := a.db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM annotations a JOIN edges e ON a.edge_id = e.id %s`, where,
	), args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count node annotations: %w", err)
	}

	// Validate sort field.
	sortBy := "a.created_at"
	switch opts.SortBy {
	case "created_at":
		sortBy = "a.created_at"
	case "updated_at":
		sortBy = "a.updated_at"
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT a.id, a.short_id, a.repo_id, a.node_id, a.edge_id, a.motivation, a.created_by, a.version, a.created_at, a.updated_at
		 FROM annotations a JOIN edges e ON a.edge_id = e.id %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list node annotations: %w", err)
	}
	defer rows.Close()

	var annotations []core.Annotation
	for rows.Next() {
		ann, err := a.scanAnnotationRowAndHydrate(rows)
		if err != nil {
			return nil, 0, err
		}
		annotations = append(annotations, *ann)
	}

	return annotations, total, nil
}

// writeNodeBody writes node body content to a file. If body is nil or empty,
// the content file is removed (if it exists).
func (a *LocalArchive) writeNodeBody(nodeID uuid.UUID, body json.RawMessage) error {
	path := a.contentPath(nodeID)

	if len(body) == 0 || string(body) == "null" {
		// Remove content file if body is empty.
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove content file: %w", err)
		}
		return nil
	}

	return os.WriteFile(path, []byte(body), 0644)
}

// readNodeBody reads node body content from a file. Returns nil if no content file exists.
func (a *LocalArchive) readNodeBody(nodeID uuid.UUID) (json.RawMessage, error) {
	path := a.contentPath(nodeID)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read content file: %w", err)
	}

	return json.RawMessage(data), nil
}

// scanNodeFromSingleRow scans a node from a *sql.Row (single-row query).
// Does NOT load body content — caller must read from file separately.
func scanNodeFromSingleRow(row *sql.Row) (*core.Node, error) {
	var node core.Node
	var idStr, repoIDStr, createdByStr, createdAt, updatedAt string
	var metadataStr sql.NullString

	err := row.Scan(&idStr, &node.ShortID, &repoIDStr, &node.Type, &node.Subject,
		&node.ContentType, &metadataStr, &createdByStr,
		&node.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("node", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan node: %w", err)
	}

	return parseNodeFields(&node, idStr, repoIDStr, createdByStr, metadataStr, createdAt, updatedAt)
}

// scanNodeFromRow scans a node from a *sql.Rows iterator.
// Does NOT load body content — caller must read from file separately.
func scanNodeFromRow(rows *sql.Rows) (*core.Node, error) {
	var node core.Node
	var idStr, repoIDStr, createdByStr, createdAt, updatedAt string
	var metadataStr sql.NullString

	err := rows.Scan(&idStr, &node.ShortID, &repoIDStr, &node.Type, &node.Subject,
		&node.ContentType, &metadataStr, &createdByStr,
		&node.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan node row: %w", err)
	}

	return parseNodeFields(&node, idStr, repoIDStr, createdByStr, metadataStr, createdAt, updatedAt)
}

// parseNodeFields populates a Node's parsed fields from raw scanned strings.
func parseNodeFields(node *core.Node, idStr, repoIDStr, createdByStr string, metadataStr sql.NullString, createdAt, updatedAt string) (*core.Node, error) {
	var err error

	node.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse node ID: %w", err)
	}
	node.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse node repo ID: %w", err)
	}
	node.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse node created_by: %w", err)
	}

	if metadataStr.Valid && metadataStr.String != "" {
		if err := json.Unmarshal([]byte(metadataStr.String), &node.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal node metadata: %w", err)
		}
	}

	node.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse node created_at: %w", err)
	}
	node.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse node updated_at: %w", err)
	}

	return node, nil
}
