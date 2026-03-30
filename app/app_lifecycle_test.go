package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestFullAppLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full app lifecycle test in short mode")
	}

	config := Config{
		Name: "test-app",
		NATS: NATSConfig{Embedded: true, Private: true},
		HTTP: HTTPConfig{Port: 8081},
		Modules: map[string]ModuleConfig{
			"test-module": {
				Enabled: true,
				Config:  map[string]any{"key": "value"},
			},
		},
	}

	app := New(config)

	// Create test module with HTTP handler.
	module := newMockModule("test-module")
	module.addHTTPHandler(http.MethodGet, "/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "test response")
	})

	if err := app.AddModule(module); err != nil {
		t.Fatalf("Failed to add module: %v", err)
	}

	if err := app.Start(); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Wait for HTTP server to be ready.
	time.Sleep(100 * time.Millisecond)

	// Test HTTP endpoint.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/test-module/test", config.HTTP.Port))
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Stop the app.
	if err := app.Stop(); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}
}
