package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestFullAppLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full app lifecycle test in short mode.")
	}
	// Create app configuration
	config := Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
		HTTP: HTTPConfig{
			Port: 8081,
		},
		Modules: map[string]ModuleConfig{
			"test-module": {
				Enabled: true,
				Config: map[string]any{
					"key": "value",
				},
			},
		},
	}

	// Create app instance
	app := New(config)
	app.modules = make(map[string]Module)

	// Create test module with both HTTP and NATS handlers
	module := newMockModule("test-module")

	// Add HTTP handler
	module.addHTTPHandler(http.MethodGet, "/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "test response")
	})

	// Add NATS handler
	messageReceived := make(chan bool)
	module.addMsgHandler("test.subject", func(msg *nats.Msg) {
		messageReceived <- true
	})

	app.AddModule(module)

	// Start the app
	if err := app.Start(); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Wait for app to be ready
	time.Sleep(100 * time.Millisecond)

	// Test HTTP endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/test-module/test", config.HTTP.Port))
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test NATS messaging
	if err := app.nc.Publish("test.subject", []byte("test message")); err != nil {
		t.Fatalf("Failed to publish NATS message: %v", err)
	}

	// Wait for message to be received
	select {
	case <-messageReceived:
		// Success
	case <-time.After(time.Second):
		t.Error("Timeout waiting for NATS message")
	}

	// Stop the app
	if err := app.Stop(); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}
}
