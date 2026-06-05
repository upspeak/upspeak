package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// HTTPHandler defines an HTTP route handler for a module.
type HTTPHandler struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// MsgHandler defines a message subscription handler.
// The handler function receives the subject and raw message data.
type MsgHandler struct {
	Subject string
	Handler func(subject string, data []byte)
}

// Publisher is the interface for publishing messages to the event bus.
// Implementations are provided by infrastructure modules (e.g. nats).
type Publisher interface {
	// Publish sends raw bytes on a subject.
	Publish(subject string, data []byte) error
	// PublishEvent builds a core.Event envelope for the given type, repo, and
	// payload and publishes it on the event's canonical subject. The envelope
	// shape and subject scheme are owned by core; the implementation (nats)
	// keeps every producer on the same wire format, so modules never marshal
	// events themselves.
	PublishEvent(eventType core.EventType, repoID uuid.UUID, payload any) error
}

// Subscriber is the interface for subscribing to messages from the event bus.
// Implementations are provided by infrastructure modules (e.g. nats).
type Subscriber interface {
	Subscribe(subject string, handler func(subject string, data []byte)) error
}

// Msg represents a message received from a durable consumer that requires
// explicit acknowledgement. Modules use this type to process work queue
// messages without importing NATS packages directly.
type Msg struct {
	Subject string
	Data    []byte
	ack     func() error
	nak     func() error
	inProg  func() error
	term    func() error
}

// NewMsg creates a Msg with the given subject, data, and acknowledgement functions.
// This is called by infrastructure implementations (e.g. nats), not by modules.
func NewMsg(subject string, data []byte, ack, nak, inProg, term func() error) *Msg {
	return &Msg{
		Subject: subject,
		Data:    data,
		ack:     ack,
		nak:     nak,
		inProg:  inProg,
		term:    term,
	}
}

// Ack acknowledges the message, removing it from the work queue.
func (m *Msg) Ack() error { return m.ack() }

// Nak negatively acknowledges the message, requesting redelivery.
func (m *Msg) Nak() error { return m.nak() }

// InProgress signals the server that the message is still being processed,
// resetting the ack-wait timer.
func (m *Msg) InProgress() error { return m.inProg() }

// Term terminates delivery of the message. The server will not redeliver it.
func (m *Msg) Term() error { return m.term() }

// ErrFetchTimeout is returned by Consumer.Fetch when no messages are available
// within the requested timeout. Callers should check errors.Is(err, ErrFetchTimeout)
// rather than relying on infrastructure-specific error types.
var ErrFetchTimeout = errors.New("fetch timeout: no messages available")

// Consumer is the interface for consuming messages from a durable work queue.
// Implementations are provided by infrastructure modules (e.g. nats).
type Consumer interface {
	// Fetch retrieves up to maxMsgs messages, blocking until at least one is
	// available or the timeout is reached. Returns ErrFetchTimeout if no
	// messages arrive before the deadline. Each message must be acknowledged
	// via Ack() or Nak().
	Fetch(maxMsgs int, timeout time.Duration) ([]*Msg, error)
}

// AdapterRegistry resolves a connector type to its registered core.Adapter. It
// lets the job runner (and, later, the connector module) dispatch to adapters
// without importing the concrete integrations — breaking the connector↔jobs
// import cycle. The concrete registry is built in main.go from the registered
// integrations and injected via setter.
type AdapterRegistry interface {
	// AdapterFor returns the adapter registered for the connector type, and
	// false when none is registered.
	AdapterFor(connector core.ConnectorType) (core.Adapter, bool)
}

// Module is the interface that all application modules must implement.
type Module interface {
	Name() string
	Init(config map[string]any) error
	HTTPHandlers() []HTTPHandler
	MsgHandlers() []MsgHandler
}

