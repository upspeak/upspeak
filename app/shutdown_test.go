package app

import (
	"context"
	"testing"
	"time"
)

func TestGracefulShutdown(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{Embedded: true, Private: true},
	})

	if err := app.Start(); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- app.Stop()
	}()

	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}
	case <-ctx.Done():
		t.Error("Shutdown timed out")
	}

	app.readyLock.RLock()
	if app.ready {
		t.Error("App should not be ready after shutdown")
	}
	app.readyLock.RUnlock()
}
