package archive

import (
	"testing"

	"github.com/upspeak/upspeak/core"
)

// TestSaveSource_ConnectionID verifies that a Source with a ConnectionID is
// persisted and retrieved correctly.
func TestSaveSource_ConnectionID(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	connID := core.NewID()
	src := &core.Source{
		ID:           core.NewID(),
		RepoID:       repo.ID,
		Name:         "discourse-cat",
		Connector:    core.ConnectorDiscourse,
		ConnectionID: &connID,
		Config:       map[string]any{"category_id": "5"},
		CreatedBy:    repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	got, err := a.GetSource(src.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.ConnectionID == nil || *got.ConnectionID != connID {
		t.Fatalf("connection_id not persisted: %v", got.ConnectionID)
	}
}
