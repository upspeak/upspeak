package archive

import (
	"testing"

	"github.com/upspeak/upspeak/core"
)

// TestSaveSink_ConnectionID verifies that a Sink with a ConnectionID is
// persisted and retrieved correctly.
func TestSaveSink_ConnectionID(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	connID := core.NewID()
	sink := &core.Sink{
		ID:           core.NewID(),
		RepoID:       repo.ID,
		Name:         "discourse-out",
		Connector:    core.ConnectorDiscourse,
		ConnectionID: &connID,
		Config:       map[string]any{"category_id": "5"},
		CreatedBy:    repo.OwnerID,
	}
	if err := a.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}
	got, err := a.GetSink(sink.ID)
	if err != nil {
		t.Fatalf("GetSink: %v", err)
	}
	if got.ConnectionID == nil || *got.ConnectionID != connID {
		t.Fatalf("connection_id not persisted: %v", got.ConnectionID)
	}
}
