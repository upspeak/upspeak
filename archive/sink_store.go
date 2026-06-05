package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveSink persists a sink to the database.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveSink(sink *core.Sink) error {
	if sink == nil {
		return fmt.Errorf("sink is nil")
	}

	now := time.Now().UTC()

	configJSON, err := json.Marshal(sink.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal sink config: %w", err)
	}

	filterIDsJSON, err := json.Marshal(sink.FilterIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal sink filter IDs: %w", err)
	}

	var rateLimitJSON []byte
	if sink.RateLimit != nil {
		rateLimitJSON, err = json.Marshal(sink.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal sink rate limit: %w", err)
		}
	}

	if sink.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, sink.RepoID, "sink")
		if err != nil {
			return fmt.Errorf("failed to generate sink short ID: %w", err)
		}
		sink.ShortID = core.FormatShortID(core.PrefixSink, seq)
		sink.Version = 1
		sink.CreatedAt = now
		sink.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO sinks (id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sink.ID.String(), sink.ShortID, sink.RepoID.String(), uuidPtrString(sink.ConnectionID), sink.Name,
			string(sink.Connector), string(configJSON), string(filterIDsJSON),
			string(sink.FilterChainMode), stringOrNil(rateLimitJSON), string(sink.Status),
			sink.CreatedBy.String(), sink.Version,
			sink.CreatedAt.Format(time.RFC3339), sink.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert sink: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	sink.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE sinks
		SET name = ?, connection_id = ?, connector = ?, config = ?, filter_ids = ?, filter_chain_mode = ?, rate_limit = ?, status = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, sink.Name, uuidPtrString(sink.ConnectionID), string(sink.Connector), string(configJSON), string(filterIDsJSON),
		string(sink.FilterChainMode), stringOrNil(rateLimitJSON), string(sink.Status),
		sink.UpdatedAt.Format(time.RFC3339),
		sink.ID.String(), sink.Version)
	if err != nil {
		return fmt.Errorf("failed to update sink: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "sink",
			EntityID:   sink.ID,
			Expected:   sink.Version,
		}
	}

	sink.Version++
	return nil
}

// getSink retrieves a sink by UUID.
func (a *LocalArchive) getSink(sinkID uuid.UUID) (*core.Sink, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at
		FROM sinks WHERE id = ?
	`, sinkID.String())

	return scanSinkFromSingleRow(row)
}

// deleteSink deletes a sink by UUID.
func (a *LocalArchive) deleteSink(sinkID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM sinks WHERE id = ?`, sinkID.String())
	if err != nil {
		return fmt.Errorf("failed to delete sink: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("sink", sinkID.String())
	}

	return nil
}

// listSinks returns paginated sinks for a repository.
func (a *LocalArchive) listSinks(repoID uuid.UUID, opts core.SinkListOptions) ([]core.Sink, int, error) {
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
	err := a.db.QueryRow(`SELECT COUNT(*) FROM sinks `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count sinks: %w", err)
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
		 FROM sinks %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sinks: %w", err)
	}
	defer rows.Close()

	var sinks []core.Sink
	for rows.Next() {
		s, err := scanSinkFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		sinks = append(sinks, *s)
	}

	return sinks, total, nil
}

// scanSinkFromSingleRow scans a sink from a *sql.Row (single-row query).
func scanSinkFromSingleRow(row *sql.Row) (*core.Sink, error) {
	var sink core.Sink
	var idStr, repoIDStr, createdByStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, statusStr, createdAt, updatedAt string
	var connectionIDStr, rateLimitStr sql.NullString

	err := row.Scan(&idStr, &sink.ShortID, &repoIDStr, &connectionIDStr, &sink.Name, &connectorStr,
		&configStr, &filterIDsStr, &filterChainModeStr, &rateLimitStr, &statusStr,
		&createdByStr, &sink.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("sink", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan sink: %w", err)
	}

	return parseSinkFields(&sink, idStr, repoIDStr, createdByStr, connectionIDStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr.String, statusStr, createdAt, updatedAt)
}

// scanSinkFromRow scans a sink from a *sql.Rows iterator.
func scanSinkFromRow(rows *sql.Rows) (*core.Sink, error) {
	var sink core.Sink
	var idStr, repoIDStr, createdByStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, statusStr, createdAt, updatedAt string
	var connectionIDStr, rateLimitStr sql.NullString

	err := rows.Scan(&idStr, &sink.ShortID, &repoIDStr, &connectionIDStr, &sink.Name, &connectorStr,
		&configStr, &filterIDsStr, &filterChainModeStr, &rateLimitStr, &statusStr,
		&createdByStr, &sink.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan sink row: %w", err)
	}

	return parseSinkFields(&sink, idStr, repoIDStr, createdByStr, connectionIDStr, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr.String, statusStr, createdAt, updatedAt)
}

// parseSinkFields populates a Sink's parsed fields from raw scanned strings.
func parseSinkFields(sink *core.Sink, idStr, repoIDStr, createdByStr string, connectionIDStr sql.NullString, connectorStr, configStr, filterIDsStr, filterChainModeStr, rateLimitStr, statusStr, createdAt, updatedAt string) (*core.Sink, error) {
	var err error

	sink.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink ID: %w", err)
	}
	sink.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink repo ID: %w", err)
	}
	sink.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink created_by: %w", err)
	}

	if connectionIDStr.Valid && connectionIDStr.String != "" {
		cid, err := uuid.Parse(connectionIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sink connection_id: %w", err)
		}
		sink.ConnectionID = &cid
	}

	sink.Connector = core.ConnectorType(connectorStr)
	sink.FilterChainMode = core.FilterMode(filterChainModeStr)
	sink.Status = core.ResourceStatus(statusStr)

	if configStr != "" {
		if err := json.Unmarshal([]byte(configStr), &sink.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sink config: %w", err)
		}
	}

	if filterIDsStr != "" {
		var filterIDStrings []string
		if err := json.Unmarshal([]byte(filterIDsStr), &filterIDStrings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sink filter IDs: %w", err)
		}
		sink.FilterIDs = make([]uuid.UUID, len(filterIDStrings))
		for i, idStr := range filterIDStrings {
			sink.FilterIDs[i], err = uuid.Parse(idStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse filter ID %s: %w", idStr, err)
			}
		}
	}

	if rateLimitStr != "" {
		var rateLimit core.RateLimit
		if err := json.Unmarshal([]byte(rateLimitStr), &rateLimit); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sink rate limit: %w", err)
		}
		sink.RateLimit = &rateLimit
	}

	sink.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink created_at: %w", err)
	}
	sink.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sink updated_at: %w", err)
	}

	return sink, nil
}
