package repo

import (
	"log/slog"
	"os"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements the app.Module interface for repository and knowledge graph
// management. It exposes HTTP endpoints for repository CRUD and entity operations
// (nodes, edges, threads, annotations) with flat URL routing.
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
		// Repository CRUD.
		{Method: "POST", Path: "/repos", Handler: m.createRepoHandler()},
		{Method: "GET", Path: "/repos", Handler: m.listReposHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}", Handler: m.getRepoHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}", Handler: m.updateRepoHandler()},
		{Method: "PATCH", Path: "/repos/{repo_ref}", Handler: m.patchRepoHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}", Handler: m.deleteRepoHandler()},

		// Node collection endpoints.
		{Method: "POST", Path: "/repos/{repo_ref}/nodes", Handler: m.createNodeHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/nodes/batch", Handler: m.batchCreateNodesHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/nodes", Handler: m.listNodesHandler()},

		// Edge collection endpoints.
		{Method: "POST", Path: "/repos/{repo_ref}/edges", Handler: m.createEdgeHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/edges/batch", Handler: m.batchCreateEdgesHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/edges", Handler: m.listEdgesHandler()},

		// Thread collection endpoints.
		{Method: "POST", Path: "/repos/{repo_ref}/threads", Handler: m.createThreadHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/threads", Handler: m.listThreadsHandler()},

		// Annotation collection endpoints.
		{Method: "POST", Path: "/repos/{repo_ref}/annotations", Handler: m.createAnnotationHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/annotations", Handler: m.listAnnotationsHandler()},

		// Flat URL entity access (GET/PUT/PATCH/DELETE by entity ref).
		{Method: "GET", Path: "/repos/{repo_ref}/{entity_ref}", Handler: m.entityHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}/{entity_ref}", Handler: m.entityHandler()},
		{Method: "PATCH", Path: "/repos/{repo_ref}/{entity_ref}", Handler: m.entityHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}/{entity_ref}", Handler: m.entityHandler()},

		// Entity sub-resources (e.g. /NODE-42/edges, /THREAD-7/nodes).
		{Method: "GET", Path: "/repos/{repo_ref}/{entity_ref}/{sub}", Handler: m.entitySubHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/{entity_ref}/{sub}", Handler: m.entitySubHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}/{entity_ref}/{sub}", Handler: m.threadNodeDeleteHandler()},
	}
}

// MsgHandlers returns the message handlers for the repo module.
// Cascading delete handlers will be added when JetStream consumers are wired.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{}
}