// Runner is a long-lived background loop started by the application after module
// wiring — for example the job runner, schedule runner, rules engine, and
// realtime hub. It runs until the supplied context is cancelled. Standardising
// on this interface lets main.go start every background loop uniformly.
type Runner interface {
	Run(ctx context.Context)
}

// moduleMount represents a module and its mount path.
type moduleMount struct {
	module Module
	path   string // Normalised mount path ("" for root, "/api/v1", etc.)
}

// App is the main application container that manages modules, HTTP routing,
// and the application lifecycle.
type App struct {
	config        Config
	subscriber    Subscriber
	httpServer    *http.Server
	httpRouter    *http.ServeMux
	modules       map[string]moduleMount // Module name -> moduleMount
	rootModule    string                 // Track which module (if any) is at root
	ready         bool
	readyLock     sync.RWMutex
	modulesInited bool
}

// New creates a new App instance from a given Config.
func New(config Config) *App {
	return &App{
		config:     config,
		httpRouter: http.NewServeMux(),
		modules:    make(map[string]moduleMount),
	}
}

// SetSubscriber sets the message subscriber used for registering message handlers.
// This must be called before Start() if any module defines MsgHandlers.
func (a *App) SetSubscriber(sub Subscriber) {
	a.subscriber = sub
}

// normalizePath normalises a mount path for consistent handling.
func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}

// isReservedPath checks if a path conflicts with system endpoints.
func isReservedPath(path string) bool {
	reservedPaths := []string{"/healthz", "/readiness"}
	for _, reserved := range reservedPaths {
		if path == reserved || strings.HasPrefix(path, reserved+"/") {
			return true
		}
	}
	return false
}

// AddModuleOnPath registers a module at the specified path.
//
// Path rules:
//   - Empty string "" mounts at root (/)
//   - Leading slash is optional and will be normalised
//   - Trailing slashes are removed
//   - Only one module can be mounted at root
//   - Multiple modules can share the same non-root path
//   - Paths cannot conflict with reserved system paths (/healthz, /readiness)
//
// Examples:
//
//	app.AddModuleOnPath(&repo.Module{}, "/api/v1")
//	app.AddModuleOnPath(&filter.Module{}, "/api/v1")  // OK: shared path
func (a *App) AddModuleOnPath(module Module, path string) error {
	moduleName := module.Name()

	if _, exists := a.modules[moduleName]; exists {
		return fmt.Errorf("module %s is already registered", moduleName)
	}

	normalizedPath := normalizePath(path)

	if strings.Contains(normalizedPath, "..") {
		return fmt.Errorf("invalid path: path traversal not allowed")
	}

	if isReservedPath(normalizedPath) {
		return fmt.Errorf("path %s conflicts with reserved system endpoint", normalizedPath)
	}

	if normalizedPath == "" {
		if a.rootModule != "" {
			return fmt.Errorf(
				"cannot mount module %s at root: module %s is already mounted at root",
				moduleName, a.rootModule,
			)
		}
		a.rootModule = moduleName
	}

	slog.Info("Adding module",
		"module", moduleName,
		"path", normalizedPath,
		"isRoot", normalizedPath == "")

	a.modules[moduleName] = moduleMount{
		module: module,
		path:   normalizedPath,
	}

	return nil
}

// AddModule registers a module at /<module.Name()>/.
// This is a convenience wrapper around AddModuleOnPath.
func (a *App) AddModule(module Module) error {
	return a.AddModuleOnPath(module, module.Name())
}

