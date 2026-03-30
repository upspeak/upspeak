package repo

import (
	"log/slog"
	"os"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements the app.Module interface for repository management.
// It exposes HTTP endpoints for repository CRUD and will be extended in
// Phase 2 for knowledge graph operations (nodes, edges, threads, annotations).
type Module struct {
	archive core.Archive
	pub     app.Publisher
	logger  *slog.Logger
}

// Name returns the module name.
func (m *Module) Name() string {
	return "repo"
}

// Init initialises the repo module.
func (m *Module) Init(config map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	m.logger.Info("Initialised repo module")
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) {
	m.archive = archive
}

// SetPublisher injects the publisher dependency.
func (m *Module) SetPublisher(pub app.Publisher) {
	m.pub = pub
}

// HTTPHandlers returns the HTTP handlers for the repo module.
// All paths are relative to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		// Repository CRUD
		{Method: "POST", Path: "/repos", Handler: m.createRepoHandler()},
		{Method: "GET", Path: "/repos", Handler: m.listReposHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}", Handler: m.getRepoHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}", Handler: m.updateRepoHandler()},
		{Method: "PATCH", Path: "/repos/{repo_ref}", Handler: m.patchRepoHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}", Handler: m.deleteRepoHandler()},
	}
}

// MsgHandlers returns the message handlers for the repo module.
// Event-driven handlers will be added in Phase 2.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{}
}
