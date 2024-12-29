package app

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	configContent := []byte(`
name: test-app
nats:
  embedded: true
  private: true
  logging: false
http:
  port: 9090
modules:
  test-module:
    enabled: true
    config:
      key: value
`)

	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(configContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test loading the config
	config, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify config values
	if config.Name != "test-app" {
		t.Errorf("Expected name 'test-app', got '%s'", config.Name)
	}
	if !config.NATS.Embedded {
		t.Error("Expected NATS.Embedded to be true")
	}
	if !config.NATS.Private {
		t.Error("Expected NATS.Private to be true")
	}
	if config.NATS.Logging {
		t.Error("Expected NATS.Logging to be false")
	}
	if config.HTTP.Port != 9090 {
		t.Errorf("Expected HTTP.Port to be 9090, got %d", config.HTTP.Port)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Create empty config file
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	config, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify default values
	if config.Name != "upspeak" {
		t.Errorf("Expected default name 'upspeak', got '%s'", config.Name)
	}
	if !config.NATS.Embedded {
		t.Error("Expected default NATS.Embedded to be true")
	}
	if config.NATS.Private {
		t.Error("Expected default NATS.Private to be false")
	}
	if !config.NATS.Logging {
		t.Error("Expected default NATS.Logging to be true")
	}
	if config.HTTP.Port != 8080 {
		t.Errorf("Expected default HTTP.Port to be 8080, got %d", config.HTTP.Port)
	}
}