// registerModule initialises a module and registers its handlers.
func (a *App) registerModule(name string, mount moduleMount) error {
	module := mount.module

	modConfig, exists := a.config.Modules[name]

	if exists && !modConfig.Enabled {
		slog.Warn("Skipping disabled module", "module", name)
		return nil
	}

	slog.Info("Initialising module", "module", name)

	var moduleConfig map[string]any
	if exists {
		moduleConfig = modConfig.Config
	}

	if err := module.Init(moduleConfig); err != nil {
		slog.Error("Failed to initialise module", "module", name, "error", err)
		return err
	}

	// Register message handlers via subscriber.
	if a.subscriber != nil {
		for _, handler := range module.MsgHandlers() {
			slog.Info("Subscribing to subject",
				"subject", handler.Subject,
				"module", name)
			if err := a.subscriber.Subscribe(handler.Subject, handler.Handler); err != nil {
				return fmt.Errorf("failed to subscribe to %s: %w", handler.Subject, err)
			}
		}
	}

	// Register HTTP handlers.
	for _, handler := range module.HTTPHandlers() {
		fullPath := a.buildHandlerPath(mount.path, handler.Path)
		slog.Info("Registering HTTP handler",
			"path", fullPath,
			"module", name,
			"method", handler.Method)

		pattern := handler.Method + " " + fullPath
		a.httpRouter.HandleFunc(pattern, handler.Handler)
	}

	return nil
}

// buildHandlerPath constructs the full path for a handler based on the module's mount path.
func (a *App) buildHandlerPath(mountPath, handlerPath string) string {
	if mountPath == "" {
		return handlerPath
	}
	return mountPath + handlerPath
}

// InitModules initialises all registered modules and registers their HTTP and
// message handlers. Call this before Start() to ensure modules are fully wired
// before the HTTP server begins accepting requests. Safe to call multiple times;
// subsequent calls are no-ops.
func (a *App) InitModules() error {
	if a.modulesInited {
		return nil
	}
	slog.Info("Initialising modules", "name", a.config.Name)

	// Register all non-root modules first.
	for name, mount := range a.modules {
		if mount.path == "" {
			continue
		}
		if err := a.registerModule(name, mount); err != nil {
			return err
		}
	}

	// Register root module last (gives it priority for catch-all routing).
	if a.rootModule != "" {
		mount := a.modules[a.rootModule]
		if err := a.registerModule(a.rootModule, mount); err != nil {
			return err
		}
	}

	// Register health and readiness endpoints.
	slog.Info("Registering health and readiness endpoints")
	a.httpRouter.HandleFunc("GET /healthz", a.healthzHandler)
	a.httpRouter.HandleFunc("GET /readiness", a.readinessHandler)

	a.modulesInited = true
	return nil
}

// Start initialises modules (if not already done) and starts the HTTP server.
func (a *App) Start() error {
	if err := a.InitModules(); err != nil {
		return err
	}

	// Start HTTP server.
	serverErr := make(chan error, 1)
	go func() {
		a.httpServer = &http.Server{
			Addr:              fmt.Sprintf(":%d", a.config.HTTP.Port),
			Handler:           api.SecurityHeaders(api.RequestID(a.httpRouter)),
			ReadHeaderTimeout: 10 * time.Second,
		}
		slog.Info("Starting HTTP server...", "port", a.config.HTTP.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait briefly for any startup errors.
	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
	case <-time.After(200 * time.Millisecond):
	}

	a.readyLock.Lock()
	a.ready = true
	a.readyLock.Unlock()

	slog.Info("App is ready")
	return nil
}

// Stop gracefully shuts down the application.
func (a *App) Stop() error {
	slog.Info("Stopping app...")

	a.stopHttpServer()

	a.readyLock.Lock()
	a.ready = false
	a.readyLock.Unlock()

	slog.Info("App stopped")
	return nil
}

func (a *App) stopHttpServer() {
	slog.Info("Stopping HTTP server...")
	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			slog.Error("Failed to shut down HTTP server", "error", err)
		} else {
			slog.Info("HTTP server stopped.")
		}
	}
}

// healthzHandler handles health checks.
func (a *App) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// readinessHandler handles readiness probes.
func (a *App) readinessHandler(w http.ResponseWriter, r *http.Request) {
	a.readyLock.RLock()
	defer a.readyLock.RUnlock()

	if a.ready {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("NOT READY"))
	}
}
