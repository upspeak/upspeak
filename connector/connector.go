// Package connector implements the app.Module for managing sources (content
// collection endpoints) and sinks (content publishing endpoints).
package connector

import (
	"context"
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

// Module implements app.Module for connector management (sources and sinks).
type Module struct {
	archive     core.Archive
	pub         app.Publisher
	rateLimiter *RateLimiter
}

// Name returns the module name.
func (m *Module) Name() string { return "connector" }

// Init initialises the connector module.
func (m *Module) Init(_ map[string]any) error {
	m.rateLimiter = NewRateLimiter()
	m.rateLimiter.StartCleanup(context.Background())
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) { m.archive = archive }

// SetPublisher injects the event publisher dependency.
func (m *Module) SetPublisher(pub app.Publisher) { m.pub = pub }

// HTTPHandlers returns all HTTP route handlers for sources and sinks.
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		// Sources
		{Method: "POST", Path: "/repos/{repo_ref}/sources", Handler: m.createSourceHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sources", Handler: m.listSourcesHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sources/{source_ref}", Handler: m.getSourceHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}/sources/{source_ref}", Handler: m.updateSourceHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}/sources/{source_ref}", Handler: m.deleteSourceHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/sources/{source_ref}/collect", Handler: m.triggerCollectHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sources/{source_ref}/history", Handler: m.sourceHistoryHandler()},
		// Sinks
		{Method: "POST", Path: "/repos/{repo_ref}/sinks", Handler: m.createSinkHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sinks", Handler: m.listSinksHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sinks/{sink_ref}", Handler: m.getSinkHandler()},
		{Method: "PUT", Path: "/repos/{repo_ref}/sinks/{sink_ref}", Handler: m.updateSinkHandler()},
		{Method: "DELETE", Path: "/repos/{repo_ref}/sinks/{sink_ref}", Handler: m.deleteSinkHandler()},
		{Method: "POST", Path: "/repos/{repo_ref}/sinks/{sink_ref}/publish", Handler: m.triggerPublishHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/sinks/{sink_ref}/history", Handler: m.sinkHistoryHandler()},
		// One-shot collect
		{Method: "POST", Path: "/repos/{repo_ref}/collect", Handler: m.oneShotCollectHandler()},
	}
}

// MsgHandlers returns message handlers. None are needed for the connector module.
func (m *Module) MsgHandlers() []app.MsgHandler { return []app.MsgHandler{} }

// publishEvent delegates domain event publication to the injected Publisher.
// The Publisher is responsible for building the core.Event envelope and
// persisting it to the NATS JetStream stream.
func (m *Module) publishEvent(repoID uuid.UUID, eventType core.EventType, payload any) {
	if m.pub == nil {
		return
	}
	if err := m.pub.PublishEvent(eventType, repoID, payload); err != nil {
		slog.Error("Failed to publish event", "type", eventType, "error", err)
	}
}

// resolveRepo resolves a repository reference (UUID, short ID, or slug) and
// writes appropriate HTTP error responses on failure.
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

// resolveSource resolves a source reference (UUID or short ID) and verifies it
// belongs to the given repository.
func (m *Module) resolveSource(w http.ResponseWriter, repoID uuid.UUID, ref string) (*core.Source, error) {
	// Try UUID parse first.
	if id, err := uuid.Parse(ref); err == nil {
		source, err := m.archive.GetSource(id)
		if err != nil {
			var notFound *core.ErrorNotFound
			if errors.As(err, &notFound) {
				api.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
				return nil, err
			}
			api.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get source")
			return nil, err
		}
		if source.RepoID != repoID {
			api.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return nil, errors.New("source does not belong to repository")
		}
		return source, nil
	}

	// Try short ID resolution via ResolveRef.
	resolvedID, entityType, err := m.archive.ResolveRef(repoID, ref)
	if err != nil {
		var notFound *core.ErrorNotFound
		if errors.As(err, &notFound) {
			api.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return nil, err
		}
		api.WriteError(w, http.StatusInternalServerError, "resolve_failed", "Failed to resolve source reference")
		return nil, err
	}
	if entityType != "source" {
		api.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
		return nil, errors.New("reference does not resolve to a source")
	}

	source, err := m.archive.GetSource(resolvedID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get source")
		return nil, err
	}
	return source, nil
}

// resolveSink resolves a sink reference (UUID or short ID) and verifies it
// belongs to the given repository.
func (m *Module) resolveSink(w http.ResponseWriter, repoID uuid.UUID, ref string) (*core.Sink, error) {
	// Try UUID parse first.
	if id, err := uuid.Parse(ref); err == nil {
		sink, err := m.archive.GetSink(id)
		if err != nil {
			var notFound *core.ErrorNotFound
			if errors.As(err, &notFound) {
				api.WriteError(w, http.StatusNotFound, "not_found", "Sink not found")
				return nil, err
			}
			api.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get sink")
			return nil, err
		}
		if sink.RepoID != repoID {
			api.WriteError(w, http.StatusNotFound, "not_found", "Sink not found")
			return nil, errors.New("sink does not belong to repository")
		}
		return sink, nil
	}

	// Try short ID resolution via ResolveRef.
	resolvedID, entityType, err := m.archive.ResolveRef(repoID, ref)
	if err != nil {
		var notFound *core.ErrorNotFound
		if errors.As(err, &notFound) {
			api.WriteError(w, http.StatusNotFound, "not_found", "Sink not found")
			return nil, err
		}
		api.WriteError(w, http.StatusInternalServerError, "resolve_failed", "Failed to resolve sink reference")
		return nil, err
	}
	if entityType != "sink" {
		api.WriteError(w, http.StatusNotFound, "not_found", "Sink not found")
		return nil, errors.New("reference does not resolve to a sink")
	}

	sink, err := m.archive.GetSink(resolvedID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get sink")
		return nil, err
	}
	return sink, nil
}
