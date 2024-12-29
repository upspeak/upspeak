package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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

type App struct {
	config     Config
	nc         *nats.Conn
	ns         *natsserver.Server
	httpServer *http.Server
	httpRouter *http.ServeMux
	modules    map[string]Module
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
		modules:    make(map[string]Module),
	}
}

// AddModule registers a module with the app.
func (a *App) AddModule(module Module) {
	a.logger.Info("Adding module...", "module", module.Name())
	a.modules[module.Name()] = module
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

	for name, module := range a.modules {
		// 2.a - Skip any module that has been explicitly disabled
		// TODO: Validate use case to decide modules should be explicitly enabled or explicitly disabled
		modConfig, exists := a.config.Modules[name]
		if exists && !modConfig.Enabled {
			a.logger.Warn("Skipping disabled module", "module", name)
			continue
		}

		a.logger.Info("Initializing module...", "module", name)

		// 2.a - Initialize module
		if err := module.Init(modConfig.Config); err != nil {
			a.logger.Error("Failed to initialize module", "module", name, "error", err)
			return err
		}

		// 2.b - Register NATS subscribers for module
		for _, handler := range module.MsgHandlers(pub) {
			a.logger.Info("Subscribing to NATS subject", "subject", handler.Subject, "module", name)
			a.nc.Subscribe(handler.Subject, handler.Handler)
		}

		// 2.c - Register HTTP handlers for module
		for _, handler := range module.HTTPHandlers(pub) {
			// Prefix the module name to the handler path
			namespacedPath := "/" + name + handler.Path
			a.logger.Info("Registering HTTP handler", "path", namespacedPath, "module", name)
			a.httpRouter.HandleFunc(namespacedPath, handler.Handler)
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
