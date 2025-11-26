package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/upspeak/upspeak/app"
)

// TestModuleUI_EmbeddedAssets verifies that SvelteKit build assets are properly embedded
func TestModuleUI_EmbeddedAssets(t *testing.T) {
	module := &ModuleUI{}

	// Initialize module
	if err := module.Init(nil); err != nil {
		t.Fatalf("Failed to initialize module: %v", err)
	}

	// Get HTTP handlers
	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	// Find the root handler
	var rootHandler http.HandlerFunc
	for _, h := range handlers {
		if h.Path == "/" {
			rootHandler = h.Handler
			break
		}
	}

	if rootHandler == nil {
		t.Fatal("Root handler not found")
	}

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedType   string
		contains       string
	}{
		{
			name:           "Root path serves index.html",
			path:           "/",
			expectedStatus: http.StatusOK,
			expectedType:   "text/html",
			contains:       "<!doctype html>",
		},
		{
			name:           "Static file robots.txt served",
			path:           "/robots.txt",
			expectedStatus: http.StatusOK,
			expectedType:   "text/plain",
			contains:       "User-agent:",
		},
		{
			name:           "Directory returns 404",
			path:           "/logo/",
			expectedStatus: http.StatusNotFound,
			expectedType:   "",
			contains:       "",
		},
		{
			name:           "Directory without slash returns 404",
			path:           "/logo",
			expectedStatus: http.StatusNotFound,
			expectedType:   "",
			contains:       "",
		},
		{
			name:           "Non-existent route serves index.html for SPA",
			path:           "/some-spa-route",
			expectedStatus: http.StatusOK,
			expectedType:   "text/html",
			contains:       "<!doctype html>",
		},
		{
			name:           "Non-existent nested route serves index.html",
			path:           "/some/nested/route",
			expectedStatus: http.StatusOK,
			expectedType:   "text/html",
			contains:       "<!doctype html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			rootHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if tt.expectedType != "" {
				contentType := resp.Header.Get("Content-Type")
				if !strings.Contains(contentType, tt.expectedType) {
					t.Errorf("Expected content type to contain %q, got %q", tt.expectedType, contentType)
				}
			}

			if tt.contains != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				if !strings.Contains(string(body), tt.contains) {
					t.Errorf("Expected response body to contain %q", tt.contains)
				}
			}
		})
	}
}

// TestModuleUI_AppAssets verifies that SvelteKit _app assets are served correctly
func TestModuleUI_AppAssets(t *testing.T) {
	module := &ModuleUI{}

	if err := module.Init(nil); err != nil {
		t.Fatalf("Failed to initialize module: %v", err)
	}

	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	// Find the _app handler
	var appHandler http.HandlerFunc
	for _, h := range handlers {
		if h.Path == "/_app/" {
			appHandler = h.Handler
			break
		}
	}

	if appHandler == nil {
		t.Fatal("_app handler not found")
	}

	// Test that _app assets are served
	req := httptest.NewRequest("GET", "/_app/version.json", nil)
	w := httptest.NewRecorder()

	appHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for _app/version.json, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected JSON content type for version.json, got %q", contentType)
	}
}

// TestModuleUI_StaticFileFromBuild verifies static files copied to build directory are accessible
func TestModuleUI_StaticFileFromBuild(t *testing.T) {
	module := &ModuleUI{}

	if err := module.Init(nil); err != nil {
		t.Fatalf("Failed to initialize module: %v", err)
	}

	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	var rootHandler http.HandlerFunc
	for _, h := range handlers {
		if h.Path == "/" {
			rootHandler = h.Handler
			break
		}
	}

	if rootHandler == nil {
		t.Fatal("Root handler not found")
	}

	tests := []struct {
		name         string
		path         string
		expectedType string
	}{
		{
			name:         "robots.txt from static",
			path:         "/robots.txt",
			expectedType: "text/plain",
		},
		{
			name:         "SVG logo file",
			path:         "/logo/logo-no-bg.svg",
			expectedType: "image/svg+xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			rootHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200 for %s, got %d", tt.path, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, tt.expectedType) {
				t.Errorf("Expected content type to contain %q for %s, got %q", tt.expectedType, tt.path, contentType)
			}
		})
	}
}

// TestModuleUI_ModuleInterface verifies the module implements the app.Module interface correctly
func TestModuleUI_ModuleInterface(t *testing.T) {
	module := &ModuleUI{}

	// Test Name
	if name := module.Name(); name != "ui" {
		t.Errorf("Expected module name 'ui', got %q", name)
	}

	// Test Init
	if err := module.Init(nil); err != nil {
		t.Errorf("Init failed: %v", err)
	}

	// Test HTTPHandlers returns expected handlers
	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	expectedPaths := []string{"/_app/", "/favicon.ico", "/"}
	foundPaths := make(map[string]bool)

	for _, h := range handlers {
		foundPaths[h.Path] = true
		if h.Method != "GET" {
			t.Errorf("Expected method GET for path %s, got %s", h.Path, h.Method)
		}
		if h.Handler == nil {
			t.Errorf("Handler is nil for path %s", h.Path)
		}
	}

	for _, expectedPath := range expectedPaths {
		if !foundPaths[expectedPath] {
			t.Errorf("Expected handler for path %s not found", expectedPath)
		}
	}

	// Test MsgHandlers returns empty
	msgHandlers := module.MsgHandlers(pub)
	if len(msgHandlers) != 0 {
		t.Errorf("Expected no message handlers, got %d", len(msgHandlers))
	}
}

// TestModuleUI_DirectoryListingPrevention ensures directories cannot be listed
func TestModuleUI_DirectoryListingPrevention(t *testing.T) {
	module := &ModuleUI{}

	if err := module.Init(nil); err != nil {
		t.Fatalf("Failed to initialize module: %v", err)
	}

	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	var rootHandler http.HandlerFunc
	for _, h := range handlers {
		if h.Path == "/" {
			rootHandler = h.Handler
			break
		}
	}

	directoryPaths := []string{"/logo/", "/logo"}

	for _, path := range directoryPaths {
		t.Run("Directory "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			rootHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 404 for directory %s, got %d", path, resp.StatusCode)
			}
		})
	}
}

// TestModuleUI_SPAFallback ensures non-existent routes fall back to index.html
func TestModuleUI_SPAFallback(t *testing.T) {
	module := &ModuleUI{}

	if err := module.Init(nil); err != nil {
		t.Fatalf("Failed to initialize module: %v", err)
	}

	pub := app.Publisher{}
	handlers := module.HTTPHandlers(pub)

	var rootHandler http.HandlerFunc
	for _, h := range handlers {
		if h.Path == "/" {
			rootHandler = h.Handler
			break
		}
	}

	spaRoutes := []string{
		"/about",
		"/user/123",
		"/settings/profile",
		"/non/existent/route",
	}

	for _, route := range spaRoutes {
		t.Run("SPA route "+route, func(t *testing.T) {
			req := httptest.NewRequest("GET", route, nil)
			w := httptest.NewRecorder()

			rootHandler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 for SPA route %s, got %d", route, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if !strings.Contains(string(body), "<!doctype html>") {
				t.Errorf("Expected index.html for SPA route %s", route)
			}

			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "text/html") {
				t.Errorf("Expected HTML content type for SPA route %s, got %q", route, contentType)
			}
		})
	}
}
