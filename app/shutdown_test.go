package app

import (
	"context"
	"testing"
	"time"
)

func TestGracefulShutdown(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	})

	if err := app.Start(); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start shutdown in a goroutine
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- app.Stop()
	}()

	// Wait for shutdown or timeout
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	case <-ctx.Done():
		t.Error("Shutdown timed out")
	}

	// Verify app state after shutdown
	app.readyLock.RLock()
	if app.ready {
		t.Error("App should not be ready after shutdown")
	}
	app.readyLock.RUnlock()

	if app.nc != nil && !app.nc.IsClosed() {
		t.Error("NATS connection should be closed")
	}
}
