package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type HTTPHandler struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

type MsgHandler struct {
	Subject string
	Handler func(msg *nats.Msg)
}

type Publisher struct {
	nc *nats.Conn
}

func (p *Publisher) Publish(subject string, data []byte) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
	}
	return p.nc.PublishMsg(msg)
}

type Module interface {
	Name() string
	Init(config map[string]any) error
	HTTPHandlers(pub Publisher) []HTTPHandler
	MsgHandlers(pub Publisher) []MsgHandler
}

// moduleMount represents a module and its mount path
type moduleMount struct {
	module Module
	path   string // Normalized mount path ("" for root, "/api", "/writer", etc.)
}

type App struct {
	config     Config
	nc         *nats.Conn
	ns         *natsserver.Server
	httpServer *http.Server
	httpRouter *http.ServeMux
	modules    map[string]moduleMount // Module name -> moduleMount
	rootModule string                 // Track which module (if any) is at root
	logger     *slog.Logger
	ready      bool
	readyLock  sync.RWMutex
}

// Create a new App instance from a given Config
func New(config Config) *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return &App{
		config:     config,
		httpRouter: http.NewServeMux(),
		logger:     logger,
		modules:    make(map[string]moduleMount),
	}
}

// normalizePath normalizes a mount path for consistent handling
func normalizePath(path string) string {
	// Trim whitespace
	path = strings.TrimSpace(path)

	// Empty string means root
	if path == "" {
		return ""
	}

	// Ensure leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Remove trailing slash (except for root)
	path = strings.TrimSuffix(path, "/")

	return path
}

// isReservedPath checks if a path conflicts with system endpoints
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
//   - Leading slash is optional and will be normalized
//   - Trailing slashes are removed
//   - Only one module can be mounted at root
//   - Paths cannot conflict with reserved system paths (/healthz, /readiness)
//
// Examples:
//
//	app.AddModuleOnPath(&ui.Module{}, "")         // Root: /
//	app.AddModuleOnPath(&api.Module{}, "/api")    // Namespaced: /api/*
//	app.AddModuleOnPath(&writer.Module{}, "v1")   // Namespaced: /v1/*
func (a *App) AddModuleOnPath(module Module, path string) error {
	moduleName := module.Name()

	// Check if module already registered
	if _, exists := a.modules[moduleName]; exists {
		return fmt.Errorf("module %s is already registered", moduleName)
	}

	// Normalize the path
	normalizedPath := normalizePath(path)

	// Prevent path traversal attempts
	if strings.Contains(normalizedPath, "..") {
		return fmt.Errorf("invalid path: path traversal not allowed")
	}

	// Check for reserved path conflicts
	if isReservedPath(normalizedPath) {
		return fmt.Errorf("path %s conflicts with reserved system endpoint", normalizedPath)
	}

	// Check root mounting restriction
	if normalizedPath == "" {
		if a.rootModule != "" {
			return fmt.Errorf(
				"cannot mount module %s at root: module %s is already mounted at root",
				moduleName, a.rootModule,
			)
		}
		a.rootModule = moduleName
	}

	// Check for exact path conflicts with other modules
	for name, mount := range a.modules {
		if mount.path == normalizedPath {
			return fmt.Errorf(
				"path conflict: module %s is already mounted at %s",
				name, normalizedPath,
			)
		}
	}

	a.logger.Info("Adding module",
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
	// Use module name as the path
	return a.AddModuleOnPath(module, module.Name())
}

// registerModule initializes a module and registers its handlers
func (a *App) registerModule(name string, mount moduleMount, pub Publisher) error {
	module := mount.module

	// Check if module is disabled in config
	modConfig, exists := a.config.Modules[name]
	if exists && !modConfig.Enabled {
		a.logger.Warn("Skipping disabled module", "module", name)
		return nil
	}

	a.logger.Info("Initializing module", "module", name)

	// Initialize module
	if err := module.Init(modConfig.Config); err != nil {
		a.logger.Error("Failed to initialize module", "module", name, "error", err)
		return err
	}

	// Register NATS subscribers
	for _, handler := range module.MsgHandlers(pub) {
		a.logger.Info("Subscribing to NATS subject",
			"subject", handler.Subject,
			"module", name)
		if _, err := a.nc.Subscribe(handler.Subject, handler.Handler); err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", handler.Subject, err)
		}
	}

	// Register HTTP handlers
	for _, handler := range module.HTTPHandlers(pub) {
		fullPath := a.buildHandlerPath(mount.path, handler.Path)
		a.logger.Info("Registering HTTP handler",
			"path", fullPath,
			"module", name,
			"method", handler.Method)

		// Register handler with method pattern
		pattern := handler.Method + " " + fullPath
		a.httpRouter.HandleFunc(pattern, handler.Handler)
	}

	return nil
}

