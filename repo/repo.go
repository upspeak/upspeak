package repo

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/rs/xid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// ModuleRepo implements the app.Module interface for managing repositories.
// It exposes HTTP endpoints for repository operations and handles NATS messaging
// for event-driven communication with archives and other modules.
type ModuleRepo struct {
	repos  map[string]*core.Repository // Map of repo ID to Repository
	logger *slog.Logger
}

// Config defines the configuration for the repo module.
type Config struct {
	RepoID   string `mapstructure:"repo_id"`   // Repository ID
	RepoName string `mapstructure:"repo_name"` // Repository name
}

// Name returns the module name.
func (m *ModuleRepo) Name() string {
	return "repo"
}

// Init initialises the repo module with the given configuration.
// For now, it creates a single repository. The archive will be set up
// by the archive module.
func (m *ModuleRepo) Init(config map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	m.repos = make(map[string]*core.Repository)

	var cfg Config
	if config != nil {
		// Parse configuration
		if repoID, ok := config["repo_id"].(string); ok {
			cfg.RepoID = repoID
		}
		if repoName, ok := config["repo_name"].(string); ok {
			cfg.RepoName = repoName
		}
	}

	// Set defaults if not provided
	if cfg.RepoID == "" {
		cfg.RepoID = xid.New().String()
	}
	if cfg.RepoName == "" {
		cfg.RepoName = "default"
	}

	// Parse repo ID
	repoID, err := xid.FromString(cfg.RepoID)
	if err != nil {
		return fmt.Errorf("invalid repo_id: %w", err)
	}

	// Create repository with nil archive for now
	// The archive will be set via SetArchive after the archive module is initialised
	repo := core.NewRepository(repoID, cfg.RepoName, nil)
	m.repos[repoID.String()] = repo

	m.logger.Info("Initialised repo module",
		"repo_id", repo.ID.String(),
		"repo_name", repo.Name)

	return nil
}

// SetArchive sets the archive for all repositories.
// This should be called after the archive module is initialised.
func (m *ModuleRepo) SetArchive(archive core.Archive) {
	for id, repo := range m.repos {
		m.repos[id] = core.NewRepository(repo.ID, repo.Name, archive)
		m.logger.Info("Archive set for repository", "repo_id", id)
	}
}

// GetRepository returns a repository by its ID.
func (m *ModuleRepo) GetRepository(repoID string) (*core.Repository, error) {
	repo, exists := m.repos[repoID]
	if !exists {
		return nil, fmt.Errorf("repository not found: %s", repoID)
	}
	return repo, nil
}

// ListRepositories returns all repository IDs and names.
func (m *ModuleRepo) ListRepositories() map[string]string {
	result := make(map[string]string)
	for id, repo := range m.repos {
		result[id] = repo.Name
	}
	return result
}

// validateRepoID validates the repo ID from the request path and returns the repository.
func (m *ModuleRepo) validateRepoID(repoID string) (*core.Repository, error) {
	if repoID == "" {
		return nil, fmt.Errorf("repository ID is required")
	}

	repo, err := m.GetRepository(repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	return repo, nil
}

// HTTPHandlers returns the HTTP handlers for the repo module.
// All endpoints are namespaced under /repos/{repo_id}/
func (m *ModuleRepo) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
	return []app.HTTPHandler{
		// List repositories
		{
			Method:  "GET",
			Path:    "/",
			Handler: m.listReposHandler(),
		},
		// Node endpoints
		{
			Method:  "POST",
			Path:    "/{repo_id}/nodes",
			Handler: m.createNodeHandler(pub),
		},
		{
			Method:  "GET",
			Path:    "/{repo_id}/nodes/{id}",
			Handler: m.getNodeHandler(),
		},
		{
			Method:  "PUT",
			Path:    "/{repo_id}/nodes/{id}",
			Handler: m.updateNodeHandler(pub),
		},
		{
			Method:  "DELETE",
			Path:    "/{repo_id}/nodes/{id}",
			Handler: m.deleteNodeHandler(pub),
		},
		// Edge endpoints
		{
			Method:  "POST",
			Path:    "/{repo_id}/edges",
			Handler: m.createEdgeHandler(pub),
		},
		{
			Method:  "GET",
			Path:    "/{repo_id}/edges/{id}",
			Handler: m.getEdgeHandler(),
		},
		{
			Method:  "PUT",
			Path:    "/{repo_id}/edges/{id}",
			Handler: m.updateEdgeHandler(pub),
		},
		{
			Method:  "DELETE",
			Path:    "/{repo_id}/edges/{id}",
			Handler: m.deleteEdgeHandler(pub),
		},
		// Thread endpoints
		{
			Method:  "POST",
			Path:    "/{repo_id}/threads",
			Handler: m.createThreadHandler(pub),
		},
		{
			Method:  "GET",
			Path:    "/{repo_id}/threads/{id}",
			Handler: m.getThreadHandler(),
		},
		{
			Method:  "PUT",
			Path:    "/{repo_id}/threads/{id}",
			Handler: m.updateThreadHandler(pub),
		},
		{
			Method:  "DELETE",
			Path:    "/{repo_id}/threads/{id}",
			Handler: m.deleteThreadHandler(pub),
		},
	}
}

// MsgHandlers returns the NATS message handlers for the repo module.
func (m *ModuleRepo) MsgHandlers(pub app.Publisher) []app.MsgHandler {
	var handlers []app.MsgHandler

	// Create a handler for each repository
	for repoID := range m.repos {
		inSubject := fmt.Sprintf("repo.%s.in", repoID)
		handlers = append(handlers, app.MsgHandler{
			Subject: inSubject,
			Handler: m.handleInputEvent(pub, repoID),
		})
	}

	return handlers
}

// listReposHandler handles GET /repos
func (m *ModuleRepo) listReposHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repos := m.ListRepositories()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(repos)
	}
}

// handleInputEvent processes incoming events from NATS and publishes output events.
func (m *ModuleRepo) handleInputEvent(pub app.Publisher, repoID string) func(msg *nats.Msg) {
	outSubject := fmt.Sprintf("repo.%s.out", repoID)

	return func(msg *nats.Msg) {
		var inputEvent core.Event
		if err := json.Unmarshal(msg.Data, &inputEvent); err != nil {
			m.logger.Error("Failed to unmarshal input event", "error", err)
			return
		}

		m.logger.Info("Processing input event",
			"repo_id", repoID,
			"event_type", inputEvent.Type,
			"event_id", inputEvent.ID.String())

		// Get the repository
		repo, err := m.GetRepository(repoID)
		if err != nil {
			m.logger.Error("Repository not found", "repo_id", repoID, "error", err)
			return
		}

		// Process the event using the repository
		outputEvent, err := repo.HandleInputEvent(inputEvent)
		if err != nil {
			m.logger.Error("Failed to process input event",
				"repo_id", repoID,
				"error", err,
				"event_type", inputEvent.Type)
			return
		}

		// Publish the output event
		outputData, err := json.Marshal(outputEvent)
		if err != nil {
			m.logger.Error("Failed to marshal output event", "error", err)
			return
		}

		if err := pub.Publish(outSubject, outputData); err != nil {
			m.logger.Error("Failed to publish output event", "error", err)
			return
		}

		m.logger.Info("Published output event",
			"repo_id", repoID,
			"event_type", outputEvent.Type,
			"event_id", outputEvent.ID.String())
	}
}
