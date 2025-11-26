package archive

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// ModuleArchive implements the app.Module interface for managing archives.
// It handles persistent storage of domain entities (Nodes, Edges, Threads, Annotations).
type ModuleArchive struct {
	archive core.Archive
	logger  *slog.Logger
}

// Config defines the configuration for the archive module.
type Config struct {
	Type string `mapstructure:"type"` // Archive type: "local" or "remote"
	Path string `mapstructure:"path"` // Path for local archive
}

// Name returns the module name.
func (m *ModuleArchive) Name() string {
	return "archive"
}

// Init initialises the archive module with the given configuration.
func (m *ModuleArchive) Init(config map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))

	var cfg Config
	if config != nil {
		if archiveType, ok := config["type"].(string); ok {
			cfg.Type = archiveType
		}
		if path, ok := config["path"].(string); ok {
			cfg.Path = path
		}
	}

	// Set defaults
	if cfg.Type == "" {
		cfg.Type = "local"
	}
	if cfg.Path == "" {
		cfg.Path = "./data"
	}

	// Create archive based on type
	switch cfg.Type {
	case "local":
		localArchive, err := NewLocalArchive(cfg.Path)
		if err != nil {
			return fmt.Errorf("failed to create local archive: %w", err)
		}
		m.archive = localArchive
		m.logger.Info("Initialised local archive", "path", cfg.Path)
	default:
		return fmt.Errorf("unsupported archive type: %s", cfg.Type)
	}

	return nil
}

// HTTPHandlers returns the HTTP handlers for the archive module.
// The archive module doesn't expose HTTP endpoints directly.
func (m *ModuleArchive) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
	return []app.HTTPHandler{}
}

// MsgHandlers returns the NATS message handlers for the archive module.
// The archive module listens for NodeDeleted events to clean up related edges.
func (m *ModuleArchive) MsgHandlers(pub app.Publisher) []app.MsgHandler {
	return []app.MsgHandler{
		{
			Subject: "repos.*.out",
			Handler: m.handleRepositoryEvents,
		},
	}
}

// GetArchive returns the archive instance managed by this module.
// This is used to inject the archive into repositories.
func (m *ModuleArchive) GetArchive() core.Archive {
	return m.archive
}

// handleRepositoryEvents processes events from repositories to maintain data integrity.
// Currently handles NodeDeleted events to cascade delete related edges.
func (m *ModuleArchive) handleRepositoryEvents(msg *nats.Msg) {
	var event core.Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		m.logger.Error("Failed to unmarshal event", "error", err)
		return
	}

	// Only handle NodeDeleted events
	if event.Type != core.EventNodeDeleted {
		return
	}

	// Unmarshal the payload
	var payload core.EventNodeDeletePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		m.logger.Error("Failed to unmarshal NodeDeleted payload", "error", err)
		return
	}

	// Get the local archive instance to access deletion methods
	localArchive, ok := m.archive.(*LocalArchive)
	if !ok {
		m.logger.Warn("Archive is not a LocalArchive, skipping edge cleanup")
		return
	}

	// Delete all edges connected to the deleted node
	if err := localArchive.DeleteEdgesByNode(payload.NodeId); err != nil {
		m.logger.Error("Failed to delete edges for deleted node",
			"node_id", payload.NodeId.String(),
			"error", err)
		return
	}

	m.logger.Info("Cleaned up edges for deleted node",
		"node_id", payload.NodeId.String())
}
