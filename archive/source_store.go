package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveSource persists a source to the database.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveSource(source *core.Source) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}

	now := time.Now().UTC()

	configJSON, err := json.Marshal(source.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal source config: %w", err)
	}

	filterIDsJSON, err := json.Marshal(source.FilterIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal source filter IDs: %w", err)
	}

	var rateLimitJSON []byte
	if source.RateLimit != nil {
		rateLimitJSON, err = json.Marshal(source.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal source rate limit: %w", err)
		}
	}

	if source.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, source.RepoID, "source")
		if err != nil {
			return fmt.Errorf("failed to generate source short ID: %w", err)
		}
		source.ShortID = core.FormatShortID(core.PrefixSource, seq)
		source.Version = 1
		source.CreatedAt = now
		source.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO sources (id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, source.ID.String(), source.ShortID, source.RepoID.String(), uuidPtrString(source.ConnectionID), source.Name,
			string(source.Connector), string(configJSON), string(filterIDsJSON),
			string(source.FilterChainMode), stringOrNil(rateLimitJSON), string(source.Status),
			source.CreatedBy.String(), source.Version,
			source.CreatedAt.Format(time.RFC3339), source.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert source: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	source.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE sources
		SET name = ?, connection_id = ?, connector = ?, config = ?, filter_ids = ?, filter_chain_mode = ?, rate_limit = ?, status = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, source.Name, uuidPtrString(source.ConnectionID), string(source.Connector), string(configJSON), string(filterIDsJSON),
		string(source.FilterChainMode), stringOrNil(rateLimitJSON), string(source.Status),
		source.UpdatedAt.Format(time.RFC3339),
		source.ID.String(), source.Version)
	if err != nil {
		return fmt.Errorf("failed to update source: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "source",
			EntityID:   source.ID,
			Expected:   source.Version,
		}
	}

	source.Version++
	return nil
}

// getSource retrieves a source by UUID.
func (a *LocalArchive) getSource(sourceID uuid.UUID) (*core.Source, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at
		FROM sources WHERE id = ?
	`, sourceID.String())

	return scanSourceFromSingleRow(row)
}

// deleteSource deletes a source by UUID.
func (a *LocalArchive) deleteSource(sourceID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM sources WHERE id = ?`, sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("source", sourceID.String())
	}

	return nil
}

// listSources returns paginated sources for a repository.
func (a *LocalArchive) listSources(repoID uuid.UUID, opts core.SourceListOptions) ([]core.Source, int, error) {
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}

	// Filter by connector type if specified.
	if opts.Connector != "" {
		where += ` AND connector = ?`
		args = append(args, string(opts.Connector))
	}

	// Filter by status if specified.
	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, string(opts.Status))
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM sources `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count sources: %w", err)
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
		`SELECT id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at
		 FROM sources %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sources: %w", err)
	}
	defer rows.Close()

	var sources []core.Source
	for rows.Next() {
		s, err := scanSourceFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		sources = append(sources, *s)
	}

	return sources, total, nil
}

// scanSourceFromSingleRow scans a source from a *sql.Row (single-row query).
func scanSourceFromSingleRow(row *sql.Row) (*core.Source, error) {
	var source core.Source
	var idStr, repoIDStr, createdByStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, statusStr, createdAt, updatedAt string
	var connectionIDStr, rateLimitStr sql.NullString

	err := row.Scan(&idStr, &source.ShortID, &repoIDStr, &connectionIDStr, &source.Name, &connectorStr,
		&configStr, &filterIDsStr, &filterChainModeStr, &rateLimitStr, &statusStr,
		&createdByStr, &source.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("source", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan source: %w", err)
	}

	return parseSourceFields(&source, idStr, repoIDStr, createdByStr, connectionIDStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr.String, statusStr, createdAt, updatedAt)
}

// scanSourceFromRow scans a source from a *sql.Rows iterator.
func scanSourceFromRow(rows *sql.Rows) (*core.Source, error) {
	var source core.Source
	var idStr, repoIDStr, createdByStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, statusStr, createdAt, updatedAt string
	var connectionIDStr, rateLimitStr sql.NullString

	err := rows.Scan(&idStr, &source.ShortID, &repoIDStr, &connectionIDStr, &source.Name, &connectorStr,
		&configStr, &filterIDsStr, &filterChainModeStr, &rateLimitStr, &statusStr,
		&createdByStr, &source.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan source row: %w", err)
	}

	return parseSourceFields(&source, idStr, repoIDStr, createdByStr, connectionIDStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr.String, statusStr, createdAt, updatedAt)
}

// parseSourceFields populates a Source's parsed fields from raw scanned strings.
func parseSourceFields(source *core.Source, idStr, repoIDStr, createdByStr string, connectionIDStr sql.NullString, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr, statusStr, createdAt, updatedAt string) (*core.Source, error) {
	var err error

	source.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source ID: %w", err)
	}
	source.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source repo ID: %w", err)
	}
	source.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source created_by: %w", err)
	}

	if connectionIDStr.Valid && connectionIDStr.String != "" {
		cid, err := uuid.Parse(connectionIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse source connection_id: %w", err)
		}
		source.ConnectionID = &cid
	}

	source.Connector = core.ConnectorType(connectorStr)
	source.FilterChainMode = core.FilterMode(filterChainModeStr)
	source.Status = core.ResourceStatus(statusStr)

	if configStr != "" {
		if err := json.Unmarshal([]byte(configStr), &source.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source config: %w", err)
		}
	}

	if filterIDsStr != "" {
		var filterIDStrings []string
		if err := json.Unmarshal([]byte(filterIDsStr), &filterIDStrings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source filter IDs: %w", err)
		}
		source.FilterIDs = make([]uuid.UUID, len(filterIDStrings))
		for i, idStr := range filterIDStrings {
			source.FilterIDs[i], err = uuid.Parse(idStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse filter ID %s: %w", idStr, err)
			}
		}
	}

	if rateLimitStr != "" {
		var rateLimit core.RateLimit
		if err := json.Unmarshal([]byte(rateLimitStr), &rateLimit); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source rate limit: %w", err)
		}
		source.RateLimit = &rateLimit
	}

	source.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source created_at: %w", err)
	}
	source.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source updated_at: %w", err)
	}

	return source, nil
}

// stringOrNil converts a byte slice to a string pointer (nil if empty).
func stringOrNil(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
