package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveFilter persists a filter to the database.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveFilter(filter *core.Filter) error {
	if filter == nil {
		return fmt.Errorf("filter is nil")
	}

	now := time.Now().UTC()

	conditionsJSON, err := json.Marshal(filter.Conditions)
	if err != nil {
		return fmt.Errorf("failed to marshal filter conditions: %w", err)
	}

	if filter.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, filter.RepoID, "filter")
		if err != nil {
			return fmt.Errorf("failed to generate filter short ID: %w", err)
		}
		filter.ShortID = core.FormatShortID(core.PrefixFilter, seq)
		filter.Version = 1
		filter.CreatedAt = now
		filter.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO filters (id, short_id, repo_id, name, description, mode, conditions, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, filter.ID.String(), filter.ShortID, filter.RepoID.String(), filter.Name,
			filter.Description, string(filter.Mode), string(conditionsJSON),
			filter.CreatedBy.String(), filter.Version,
			filter.CreatedAt.Format(time.RFC3339), filter.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert filter: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	filter.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE filters
		SET name = ?, description = ?, mode = ?, conditions = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, filter.Name, filter.Description, string(filter.Mode), string(conditionsJSON),
		filter.UpdatedAt.Format(time.RFC3339),
		filter.ID.String(), filter.Version)
	if err != nil {
		return fmt.Errorf("failed to update filter: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "filter",
			EntityID:   filter.ID,
			Expected:   filter.Version,
		}
	}

	filter.Version++
	return nil
}

// getFilter retrieves a filter by UUID.
func (a *LocalArchive) getFilter(filterID uuid.UUID) (*core.Filter, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, name, description, mode, conditions, created_by, version, created_at, updated_at
		FROM filters WHERE id = ?
	`, filterID.String())

	return scanFilterFromSingleRow(row)
}

// deleteFilter deletes a filter by UUID.
func (a *LocalArchive) deleteFilter(filterID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM filters WHERE id = ?`, filterID.String())
	if err != nil {
		return fmt.Errorf("failed to delete filter: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("filter", filterID.String())
	}

	return nil
}

// listFilters returns paginated filters for a repository.
func (a *LocalArchive) listFilters(repoID uuid.UUID, opts core.FilterListOptions) ([]core.Filter, int, error) {
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM filters `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count filters: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "name":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, name, description, mode, conditions, created_by, version, created_at, updated_at
		 FROM filters %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list filters: %w", err)
	}
	defer rows.Close()

	var filters []core.Filter
	for rows.Next() {
		f, err := scanFilterFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		filters = append(filters, *f)
	}

	return filters, total, nil
}

// getFilterReferences checks for entities (sources, sinks, rules) that reference
// a filter. Returns an empty slice if no references exist.
func (a *LocalArchive) getFilterReferences(filterID uuid.UUID) ([]core.FilterReference, error) {
	var refs []core.FilterReference
	fID := filterID.String()

	// Check sources that reference this filter.
	rows, err := a.db.Query(`SELECT id, name FROM sources WHERE filter_ids LIKE ?`, "%"+fID+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to check source filter references: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("failed to scan source reference: %w", err)
		}
		refs = append(refs, core.FilterReference{EntityType: "source", EntityID: id, EntityName: name})
	}

	// Check sinks that reference this filter.
	sinkRows, err := a.db.Query(`SELECT id, name FROM sinks WHERE filter_ids LIKE ?`, "%"+fID+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to check sink filter references: %w", err)
	}
	defer sinkRows.Close()
	for sinkRows.Next() {
		var id, name string
		if err := sinkRows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("failed to scan sink reference: %w", err)
		}
		refs = append(refs, core.FilterReference{EntityType: "sink", EntityID: id, EntityName: name})
	}

	return refs, nil
}

// scanFilterFromSingleRow scans a filter from a *sql.Row (single-row query).
func scanFilterFromSingleRow(row *sql.Row) (*core.Filter, error) {
	var filter core.Filter
	var idStr, repoIDStr, createdByStr, modeStr, conditionsStr, createdAt, updatedAt string
	var description string

	err := row.Scan(&idStr, &filter.ShortID, &repoIDStr, &filter.Name, &description,
		&modeStr, &conditionsStr, &createdByStr,
		&filter.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("filter", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan filter: %w", err)
	}

	filter.Description = description

	return parseFilterFields(&filter, idStr, repoIDStr, createdByStr, modeStr, conditionsStr, createdAt, updatedAt)
}

// scanFilterFromRow scans a filter from a *sql.Rows iterator.
func scanFilterFromRow(rows *sql.Rows) (*core.Filter, error) {
	var filter core.Filter
	var idStr, repoIDStr, createdByStr, modeStr, conditionsStr, createdAt, updatedAt string
	var description string

	err := rows.Scan(&idStr, &filter.ShortID, &repoIDStr, &filter.Name, &description,
		&modeStr, &conditionsStr, &createdByStr,
		&filter.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan filter row: %w", err)
	}

	filter.Description = description

	return parseFilterFields(&filter, idStr, repoIDStr, createdByStr, modeStr, conditionsStr, createdAt, updatedAt)
}

// parseFilterFields populates a Filter's parsed fields from raw scanned strings.
func parseFilterFields(filter *core.Filter, idStr, repoIDStr, createdByStr, modeStr, conditionsStr, createdAt, updatedAt string) (*core.Filter, error) {
	var err error

	filter.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter ID: %w", err)
	}
	filter.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter repo ID: %w", err)
	}
	filter.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter created_by: %w", err)
	}

	filter.Mode = core.FilterMode(modeStr)

	if conditionsStr != "" {
		if err := json.Unmarshal([]byte(conditionsStr), &filter.Conditions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal filter conditions: %w", err)
		}
	}

	filter.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter created_at: %w", err)
	}
	filter.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse filter updated_at: %w", err)
	}

	return filter, nil
}
