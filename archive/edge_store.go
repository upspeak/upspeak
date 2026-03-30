package archive

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveEdge persists an edge to the database.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveEdge(edge *core.Edge) error {
	if edge == nil {
		return fmt.Errorf("edge is nil")
	}

	now := time.Now().UTC()

	if edge.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, edge.RepoID, "edge")
		if err != nil {
			return fmt.Errorf("failed to generate edge short ID: %w", err)
		}
		edge.ShortID = core.FormatShortID(core.PrefixEdge, seq)
		edge.Version = 1
		edge.CreatedAt = now
		edge.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO edges (id, short_id, repo_id, type, source, target, label, weight, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, edge.ID.String(), edge.ShortID, edge.RepoID.String(), edge.Type,
			edge.Source.String(), edge.Target.String(), edge.Label, edge.Weight,
			edge.CreatedBy.String(), edge.Version,
			edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert edge: %w", err)
		}
		return nil
	}

	// Update: optimistic concurrency check.
	edge.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE edges
		SET type = ?, source = ?, target = ?, label = ?, weight = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, edge.Type, edge.Source.String(), edge.Target.String(), edge.Label, edge.Weight,
		edge.UpdatedAt.Format(time.RFC3339),
		edge.ID.String(), edge.Version)
	if err != nil {
		return fmt.Errorf("failed to update edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "edge",
			EntityID:   edge.ID,
			Expected:   edge.Version,
		}
	}

	edge.Version++
	return nil
}

// saveBatchEdges persists multiple edges in a single atomic transaction.
// All edges must belong to the specified repository.
func (a *LocalArchive) saveBatchEdges(repoID uuid.UUID, edges []*core.Edge) error {
	if len(edges) == 0 {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	for _, edge := range edges {
		if edge == nil {
			return fmt.Errorf("nil edge in batch")
		}

		seq, err := nextRepoSequence(tx, repoID, "edge")
		if err != nil {
			return fmt.Errorf("failed to generate edge short ID: %w", err)
		}
		edge.ShortID = core.FormatShortID(core.PrefixEdge, seq)
		edge.RepoID = repoID
		edge.Version = 1
		edge.CreatedAt = now
		edge.UpdatedAt = now

		_, err = tx.Exec(`
			INSERT INTO edges (id, short_id, repo_id, type, source, target, label, weight, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, edge.ID.String(), edge.ShortID, edge.RepoID.String(), edge.Type,
			edge.Source.String(), edge.Target.String(), edge.Label, edge.Weight,
			edge.CreatedBy.String(), edge.Version,
			edge.CreatedAt.Format(time.RFC3339), edge.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert edge in batch: %w", err)
		}
	}

	return tx.Commit()
}

// getEdge retrieves an edge by UUID.
func (a *LocalArchive) getEdge(edgeID uuid.UUID) (*core.Edge, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, type, source, target, label, weight, created_by, version, created_at, updated_at
		FROM edges WHERE id = ?
	`, edgeID.String())
	return scanEdgeFromSingleRow(row)
}

// deleteEdge deletes an edge by UUID.
func (a *LocalArchive) deleteEdge(edgeID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM edges WHERE id = ?`, edgeID.String())
	if err != nil {
		return fmt.Errorf("failed to delete edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("edge", edgeID.String())
	}

	return nil
}

// listEdges returns paginated edges for a repository with optional source, target, and type filters.
func (a *LocalArchive) listEdges(repoID uuid.UUID, source, target, edgeType string, opts core.ListOptions) ([]core.Edge, int, error) {
	// Build WHERE clause.
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}

	if source != "" {
		where += ` AND source = ?`
		args = append(args, source)
	}
	if target != "" {
		where += ` AND target = ?`
		args = append(args, target)
	}
	if edgeType != "" {
		where += ` AND type = ?`
		args = append(args, edgeType)
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM edges `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count edges: %w", err)
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
		return nil, 0, fmt.Errorf("failed to list edges: %w", err)
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

// scanEdgeFromSingleRow scans an edge from a *sql.Row (single-row query).
func scanEdgeFromSingleRow(row *sql.Row) (*core.Edge, error) {
	var edge core.Edge
	var idStr, repoIDStr, sourceStr, targetStr, createdByStr, createdAt, updatedAt string

	err := row.Scan(&idStr, &edge.ShortID, &repoIDStr, &edge.Type,
		&sourceStr, &targetStr, &edge.Label, &edge.Weight,
		&createdByStr, &edge.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("edge", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan edge: %w", err)
	}

	return parseEdgeFields(&edge, idStr, repoIDStr, sourceStr, targetStr, createdByStr, createdAt, updatedAt)
}

// scanEdgeFromRow scans an edge from a *sql.Rows iterator.
func scanEdgeFromRow(rows *sql.Rows) (*core.Edge, error) {
	var edge core.Edge
	var idStr, repoIDStr, sourceStr, targetStr, createdByStr, createdAt, updatedAt string

	err := rows.Scan(&idStr, &edge.ShortID, &repoIDStr, &edge.Type,
		&sourceStr, &targetStr, &edge.Label, &edge.Weight,
		&createdByStr, &edge.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan edge row: %w", err)
	}

	return parseEdgeFields(&edge, idStr, repoIDStr, sourceStr, targetStr, createdByStr, createdAt, updatedAt)
}

// parseEdgeFields populates an Edge's parsed fields from raw scanned strings.
func parseEdgeFields(edge *core.Edge, idStr, repoIDStr, sourceStr, targetStr, createdByStr, createdAt, updatedAt string) (*core.Edge, error) {
	var err error

	edge.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge ID: %w", err)
	}
	edge.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge repo ID: %w", err)
	}
	edge.Source, err = uuid.Parse(sourceStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge source: %w", err)
	}
	edge.Target, err = uuid.Parse(targetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge target: %w", err)
	}
	edge.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge created_by: %w", err)
	}

	edge.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge created_at: %w", err)
	}
	edge.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse edge updated_at: %w", err)
	}

	return edge, nil
}
