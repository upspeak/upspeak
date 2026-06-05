package archive

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestIngestCursor_UpsertAndGet(t *testing.T) {
	a := setupTestArchive(t)
	srcID := core.NewID()

	// Missing → ErrorNotFound.
	if _, err := a.GetIngestCursor(srcID); !errors.As(err, new(*core.ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound, got %v", err)
	}

	// Save.
	c := &core.IngestCursor{SourceID: srcID, Cursor: json.RawMessage(`{"since":"a"}`)}
	if err := a.SaveIngestCursor(c); err != nil {
		t.Fatalf("SaveIngestCursor: %v", err)
	}
	got, err := a.GetIngestCursor(srcID)
	if err != nil {
		t.Fatalf("GetIngestCursor: %v", err)
	}
	if string(got.Cursor) != `{"since":"a"}` {
		t.Fatalf("cursor mismatch: %s", got.Cursor)
	}

	// Upsert (overwrite).
	c.Cursor = json.RawMessage(`{"since":"b"}`)
	if err := a.SaveIngestCursor(c); err != nil {
		t.Fatalf("SaveIngestCursor (upsert): %v", err)
	}
	got, _ = a.GetIngestCursor(srcID)
	if string(got.Cursor) != `{"since":"b"}` {
		t.Fatalf("upsert failed: %s", got.Cursor)
	}
}
