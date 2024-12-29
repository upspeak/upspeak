package app

import (
	"testing"
	"time"
)

func TestStartEmbeddedNatsServer(t *testing.T) {
	opts := NATSConfig{
		Private: true,
		Logging: false,
	}

	server, err := startEmbeddedNatsServer("test-app", opts)
	if err != nil {
		t.Fatalf("Failed to start embedded NATS server: %v", err)
	}
	defer server.Shutdown()

	if !server.ReadyForConnections(2 * time.Second) {
		t.Error("Server not ready for connections")
	}
}

func TestConnectToEmbeddedNATS(t *testing.T) {
	opts := NATSConfig{
		Private: true,
		Logging: false,
	}

	server, err := startEmbeddedNatsServer("test-app", opts)
	if err != nil {
		t.Fatalf("Failed to start embedded NATS server: %v", err)
	}
	defer server.Shutdown()

	conn, err := connectToEmbeddedNATS("test-app", server, opts)
	if err != nil {
		t.Fatalf("Failed to connect to embedded NATS: %v", err)
	}
	defer conn.Close()

	if !conn.IsConnected() {
		t.Error("Connection should be established")
	}
}

func TestNATSConnectionFailure(t *testing.T) {
	config := Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: false,
			URL:      "nats://nonexistent:4222",
		},
	}

	app := New(config)
	err := app.startNats()

	if err == nil {
		t.Error("Expected error when connecting to non-existent NATS server")
	}
}
