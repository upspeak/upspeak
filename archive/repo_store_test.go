package archive

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func setupTestArchive(t *testing.T) *LocalArchive {
	t.Helper()
	dir := t.TempDir()
	a, err := NewLocalArchive(dir)
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

var testOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

func TestSaveAndGetRepository(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:          core.NewID(),
		Slug:        "research",
		Name:        "AI Research",
		Description: "AI governance research",
		OwnerID:     testOwnerID,
	}

	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("SaveRepository failed: %v", err)
	}

	if repo.Version != 1 {
		t.Errorf("Expected version 1, got %d", repo.Version)
	}
	if repo.ShortID != "REPO-1" {
		t.Errorf("Expected short ID REPO-1, got %s", repo.ShortID)
	}

	got, err := a.GetRepository(repo.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}

	if got.Slug != "research" {
		t.Errorf("Expected slug 'research', got %q", got.Slug)
	}
	if got.Name != "AI Research" {
		t.Errorf("Expected name 'AI Research', got %q", got.Name)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
}

func TestSaveRepository_ShortIDSequence(t *testing.T) {
	a := setupTestArchive(t)

	for i := 1; i <= 3; i++ {
		repo := &core.Repository{
			ID:      core.NewID(),
			Slug:    core.FormatShortID("repo", i), // use as slug to keep unique
			Name:    "Test Repo",
			OwnerID: testOwnerID,
		}
		if err := a.SaveRepository(repo); err != nil {
			t.Fatalf("SaveRepository %d failed: %v", i, err)
		}
		expected := core.FormatShortID(core.PrefixRepo, i)
		if repo.ShortID != expected {
			t.Errorf("Repo %d: expected short ID %s, got %s", i, expected, repo.ShortID)
		}
	}
}

func TestGetRepositoryBySlug(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "my-project",
		Name:    "My Project",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("SaveRepository failed: %v", err)
	}

	got, err := a.GetRepositoryBySlug(testOwnerID, "my-project")
	if err != nil {
		t.Fatalf("GetRepositoryBySlug failed: %v", err)
	}
	if got.ID != repo.ID {
		t.Errorf("Expected ID %s, got %s", repo.ID, got.ID)
	}
}

