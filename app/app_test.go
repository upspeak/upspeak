package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nats-io/nats.go"
)

// mockModule implements the Module interface for testing
type mockModule struct {
	name         string
	initFunc     func(config map[string]any) error
	httpHandlers []HTTPHandler
	msgHandlers  []MsgHandler
	initCalled   bool
	initConfig   map[string]any
}

func newMockModule(name string) *mockModule {
	return &mockModule{
		name: name,
		initFunc: func(config map[string]any) error {
			return nil
		},
	}
}

func (m *mockModule) Name() string {
	return m.name
}

func (m *mockModule) Init(config map[string]any) error {
	m.initCalled = true
	m.initConfig = config
	return m.initFunc(config)
}

func (m *mockModule) HTTPHandlers(pub Publisher) []HTTPHandler {
	return m.httpHandlers
}

func (m *mockModule) MsgHandlers(pub Publisher) []MsgHandler {
	return m.msgHandlers
}

func (m *mockModule) setInitError(err error) {
	m.initFunc = func(config map[string]any) error {
		return err
	}
}

func (m *mockModule) addHTTPHandler(method, path string, handler http.HandlerFunc) {
	m.httpHandlers = append(m.httpHandlers, HTTPHandler{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

func (m *mockModule) addMsgHandler(subject string, handler func(msg *nats.Msg)) {
	m.msgHandlers = append(m.msgHandlers, MsgHandler{
		Subject: subject,
		Handler: handler,
	})
}

func TestNew(t *testing.T) {
	config := Config{
		Name: "test-app",
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
		HTTP: HTTPConfig{
			Port: 8080,
		},
	}

	app := New(config)
	if app == nil {
		t.Fatal("Expected non-nil app")
	}
	if app.config.Name != "test-app" {
		t.Errorf("Expected app name to be 'test-app', got %s", app.config.Name)
	}
}

func TestHealthzHandler(t *testing.T) {
	app := &App{}

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	app.healthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", w.Body.String())
	}
}

func TestReadinessHandler(t *testing.T) {
	app := &App{}

	tests := []struct {
		name       string
		ready      bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "not ready",
			ready:      false,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "NOT READY",
		},
		{
			name:       "ready",
			ready:      true,
			wantStatus: http.StatusOK,
			wantBody:   "READY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app.readyLock.Lock()
			app.ready = tt.ready
			app.readyLock.Unlock()

			req := httptest.NewRequest("GET", "/readiness", nil)
			w := httptest.NewRecorder()

			app.readinessHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status code %d, got %d", tt.wantStatus, w.Code)
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("Expected body '%s', got '%s'", tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestHTTPServerFailure(t *testing.T) {
	// Create two apps trying to use the same port
	config := Config{
		Name: "test-app",
		HTTP: HTTPConfig{
			Port: 8082,
		},
		NATS: NATSConfig{
			Embedded: true,
			Private:  true,
		},
	}

	app1 := New(config)
	app2 := New(config)

	// Start first app
	if err := app1.Start(); err != nil {
		t.Fatalf("Failed to start first app: %v", err)
	}

	// Try to start second app on same port
	if err := app2.Start(); err == nil {
		t.Error("Expected error when starting second app on same port")
	}

	// Cleanup
	app1.Stop()
	app2.Stop()
}
