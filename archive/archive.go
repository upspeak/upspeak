package archive

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// ModuleArchive implements the app.Module interface for managing archives.
// It handles persistent storage of domain entities.
type ModuleArchive struct {
	archive *LocalArchive
	logger  *slog.Logger
}

// Config defines the configuration for the archive module.
type Config struct {
	Type string `mapstructure:"type"` // Archive type: "local"
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

	if cfg.Type == "" {
		cfg.Type = "local"
	}
	if cfg.Path == "" {
		cfg.Path = "./data"
	}

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
func (m *ModuleArchive) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{}
}

// MsgHandlers returns the message handlers for the archive module.
// Cascade deletion handlers will be added in Phase 2.
func (m *ModuleArchive) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{}
}

// GetArchive returns the archive instance as a core.Archive interface.
func (m *ModuleArchive) GetArchive() core.Archive {
	return m.archive
}
