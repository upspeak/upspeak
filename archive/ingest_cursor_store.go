package archive

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveIngestCursor upserts the per-source ingestion cursor. The cursor payload
// is opaque to the archive layer — it is stored as-is and returned verbatim.
func (a *LocalArchive) saveIngestCursor(c *core.IngestCursor) error {
	if c == nil {
		return fmt.Errorf("ingest cursor is nil")
	}
	now := time.Now().UTC()
	c.UpdatedAt = now
	_, err := a.db.Exec(`
		INSERT INTO ingest_cursors (source_id, cursor, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (source_id) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at
	`, c.SourceID.String(), string(c.Cursor), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to save ingest cursor: %w", err)
	}
	return nil
}

// getIngestCursor returns the cursor for a source, or ErrorNotFound when no
// cursor has been persisted for that source yet.
func (a *LocalArchive) getIngestCursor(sourceID uuid.UUID) (*core.IngestCursor, error) {
	var cursorStr, updatedAt string
	err := a.db.QueryRow(`
		SELECT cursor, updated_at FROM ingest_cursors WHERE source_id = ?
	`, sourceID.String()).Scan(&cursorStr, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("ingest_cursor", sourceID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ingest cursor: %w", err)
	}
	c := &core.IngestCursor{SourceID: sourceID, Cursor: []byte(cursorStr)}
	c.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ingest cursor updated_at: %w", err)
	}
	return c, nil
}