func TestListRepositories(t *testing.T) {
	a := setupTestArchive(t)

	for i := 0; i < 5; i++ {
		repo := &core.Repository{
			ID:      core.NewID(),
			Slug:    core.FormatShortID("repo", i+1),
			Name:    "Repo",
			OwnerID: testOwnerID,
		}
		if err := a.SaveRepository(repo); err != nil {
			t.Fatalf("SaveRepository failed: %v", err)
		}
	}

	repos, total, err := a.ListRepositories(testOwnerID, core.ListOptions{Limit: 3, Offset: 0, SortBy: "created_at", Order: "desc"})
	if err != nil {
		t.Fatalf("ListRepositories failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(repos) != 3 {
		t.Errorf("Expected 3 repos in page, got %d", len(repos))
	}
}

func TestSaveRepository_Update(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "original",
		Name:    "Original Name",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update.
	repo.Name = "Updated Name"
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if repo.Version != 2 {
		t.Errorf("Expected version 2 after update, got %d", repo.Version)
	}

	got, _ := a.GetRepository(repo.ID)
	if got.Name != "Updated Name" {
		t.Errorf("Expected updated name, got %q", got.Name)
	}
}

func TestSaveRepository_VersionConflict(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "test",
		Name:    "Test",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Simulate stale version.
	stale := *repo
	stale.Version = 999
	stale.Name = "Stale Update"

	err := a.SaveRepository(&stale)
	if err == nil {
		t.Fatal("Expected version conflict error, got nil")
	}

	if _, ok := err.(*core.VersionConflictError); !ok {
		t.Errorf("Expected VersionConflictError, got %T: %v", err, err)
	}
}

func TestDeleteRepository(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "to-delete",
		Name:    "Delete Me",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := a.DeleteRepository(repo.ID); err != nil {
		t.Fatalf("DeleteRepository failed: %v", err)
	}

	_, err := a.GetRepository(repo.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

func TestSlugRedirect(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "new-slug",
		Name:    "Test",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := a.SaveSlugRedirect(testOwnerID, "old-slug", repo.ID); err != nil {
		t.Fatalf("SaveSlugRedirect failed: %v", err)
	}

	repoID, currentSlug, err := a.GetSlugRedirect(testOwnerID, "old-slug")
	if err != nil {
		t.Fatalf("GetSlugRedirect failed: %v", err)
	}
	if repoID != repo.ID {
		t.Errorf("Expected repo ID %s, got %s", repo.ID, repoID)
	}
	if currentSlug != "new-slug" {
		t.Errorf("Expected current slug 'new-slug', got %q", currentSlug)
	}
}

func TestResolveRepoRef(t *testing.T) {
	a := setupTestArchive(t)

	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "research",
		Name:    "Research",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Resolve by UUID.
	got, err := a.ResolveRepoRef(testOwnerID, repo.ID.String())
	if err != nil {
		t.Fatalf("ResolveRepoRef by UUID failed: %v", err)
	}
	if got.ID != repo.ID {
		t.Errorf("UUID resolution: expected ID %s, got %s", repo.ID, got.ID)
	}

	// Resolve by short ID.
	got, err = a.ResolveRepoRef(testOwnerID, repo.ShortID)
	if err != nil {
		t.Fatalf("ResolveRepoRef by short ID failed: %v", err)
	}
	if got.ID != repo.ID {
		t.Errorf("Short ID resolution: expected ID %s, got %s", repo.ID, got.ID)
	}

	// Resolve by slug.
	got, err = a.ResolveRepoRef(testOwnerID, "research")
	if err != nil {
		t.Fatalf("ResolveRepoRef by slug failed: %v", err)
	}
	if got.ID != repo.ID {
		t.Errorf("Slug resolution: expected ID %s, got %s", repo.ID, got.ID)
	}

	// Resolve by old slug (redirect).
	if err := a.SaveSlugRedirect(testOwnerID, "old-name", repo.ID); err != nil {
		t.Fatalf("SaveSlugRedirect failed: %v", err)
	}
	_, err = a.ResolveRepoRef(testOwnerID, "old-name")
	if err == nil {
		t.Fatal("Expected ErrorSlugRedirect, got nil")
	}
	if _, ok := err.(*core.ErrorSlugRedirect); !ok {
		t.Errorf("Expected *ErrorSlugRedirect, got %T: %v", err, err)
	}

	// Resolve non-existent.
	_, err = a.ResolveRepoRef(testOwnerID, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent ref, got nil")
	}
}

func TestSequences(t *testing.T) {
	a := setupTestArchive(t)
	repoID := core.NewID()

	// Repo sequence (internal function, tested via the archive's db).
	for i := 1; i <= 3; i++ {
		seq, err := nextRepoSequence(a.db, repoID, "node")
		if err != nil {
			t.Fatalf("nextRepoSequence failed: %v", err)
		}
		if seq != i {
			t.Errorf("Expected sequence %d, got %d", i, seq)
		}
	}

	// User sequence (internal function).
	for i := 1; i <= 3; i++ {
		seq, err := nextUserSequence(a.db, testOwnerID, "repo")
		if err != nil {
			t.Fatalf("nextUserSequence failed: %v", err)
		}
		if seq != i {
			t.Errorf("Expected sequence %d, got %d", i, seq)
		}
	}

	// Global sequence (internal function).
	for i := 1; i <= 3; i++ {
		seq, err := nextGlobalSequence(a.db, "job")
		if err != nil {
			t.Fatalf("nextGlobalSequence failed: %v", err)
		}
		if seq != i {
			t.Errorf("Expected sequence %d, got %d", i, seq)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
