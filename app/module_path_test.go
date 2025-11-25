package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestAddModuleOnPath(t *testing.T) {
	tests := []struct {
		name    string
		modules []struct {
			module Module
			path   string
		}
		wantErr     bool
		errContains string
	}{
		{
			name: "single module at root",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("ui"), ""},
			},
			wantErr: false,
		},
		{
			name: "single module at root with slash",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("ui"), "/"},
			},
			wantErr: false,
		},
		{
			name: "multiple modules at different paths",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "/api"},
				{newMockModule("writer"), "/writer"},
			},
			wantErr: false,
		},
		{
			name: "module at root and namespaced modules",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "/api"},
				{newMockModule("ui"), ""},
			},
			wantErr: false,
		},
		{
			name: "two modules at root - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("ui"), ""},
				{newMockModule("admin"), ""},
			},
			wantErr:     true,
			errContains: "already mounted at root",
		},
		{
			name: "two modules at same path - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "/api"},
				{newMockModule("api2"), "/api"},
			},
			wantErr:     true,
			errContains: "path conflict",
		},
		{
			name: "path normalization - no leading slash",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "api"},
			},
			wantErr: false,
		},
		{
			name: "path normalization - trailing slash",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "/api/"},
			},
			wantErr: false,
		},
		{
			name: "duplicate module registration - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("api"), "/api"},
				{newMockModule("api"), "/v2"},
			},
			wantErr:     true,
			errContains: "already registered",
		},
		{
			name: "path traversal attempt - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("evil"), "../etc"},
			},
			wantErr:     true,
			errContains: "path traversal not allowed",
		},
		{
			name: "reserved path /healthz - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("health"), "/healthz"},
			},
			wantErr:     true,
			errContains: "reserved system endpoint",
		},
		{
			name: "reserved path /readiness - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("ready"), "/readiness"},
			},
			wantErr:     true,
			errContains: "reserved system endpoint",
		},
		{
			name: "path under reserved path - should fail",
			modules: []struct {
				module Module
				path   string
			}{
				{newMockModule("health"), "/healthz/detail"},
			},
			wantErr:     true,
			errContains: "reserved system endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New(Config{
				Name: "test-app",
				NATS: NATSConfig{
					Embedded: true,
					Private:  true,
				},
			})

			var err error
			for _, m := range tt.modules {
				err = app.AddModuleOnPath(m.module, m.path)
				if err != nil {
					break
				}
			}

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestAddModuleOnPathNormalization(t *testing.T) {
	tests := []struct {
		name         string
		inputPath    string
		expectedPath string
	}{
		{"empty string", "", ""},
		{"root slash", "/", ""},
		{"simple path", "api", "/api"},
		{"path with leading slash", "/api", "/api"},
		{"path with trailing slash", "/api/", "/api"},
		{"path with both slashes", "/api/", "/api"},
		{"whitespace trimming", "  /api  ", "/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New(Config{
				Name: "test-app",
				NATS: NATSConfig{
					Embedded: true,
					Private:  true,
				},
			})

			module := newMockModule("test")
			if err := app.AddModuleOnPath(module, tt.inputPath); err != nil {
				t.Fatalf("Failed to add module: %v", err)
			}

			if app.modules["test"].path != tt.expectedPath {
				t.Errorf("Expected path '%s', got '%s'", tt.expectedPath, app.modules["test"].path)
			}
		})
	}
}

func TestAddModuleUsesAddModuleOnPath(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	})

	module := newMockModule("writer")
	if err := app.AddModule(module); err != nil {
		t.Fatalf("Failed to add module: %v", err)
	}

	// AddModule should use module name as path
	if app.modules["writer"].path != "/writer" {
		t.Errorf("Expected path /writer, got %s", app.modules["writer"].path)
	}
}

func TestRootModuleTracking(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	})

	// Add a root module
	uiModule := newMockModule("ui")
	if err := app.AddModuleOnPath(uiModule, ""); err != nil {
		t.Fatalf("Failed to add UI module: %v", err)
	}

	if app.rootModule != "ui" {
		t.Errorf("Expected rootModule to be 'ui', got '%s'", app.rootModule)
	}

	// Try to add another root module - should fail
	adminModule := newMockModule("admin")
	err := app.AddModuleOnPath(adminModule, "")
	if err == nil {
		t.Error("Expected error when adding second root module")
	}
	if !strings.Contains(err.Error(), "already mounted at root") {
		t.Errorf("Expected 'already mounted at root' error, got: %v", err)
	}
}

func TestBuildHandlerPath(t *testing.T) {
	app := &App{}

	tests := []struct {
		name        string
		mountPath   string
		handlerPath string
		expected    string
	}{
		{"root mount", "", "/about", "/about"},
		{"root mount with nested path", "", "/user/profile", "/user/profile"},
		{"namespaced mount", "/api", "/users", "/api/users"},
		{"namespaced mount with nested", "/v1", "/posts/list", "/v1/posts/list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := app.buildHandlerPath(tt.mountPath, tt.handlerPath)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestModuleRegistrationOrder(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	})

	// Add modules in specific order: root, then namespaced
	uiModule := newMockModule("ui")
	uiModule.addHTTPHandler("GET", "/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("UI"))
	})

	apiModule := newMockModule("api")
	apiModule.addHTTPHandler("GET", "/users", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("API"))
	})

	// Add in order: API first, then UI at root
	if err := app.AddModuleOnPath(apiModule, "/api"); err != nil {
		t.Fatalf("Failed to add API module: %v", err)
	}

	if err := app.AddModuleOnPath(uiModule, ""); err != nil {
		t.Fatalf("Failed to add UI module: %v", err)
	}

	// Start should register non-root modules first, then root
	if err := app.Start(); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}
	defer app.Stop()

	// Verify both are registered
	if len(app.modules) != 2 {
		t.Errorf("Expected 2 modules, got %d", len(app.modules))
	}
}
