package app

import (
	"fmt"
	"testing"
)

func TestModuleRegistration(t *testing.T) {
	app := New(Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	})

	// Test adding a module
	module := newMockModule("test-module")
	if err := app.AddModule(module); err != nil {
		t.Fatalf("Failed to add module: %v", err)
	}

	if len(app.modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(app.modules))
	}

	if app.modules["test-module"].module != module {
		t.Error("Module not properly registered")
	}

	// Verify module is mounted at /test-module by default
	if app.modules["test-module"].path != "/test-module" {
		t.Errorf("Expected path /test-module, got %s", app.modules["test-module"].path)
	}
}

func TestModuleInitialization(t *testing.T) {
	tests := []struct {
		name         string
		moduleConfig map[string]ModuleConfig
		initError    error
		wantError    bool
	}{
		{
			name: "successful initialization",
			moduleConfig: map[string]ModuleConfig{
				"test-module": {
					Enabled: true,
					Config: map[string]any{
						"key": "value",
					},
				},
			},
			initError: nil,
			wantError: false,
		},
		{
			name: "disabled module",
			moduleConfig: map[string]ModuleConfig{
				"test-module": {
					Enabled: false,
				},
			},
			initError: nil,
			wantError: false,
		},
		{
			name: "initialization error",
			moduleConfig: map[string]ModuleConfig{
				"test-module": {
					Enabled: true,
				},
			},
			initError: fmt.Errorf("init error"),
			wantError: true,
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
				Modules: tt.moduleConfig,
			})

			module := newMockModule("test-module")
			if tt.initError != nil {
				module.setInitError(tt.initError)
			}
			if err := app.AddModule(module); err != nil {
				t.Fatalf("Failed to add module: %v", err)
			}

			err := app.Start()

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
