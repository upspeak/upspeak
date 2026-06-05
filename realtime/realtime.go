package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module is the real-time WebSocket module. It subscribes once to the
// repo.*.events.> fan-out subject and pushes matching domain events to connected
// clients over a single /api/v1/ws endpoint. Per-subscription filtering happens
// in-process (see hub and subscription), so there is no JetStream consumer per
// subscription.
type Module struct {
	logger   *slog.Logger
	hub      *hub
	auth     Authenticator
	resolver refResolver
}

// New creates an uninitialised realtime module. Call Init before use, then wire
// dependencies via SetArchive (and optionally SetAuthenticator).
func New() *Module {
	return &Module{}
}

// Name returns the module name.
func (m *Module) Name() string {
	return "realtime"
}

// Init initialises the module's logger, hub, and default authenticator.
func (m *Module) Init(_ map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	m.hub = newHub(m.logger)
	if m.auth == nil {
		m.auth = allowAllAuthenticator{}
	}
	m.logger.Info("Initialised realtime module")
	return nil
}

// SetArchive injects the archive used to resolve channel refs to UUIDs.
func (m *Module) SetArchive(archive core.Archive) {
	m.resolver = archive
}

// SetAuthenticator overrides the default allow-all authenticator.
func (m *Module) SetAuthenticator(auth Authenticator) {
	m.auth = auth
}

// Run starts the hub dispatch loop. It blocks until ctx is cancelled and should
// be called in a background goroutine, mirroring the other module runners.
func (m *Module) Run(ctx context.Context) {
	m.hub.run(ctx)
}

// HTTPHandlers returns the WebSocket upgrade endpoint, mounted at /api/v1/ws.
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: http.MethodGet, Path: "/ws", Handler: m.handleWS},
	}
}

// MsgHandlers subscribes to every repo's event subject via core-NATS fan-out.
// Each delivered message is a marshalled core.Event handed to the hub.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{
		{Subject: "repo.*.events.>", Handler: m.handleEvent},
	}
}

// handleEvent decodes a published domain event and hands it to the hub for
// fan-out. Malformed messages are logged and dropped.
func (m *Module) handleEvent(_ string, data []byte) {
	var ev core.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		m.logger.Warn("realtime: failed to decode event", "error", err)
		return
	}
	m.hub.ingest(&ev)
}
