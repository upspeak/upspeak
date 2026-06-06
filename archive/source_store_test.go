package archive

import (
	"testing"

	"github.com/google/uuid"
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

// TestListRepoSourcesForSink verifies that ListRepoSourcesForSink returns exactly
// those repo-connector Sources (across all repositories) whose config references
// the given sink ID, and excludes Sources pointing at a different sink.
func TestListRepoSourcesForSink(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	sinkID := uuid.New()

	// The subscribing source: config["sink_id"] matches sinkID.
	want := &core.Source{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "from-a",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkID.String()},
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(want); err != nil {
		t.Fatalf("save source: %v", err)
	}

	// A repo source pointing at a different sink must not match.
	other := &core.Source{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "other",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": uuid.New().String()},
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(other); err != nil {
		t.Fatalf("save other source: %v", err)
	}

	got, err := a.ListRepoSourcesForSink(sinkID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != want.ID {
		t.Fatalf("expected exactly the subscribing source, got %d", len(got))
	}
}