// buildHandlerPath constructs the full path for a handler based on the module's mount path
func (a *App) buildHandlerPath(mountPath, handlerPath string) string {
	if mountPath == "" {
		// Root mounting: use handler path as-is
		return handlerPath
	}

	// Namespaced mounting: concatenate mount path + handler path
	return mountPath + handlerPath
}

func (a *App) Start() error {
	a.logger.Info("Starting app", "name", a.config.Name)

	// 1 - Start NATS and/or initiate NATS connection
	a.logger.Info("Setting up NATS", "embedded", a.config.NATS.Embedded)
	if err := a.startNats(); err != nil {
		return err
	}

	// 2 - Bootstrap modules
	pub := Publisher{nc: a.nc}

	// First pass: Register all non-root modules
	for name, mount := range a.modules {
		if mount.path == "" {
			continue // Skip root module for now
		}
		if err := a.registerModule(name, mount, pub); err != nil {
			return err
		}
	}

	// Second pass: Register root module last (gives it priority for catch-all routing)
	if a.rootModule != "" {
		mount := a.modules[a.rootModule]
		if err := a.registerModule(a.rootModule, mount, pub); err != nil {
			return err
		}
	}

	// 3 - Register health and readiness endpoints
	a.logger.Info("Registering health and readiness endpoints")
	a.httpRouter.HandleFunc("/healthz", a.healthzHandler)
	a.httpRouter.HandleFunc("/readiness", a.readinessHandler)

	// 4 - Start HTTP server
	serverErr := make(chan error, 1)
	go func() {
		a.httpServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", a.config.HTTP.Port),
			Handler: a.httpRouter,
		}
		a.logger.Info("Starting HTTP server...", "port", a.config.HTTP.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server failed", "error", err)
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait briefly for any startup errors
	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
	case <-time.After(200 * time.Millisecond):
		// If no error after 200ms, server likely started successfully
	}

	// Mark the app as ready
	a.readyLock.Lock()
	a.ready = true
	a.readyLock.Unlock()

	a.logger.Info("App is ready")

	return nil
}

// Stop gracefully shuts down the application.
func (a *App) Stop() error {
	a.logger.Info("Stopping app...")

	// Stop HTTP server
	a.stopHttpServer()

	// Stop NATS
	a.stopNats()

	// Mark the app as not ready
	a.readyLock.Lock()
	a.ready = false
	a.readyLock.Unlock()

	// Existing shutdown logic...
	a.logger.Info("App stopped")

	return nil
}

func (a *App) startNats() error {
	// Setup and connect to NATS
	if a.config.NATS.Embedded {
		// Start NATS server, if using embedded mode
		ns, err := startEmbeddedNatsServer(a.config.Name, a.config.NATS)
		a.ns = ns
		if err != nil {
			return fmt.Errorf("error starting embedded NATS server: %w", err)
		}
		a.logger.Info("Started embedded NATS Server.", "name", a.config.Name)
		// Connect to the embedded NATS server
		nc, err := connectToEmbeddedNATS(a.config.Name, ns, a.config.NATS)
		if err != nil {
			return fmt.Errorf("error connecting to embedded NATS server: %w", err)
		}
		a.logger.Info("Connected to embedded NATS server.", "private", a.config.NATS.Private)
		a.nc = nc
	} else {
		// Connect to NATS server, if using remote mode
		nc, err := connectToExternalNATS(a.config.NATS)
		if err != nil {
			return fmt.Errorf("error connecting to NATS server: %w", err)
		}
		a.logger.Info("Connected to external NATS server.", "url", a.config.NATS.URL)
		a.nc = nc
	}
	return nil
}

func (a *App) stopNats() {
	a.logger.Info("Stopping NATS...")

	// Close NATS connection
	if a.nc != nil {
		a.nc.Close()
		a.logger.Info("NATS connection closed.")
	}

	// Close NATS server, if in embedded mode
	if a.ns != nil {
		a.ns.Shutdown()
		a.ns.WaitForShutdown()
		a.logger.Info("NATS server stopped.")
	}
}

func (a *App) stopHttpServer() {
	a.logger.Info("Stopping HTTP server...")
	// Shutdown HTTP server
	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.logger.Error("Failed to shut down HTTP server", "error", err)
		} else {
			a.logger.Info("HTTP server stopped.")
		}
	}
}

// healthzHandler handles health checks
func (a *App) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// readinessHandler handles readiness probes
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
