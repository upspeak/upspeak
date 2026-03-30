package archive

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveAnnotation persists an annotation to the database.
// If Version == 0, this is a create: generates an annotation short ID, saves the
// embedded node (via saveNode), saves the embedded edge (via saveEdge), and inserts
// the annotation row.
// If Version > 0, this is an update: updates the annotation motivation and the
// embedded node content.
func (a *LocalArchive) saveAnnotation(annotation *core.Annotation) error {
	if annotation == nil {
		return fmt.Errorf("annotation is nil")
	}

	now := time.Now().UTC()

	if annotation.Version == 0 {
		// Create: generate short ID, save embedded node and edge, insert annotation.
		seq, err := nextRepoSequence(a.db, annotation.RepoID, "annotation")
		if err != nil {
			return fmt.Errorf("failed to generate annotation short ID: %w", err)
		}
		annotation.ShortID = core.FormatShortID(core.PrefixAnnotation, seq)
		annotation.Version = 1
		annotation.CreatedAt = now
		annotation.UpdatedAt = now

		// Save the embedded node (annotation content).
		if err := a.saveNode(&annotation.Node); err != nil {
			return fmt.Errorf("failed to save annotation node: %w", err)
		}

		// Save the embedded edge (links annotation content to target).
		if err := a.saveEdge(&annotation.Edge); err != nil {
			return fmt.Errorf("failed to save annotation edge: %w", err)
		}

		// Insert annotation row.
		_, err = a.db.Exec(`
			INSERT INTO annotations (id, short_id, repo_id, node_id, edge_id, motivation, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, annotation.ID.String(), annotation.ShortID, annotation.RepoID.String(),
			annotation.Node.ID.String(), annotation.Edge.ID.String(),
			annotation.Motivation, annotation.CreatedBy.String(),
			annotation.Version, annotation.CreatedAt.Format(time.RFC3339),
			annotation.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert annotation: %w", err)
		}
		return nil
	}

	// Update: update annotation motivation and bump version, then update the embedded node.
	annotation.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE annotations
		SET motivation = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, annotation.Motivation, annotation.UpdatedAt.Format(time.RFC3339),
		annotation.ID.String(), annotation.Version)
	if err != nil {
		return fmt.Errorf("failed to update annotation: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "annotation",
			EntityID:   annotation.ID,
			Expected:   annotation.Version,
		}
	}

	annotation.Version++

	// Update the embedded node content.
	if err := a.saveNode(&annotation.Node); err != nil {
		return fmt.Errorf("failed to update annotation node: %w", err)
	}

	return nil
}

// getAnnotation retrieves an annotation by UUID, including its embedded node and edge.
func (a *LocalArchive) getAnnotation(annotationID uuid.UUID) (*core.Annotation, error) {
	var annotation core.Annotation
	var idStr, repoIDStr, nodeIDStr, edgeIDStr, createdByStr, createdAt, updatedAt string

	err := a.db.QueryRow(`
		SELECT id, short_id, repo_id, node_id, edge_id, motivation, created_by, version, created_at, updated_at
		FROM annotations WHERE id = ?
	`, annotationID.String()).Scan(&idStr, &annotation.ShortID, &repoIDStr,
		&nodeIDStr, &edgeIDStr, &annotation.Motivation, &createdByStr,
		&annotation.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("annotation", annotationID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan annotation: %w", err)
	}

	annotation.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation ID: %w", err)
	}
	annotation.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation repo ID: %w", err)
	}
	annotation.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation created_by: %w", err)
	}
	annotation.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation created_at: %w", err)
	}
	annotation.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation updated_at: %w", err)
	}

	// Get embedded node.
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation node ID: %w", err)
	}
	node, err := a.getNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation node: %w", err)
	}
	annotation.Node = *node

	// Get embedded edge.
	edgeID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation edge ID: %w", err)
	}
	edge, err := a.getEdge(edgeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation edge: %w", err)
	}
	annotation.Edge = *edge

	return &annotation, nil
}

// deleteAnnotation deletes an annotation, its embedded edge, and its embedded node.
func (a *LocalArchive) deleteAnnotation(annotationID uuid.UUID) error {
	// Get the annotation to find the embedded node and edge IDs.
	var nodeIDStr, edgeIDStr string
	err := a.db.QueryRow(`SELECT node_id, edge_id FROM annotations WHERE id = ?`, annotationID.String()).
		Scan(&nodeIDStr, &edgeIDStr)
	if err == sql.ErrNoRows {
		return core.NewErrorNotFound("annotation", annotationID.String())
	}
	if err != nil {
		return fmt.Errorf("failed to find annotation for deletion: %w", err)
	}

	// Delete annotation row first (has foreign keys to node and edge).
	_, err = a.db.Exec(`DELETE FROM annotations WHERE id = ?`, annotationID.String())
	if err != nil {
		return fmt.Errorf("failed to delete annotation: %w", err)
	}

	// Delete embedded edge.
	edgeID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse edge ID for deletion: %w", err)
	}
	if err := a.deleteEdge(edgeID); err != nil {
		return fmt.Errorf("failed to delete annotation edge: %w", err)
	}

	// Delete embedded node.
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		return fmt.Errorf("failed to parse node ID for deletion: %w", err)
	}
	if err := a.deleteNode(nodeID); err != nil {
		return fmt.Errorf("failed to delete annotation node: %w", err)
	}

	return nil
}

// listAnnotations returns paginated annotations for a repository, including
// embedded nodes and edges.
func (a *LocalArchive) listAnnotations(repoID uuid.UUID, opts core.ListOptions) ([]core.Annotation, int, error) {
	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM annotations WHERE repo_id = ?`, repoID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count annotations: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "motivation":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, node_id, edge_id, motivation, created_by, version, created_at, updated_at
		 FROM annotations WHERE repo_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		sortBy, order,
	)

	rows, err := a.db.Query(query, repoID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list annotations: %w", err)
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

// scanAnnotationRowAndHydrate scans an annotation row and hydrates the embedded
// node and edge by querying them separately.
func (a *LocalArchive) scanAnnotationRowAndHydrate(rows *sql.Rows) (*core.Annotation, error) {
	var annotation core.Annotation
	var idStr, repoIDStr, nodeIDStr, edgeIDStr, createdByStr, createdAt, updatedAt string

	err := rows.Scan(&idStr, &annotation.ShortID, &repoIDStr,
		&nodeIDStr, &edgeIDStr, &annotation.Motivation, &createdByStr,
		&annotation.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan annotation row: %w", err)
	}

	annotation.ID, _ = uuid.Parse(idStr)
	annotation.RepoID, _ = uuid.Parse(repoIDStr)
	annotation.CreatedBy, _ = uuid.Parse(createdByStr)
	annotation.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	annotation.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	// Hydrate embedded node.
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation node ID: %w", err)
	}
	node, err := a.getNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation node: %w", err)
	}
	annotation.Node = *node

	// Hydrate embedded edge.
	edgeID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse annotation edge ID: %w", err)
	}
	edge, err := a.getEdge(edgeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get annotation edge: %w", err)
	}
	annotation.Edge = *edge

	return &annotation, nil
}
