package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestSaveAndGetFilter(t *testing.T) {
	a := setupTestArchive(t)
	setupTestRepo(t, a)

	filter := &core.Filter{
		ID:     core.NewID(),
		RepoID: testRepoID,
		Name:   "AI Articles",
		Mode:   core.FilterModeAll,
		Conditions: []core.Condition{
			{Field: "node.type", Op: core.OpEq, Value: json.RawMessage(`"article"`)},
		},
		CreatedBy: testOwnerID,
	}

	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter failed: %v", err)
	}
	if filter.ShortID == "" {
		t.Fatal("expected short ID to be generated")
	}
	if filter.Version != 1 {
		t.Fatalf("expected version 1, got %d", filter.Version)
	}

	got, err := a.GetFilter(filter.ID)
	if err != nil {
		t.Fatalf("GetFilter failed: %v", err)
	}
	if got.Name != "AI Articles" {
		t.Errorf("expected name 'AI Articles', got '%s'", got.Name)
	}
	if got.Mode != core.FilterModeAll {
		t.Errorf("expected mode 'all', got '%s'", got.Mode)
	}
	if len(got.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got.Conditions))
	}
	if got.Conditions[0].Field != "node.type" {
		t.Errorf("expected field 'node.type', got '%s'", got.Conditions[0].Field)
	}
}

func TestSaveFilter_Update(t *testing.T) {
	a := setupTestArchive(t)
	setupTestRepo(t, a)

	filter := &core.Filter{
		ID:        core.NewID(),
		RepoID:    testRepoID,
		Name:      "Original",
		Mode:      core.FilterModeAll,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter create failed: %v", err)
	}

	filter.Name = "Updated"
	filter.Mode = core.FilterModeAny
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter update failed: %v", err)
	}
	if filter.Version != 2 {
		t.Fatalf("expected version 2, got %d", filter.Version)
	}

	got, err := a.GetFilter(filter.ID)
	if err != nil {
		t.Fatalf("GetFilter failed: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("expected name 'Updated', got '%s'", got.Name)
	}
	if got.Mode != core.FilterModeAny {
		t.Errorf("expected mode 'any', got '%s'", got.Mode)
	}
}

func TestSaveFilter_VersionConflict(t *testing.T) {
	a := setupTestArchive(t)
	setupTestRepo(t, a)

	filter := &core.Filter{
		ID:        core.NewID(),
		RepoID:    testRepoID,
		Name:      "Test",
		Mode:      core.FilterModeAll,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter failed: %v", err)
	}

	// Simulate stale version.
	filter.Version = 0 // Wrong version for update.
	filter.Version = 1 // Set correct version first, then modify...

	// Save again with correct version to bump to 2.
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter v2 failed: %v", err)
	}

	// Now try with stale version 1 (current is 2).
	filter.Version = 1
	err := a.SaveFilter(filter)
	if err == nil {
		t.Fatal("expected version conflict error")
	}
	if _, ok := err.(*core.VersionConflictError); !ok {
		t.Fatalf("expected VersionConflictError, got %T: %v", err, err)
	}
}

func TestListFilters(t *testing.T) {
	a := setupTestArchive(t)
	setupTestRepo(t, a)

	for i := 0; i < 3; i++ {
		f := &core.Filter{
			ID:        core.NewID(),
			RepoID:    testRepoID,
			Name:      "Filter " + string(rune('A'+i)),
			Mode:      core.FilterModeAll,
			CreatedBy: testOwnerID,
		}
		if err := a.SaveFilter(f); err != nil {
			t.Fatalf("SaveFilter %d failed: %v", i, err)
		}
	}

	filters, total, err := a.ListFilters(testRepoID, core.FilterListOptions{
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListFilters failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(filters) != 3 {
		t.Fatalf("expected 3 filters, got %d", len(filters))
	}
}

func TestDeleteFilter(t *testing.T) {
	a := setupTestArchive(t)
	setupTestRepo(t, a)

	filter := &core.Filter{
		ID:        core.NewID(),
		RepoID:    testRepoID,
		Name:      "To Delete",
		Mode:      core.FilterModeAll,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter failed: %v", err)
	}

	if err := a.DeleteFilter(filter.ID); err != nil {
		t.Fatalf("DeleteFilter failed: %v", err)
	}

	_, err := a.GetFilter(filter.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestGetFilterReferences_Empty(t *testing.T) {
	a := setupTestArchive(t)

	refs, err := a.GetFilterReferences(core.NewID())
	if err != nil {
		t.Fatalf("GetFilterReferences failed: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 references, got %d", len(refs))
	}
}

// setupTestRepo creates a test repository for filter tests.
var testRepoID = core.NewID()

func setupTestRepo(t *testing.T, a *LocalArchive) {
	t.Helper()
	repo := &core.Repository{
		ID:      testRepoID,
		Slug:    "test-repo",
		Name:    "Test Repo",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Failed to create test repo: %v", err)
	}
}
