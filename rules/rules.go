// Package rules implements the app.Module for managing rules — event-condition-action
// triggers that react to domain events. It exposes HTTP endpoints for rule CRUD and
// execution history. The reactive evaluation of rules is handled by the rules engine
// (see engine.go), which subscribes to repository events and fires matching rules.
package rules

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// defaultOwnerID is a placeholder owner until authentication is implemented.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// Module implements app.Module for rule management.
type Module struct {
	archive core.Archive
	pub     app.Publisher
	logger  *slog.Logger
}

// Name returns the module name.
func (m *Module) Name() string { return "rules" }

// Init initialises the rules module.
func (m *Module) Init(_ map[string]any) error {
	m.logger = slog.Default().With("module", "rules")
	m.logger.Info("Initialised rules module")
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) { m.archive = archive }

// SetPublisher injects the event publisher dependency.
func (m *Module) SetPublisher(pub app.Publisher) { m.pub = pub }

// HTTPHandlers returns all HTTP route handlers for rules. All paths are relative
// to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: "POST", Path: "/repos/{repo_ref}/rules", Handler: m.createRuleHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/rules", Handler: m.listRulesHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/rules/{rule_ref}", Handler: m.getRuleHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}/rules/{rule_ref}", Handler: m.updateRuleHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}/rules/{rule_ref}", Handler: m.deleteRuleHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/rules/{rule_ref}/pause", Handler: m.pauseRuleHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/rules/{rule_ref}/resume", Handler: m.resumeRuleHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/rules/{rule_ref}/history", Handler: m.listHistoryHandler()},
	}
}

// MsgHandlers returns message handlers. The rules engine subscribes separately
// (started from main.go after dependency wiring) to avoid the archive-wiring race
// in InitModules, so the module registers none here.
func (m *Module) MsgHandlers() []app.MsgHandler { return []app.MsgHandler{} }

// publishEvent publishes a domain event to the NATS JetStream stream.
func (m *Module) publishEvent(repoID uuid.UUID, eventType core.EventType, payload any) {
	if m.pub == nil {
		return
	}
	evt, err := core.NewEvent(eventType, repoID, payload)
	if err != nil {
		m.logger.Error("Failed to create event", "error", err)
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		m.logger.Error("Failed to marshal event", "error", err)
		return
	}
	if err := m.pub.Publish(evt.Subject(), data); err != nil {
		m.logger.Error("Failed to publish event", "subject", evt.Subject(), "error", err)
	}
}

// resolveRepo resolves a repository reference (UUID, short ID, or slug) and writes
// appropriate HTTP error responses on failure.
func (m *Module) resolveRepo(w http.ResponseWriter, ref string) (*core.Repository, error) {
	repo, err := m.archive.ResolveRepoRef(defaultOwnerID, ref)
	if err != nil {
		var redirectErr *core.ErrorSlugRedirect
		if errors.As(err, &redirectErr) {
			w.Header().Set("Location", "/api/v1/repos/"+redirectErr.NewSlug)
			w.WriteHeader(http.StatusMovedPermanently)
			return nil, err
		}
		var notFound *core.ErrorNotFound
		if errors.As(err, &notFound) {
			api.WriteError(w, http.StatusNotFound, "not_found", "Repository not found")
			return nil, err
		}
		api.WriteError(w, http.StatusInternalServerError, "resolve_failed", "Failed to resolve repository reference")
		return nil, err
	}
	return repo, nil
}

// resolveRule resolves a rule reference (UUID or short ID) and verifies it belongs
// to the given repository. Writes a 404 response and returns an error on failure.
func (m *Module) resolveRule(w http.ResponseWriter, repoID uuid.UUID, ref string) (*core.Rule, error) {
	// Try UUID parse first.
	if id, err := uuid.Parse(ref); err == nil {
		rule, err := m.archive.GetRule(id)
		if err != nil || rule.RepoID != repoID {
			api.WriteError(w, http.StatusNotFound, "not_found", "Rule not found")
			if err != nil {
				return nil, err
			}
			return nil, errors.New("rule does not belong to repository")
		}
		return rule, nil
	}

	// Try short ID resolution via ResolveRef.
	resolvedID, entityType, err := m.archive.ResolveRef(repoID, ref)
	if err != nil || entityType != "rule" {
		api.WriteError(w, http.StatusNotFound, "not_found", "Rule not found")
		if err != nil {
			return nil, err
		}
		return nil, errors.New("reference does not resolve to a rule")
	}

	rule, err := m.archive.GetRule(resolvedID)
	if err != nil || rule.RepoID != repoID {
		api.WriteError(w, http.StatusNotFound, "not_found", "Rule not found")
		if err != nil {
			return nil, err
		}
		return nil, errors.New("rule does not belong to repository")
	}
	return rule, nil
}
