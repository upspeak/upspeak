package repo

import (
	"fmt"
	"testing"

	"github.com/rs/xid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// mockArchive is a simple mock implementation of core.Archive for testing.
type mockArchive struct{}

func (m *mockArchive) SaveNode(node *core.Node) error                   { return nil }
func (m *mockArchive) GetNode(nodeID xid.ID) (*core.Node, error)        { return nil, nil }
func (m *mockArchive) DeleteNode(nodeID xid.ID) error                   { return nil }
func (m *mockArchive) SaveEdge(edge *core.Edge) error                   { return nil }
func (m *mockArchive) GetEdge(edgeID xid.ID) (*core.Edge, error)        { return nil, nil }
func (m *mockArchive) DeleteEdge(edgeID xid.ID) error                   { return nil }
func (m *mockArchive) SaveThread(thread *core.Thread) error             { return nil }
func (m *mockArchive) GetThread(nodeID xid.ID) (*core.Thread, error)    { return nil, nil }
func (m *mockArchive) DeleteThread(nodeID xid.ID) error                 { return nil }
func (m *mockArchive) SaveAnnotation(annotation *core.Annotation) error { return nil }
func (m *mockArchive) GetAnnotation(nodeID xid.ID) (*core.Annotation, error) {
	return nil, nil
}
func (m *mockArchive) DeleteAnnotation(nodeID xid.ID) error { return nil }

func TestModuleRepo_Name(t *testing.T) {
	m := &ModuleRepo{}
	if got := m.Name(); got != "repo" {
		t.Errorf("Name() = %v, want %v", got, "repo")
	}
}

func TestModuleRepo_Init(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{
			name: "with valid repo ID and name",
			config: map[string]any{
				"repo_id":   xid.New().String(),
				"repo_name": "test-repo",
			},
			wantErr: false,
		},
		{
			name:    "with empty config",
			config:  map[string]any{},
			wantErr: false,
		},
		{
			name:    "with nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "with invalid repo ID",
			config: map[string]any{
				"repo_id": "not-a-valid-xid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ModuleRepo{}
			err := m.Init(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify repository was created for successful initialization
			if !tt.wantErr && len(m.repos) != 1 {
				t.Errorf("Init() created %d repositories, want 1", len(m.repos))
			}

			// Verify that a valid repo ID was created when not provided
			if !tt.wantErr && (tt.config == nil || tt.config["repo_id"] == nil) {
				repos := m.ListRepositories()
				if len(repos) != 1 {
					t.Errorf("Expected 1 repository, got %d", len(repos))
					return
				}
				// Verify the generated ID is a valid xid
				for repoID := range repos {
					if _, err := xid.FromString(repoID); err != nil {
						t.Errorf("Generated repo ID is not a valid xid: %s, error: %v", repoID, err)
					}
				}
			}
		})
	}
}

func TestModuleRepo_Init_AutoGenerateRepoID(t *testing.T) {
	m := &ModuleRepo{}

	// Initialize without providing repo_id
	err := m.Init(map[string]any{
		"repo_name": "auto-generated",
	})

	if err != nil {
		t.Errorf("Init() without repo_id failed: %v", err)
		return
	}

	// Verify a repository was created
	repos := m.ListRepositories()
	if len(repos) != 1 {
		t.Errorf("Expected 1 repository, got %d", len(repos))
		return
	}

	// Verify the repo ID is a valid xid
	for repoID, repoName := range repos {
		if _, err := xid.FromString(repoID); err != nil {
			t.Errorf("Auto-generated repo ID is not valid: %s, error: %v", repoID, err)
		}

		if repoName != "auto-generated" {
			t.Errorf("Expected repo name 'auto-generated', got '%s'", repoName)
		}
	}
}

func TestModuleRepo_GetRepository(t *testing.T) {
	m := &ModuleRepo{}
	repoID := xid.New().String()
	m.Init(map[string]any{
		"repo_id":   repoID,
		"repo_name": "test",
	})

	tests := []struct {
		name    string
		repoID  string
		wantErr bool
	}{
		{
			name:    "existing repository",
			repoID:  repoID,
			wantErr: false,
		},
		{
			name:    "non-existent repository",
			repoID:  xid.New().String(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.GetRepository(tt.repoID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRepository() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestModuleRepo_ListRepositories(t *testing.T) {
	m := &ModuleRepo{}
	repoID := xid.New().String()
	m.Init(map[string]any{
		"repo_id":   repoID,
		"repo_name": "test-repo",
	})

	repos := m.ListRepositories()

	if len(repos) != 1 {
		t.Errorf("ListRepositories() returned %d repositories, want 1", len(repos))
	}

	if name, exists := repos[repoID]; !exists {
		t.Errorf("ListRepositories() missing repository with ID %s", repoID)
	} else if name != "test-repo" {
		t.Errorf("ListRepositories() repository name = %s, want test-repo", name)
	}
}

func TestModuleRepo_SetArchive(t *testing.T) {
	m := &ModuleRepo{}
	m.Init(map[string]any{
		"repo_id":   xid.New().String(),
		"repo_name": "test",
	})

	archive := &mockArchive{}
	m.SetArchive(archive)

	// Verify archive was set by checking we can still access the repository
	repos := m.ListRepositories()
	if len(repos) != 1 {
		t.Errorf("SetArchive() affected repository count: got %d, want 1", len(repos))
	}
}

func TestModuleRepo_HTTPHandlers(t *testing.T) {
	m := &ModuleRepo{}
	m.Init(map[string]any{})

	pub := app.Publisher{}
	handlers := m.HTTPHandlers(pub)

	// Define expected handler patterns
	expectedPaths := map[string]bool{
		"/":                       true,
		"/{repo_id}/nodes":        true,
		"/{repo_id}/nodes/{id}":   true,
		"/{repo_id}/edges":        true,
		"/{repo_id}/edges/{id}":   true,
		"/{repo_id}/threads":      true,
		"/{repo_id}/threads/{id}": true,
	}

	if len(handlers) < len(expectedPaths) {
		t.Errorf("HTTPHandlers() returned %d handlers, want at least %d", len(handlers), len(expectedPaths))
	}

	// Verify expected paths are registered
	registeredPaths := make(map[string]bool)
	for _, h := range handlers {
		registeredPaths[h.Path] = true
	}

	for path := range expectedPaths {
		if !registeredPaths[path] {
			t.Errorf("HTTPHandlers() missing expected path: %s", path)
		}
	}
}

func TestModuleRepo_MsgHandlers(t *testing.T) {
	m := &ModuleRepo{}
	repoID := xid.New().String()
	m.Init(map[string]any{
		"repo_id": repoID,
	})

	pub := app.Publisher{}
	handlers := m.MsgHandlers(pub)

	if len(handlers) != 1 {
		t.Errorf("MsgHandlers() returned %d handlers, want 1", len(handlers))
		return
	}

	expectedSubject := fmt.Sprintf("repo.%s.in", repoID)
	if handlers[0].Subject != expectedSubject {
		t.Errorf("MsgHandlers() subject = %s, want %s", handlers[0].Subject, expectedSubject)
	}
}
