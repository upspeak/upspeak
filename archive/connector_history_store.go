package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// recordCollectionAttempt persists a collection record to the database.
// History records are append-only; ID is generated if not set.
func (a *LocalArchive) recordCollectionAttempt(record *core.CollectionRecord) error {
	if record == nil {
		return fmt.Errorf("collection record is nil")
	}

	// Generate ID if not set.
	if record.ID == uuid.Nil {
		record.ID = core.NewID()
	}

	detailsJSON, err := json.Marshal(record.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal collection record details: %w", err)
	}

	_, err = a.db.Exec(`
		INSERT INTO collection_history (id, source_id, at, result, details, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, record.ID.String(), record.SourceID.String(), record.At.Format(time.RFC3339),
		record.Result, string(detailsJSON), record.ErrorMessage, record.DurationMs)
	if err != nil {
		return fmt.Errorf("failed to insert collection record: %w", err)
	}

	return nil
}

// recordPublishAttempt persists a publish record to the database.
// History records are append-only; ID is generated if not set.
func (a *LocalArchive) recordPublishAttempt(record *core.PublishRecord) error {
	if record == nil {
		return fmt.Errorf("publish record is nil")
	}

	// Generate ID if not set.
	if record.ID == uuid.Nil {
		record.ID = core.NewID()
	}

	detailsJSON, err := json.Marshal(record.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal publish record details: %w", err)
	}

	_, err = a.db.Exec(`
		INSERT INTO publish_history (id, sink_id, at, result, details, error_message, duration_ms, external_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID.String(), record.SinkID.String(), record.At.Format(time.RFC3339),
		record.Result, string(detailsJSON), record.ErrorMessage, record.DurationMs, record.ExternalURL)
	if err != nil {
		return fmt.Errorf("failed to insert publish record: %w", err)
	}

	return nil
}

// getSourceHistory returns paginated collection history for a source.
func (a *LocalArchive) getSourceHistory(sourceID uuid.UUID, opts core.ListOptions) ([]core.CollectionRecord, int, error) {
	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM collection_history WHERE source_id = ?`, sourceID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count collection history: %w", err)
	}

	// Validate sort field.
	sortBy := "at"
	switch opts.SortBy {
	case "at":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, source_id, at, result, details, error_message, duration_ms
		 FROM collection_history WHERE source_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		sortBy, order,
	)

	rows, err := a.db.Query(query, sourceID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list collection history: %w", err)
	}
	defer rows.Close()

	var records []core.CollectionRecord
	for rows.Next() {
		r, err := scanCollectionRecordFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, *r)
	}

	return records, total, nil
}

// getSinkHistory returns paginated publish history for a sink.
func (a *LocalArchive) getSinkHistory(sinkID uuid.UUID, opts core.ListOptions) ([]core.PublishRecord, int, error) {
	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM publish_history WHERE sink_id = ?`, sinkID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count publish history: %w", err)
	}

	// Validate sort field.
	sortBy := "at"
	switch opts.SortBy {
	case "at":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, sink_id, at, result, details, error_message, duration_ms, external_url
		 FROM publish_history WHERE sink_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		sortBy, order,
	)

	rows, err := a.db.Query(query, sinkID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list publish history: %w", err)
	}
	defer rows.Close()

	var records []core.PublishRecord
	for rows.Next() {
		r, err := scanPublishRecordFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, *r)
	}

	return records, total, nil
}

// scanCollectionRecordFromRow scans a collection record from a *sql.Rows iterator.
func scanCollectionRecordFromRow(rows *sql.Rows) (*core.CollectionRecord, error) {
	var record core.CollectionRecord
	var idStr, sourceIDStr, atStr, detailsStr string

	err := rows.Scan(&idStr, &sourceIDStr, &atStr, &record.Result, &detailsStr,
		&record.ErrorMessage, &record.DurationMs)
	if err != nil {
		return nil, fmt.Errorf("failed to scan collection record row: %w", err)
	}

	record.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection record ID: %w", err)
	}
	record.SourceID, err = uuid.Parse(sourceIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection record source ID: %w", err)
	}

	record.At, err = time.Parse(time.RFC3339, atStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection record at: %w", err)
	}

	if detailsStr != "" {
		if err := json.Unmarshal([]byte(detailsStr), &record.Details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal collection record details: %w", err)
		}
	}

	return &record, nil
}

// scanPublishRecordFromRow scans a publish record from a *sql.Rows iterator.
func scanPublishRecordFromRow(rows *sql.Rows) (*core.PublishRecord, error) {
	var record core.PublishRecord
	var idStr, sinkIDStr, atStr, detailsStr string

	err := rows.Scan(&idStr, &sinkIDStr, &atStr, &record.Result, &detailsStr,
		&record.ErrorMessage, &record.DurationMs, &record.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to scan publish record row: %w", err)
	}

	record.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse publish record ID: %w", err)
	}
	record.SinkID, err = uuid.Parse(sinkIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse publish record sink ID: %w", err)
	}

	record.At, err = time.Parse(time.RFC3339, atStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse publish record at: %w", err)
	}

	if detailsStr != "" {
		if err := json.Unmarshal([]byte(detailsStr), &record.Details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal publish record details: %w", err)
		}
	}

	return &record, nil
}
