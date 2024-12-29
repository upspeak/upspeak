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
	app.modules = make(map[string]Module)

	// Test adding a module
	module := newMockModule("test-module")
	app.AddModule(module)

	if len(app.modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(app.modules))
	}

	if app.modules["test-module"] != module {
		t.Error("Module not properly registered")
	}
}

func TestModuleInitialization(t *testing.T) {
	tests := []struct {
		name        string
		moduleConfig map[string]ModuleConfig
		initError   error
		wantError   bool
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
			app.modules = make(map[string]Module)

			module := newMockModule("test-module")
			if tt.initError != nil {
				module.setInitError(tt.initError)
			}
			app.AddModule(module)

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
