package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements the app.Module interface for filter management.
// It exposes HTTP endpoints for filter CRUD and the filter test endpoint.
type Module struct {
	archive core.Archive
	pub     app.Publisher
}

// Name returns the module name.
func (m *Module) Name() string {
	return "filter"
}

// Init initialises the filter module.
func (m *Module) Init(_ map[string]any) error {
	slog.Info("Initialised filter module")
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

// HTTPHandlers returns the HTTP handlers for the filter module.
// All paths are relative to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		// Filter collection endpoints.
		{Method: "POST", Path: "/repos/{repo_ref}/filters", Handler: m.createFilterHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/filters", Handler: m.listFiltersHandler()},

		// Filter test endpoint (via explicit /filters/{filter_ref}/test path).
		{Method: "POST", Path: "/repos/{repo_ref}/filters/{filter_ref}/test", Handler: m.testFilterHandler()},
	}
}

// MsgHandlers returns the message handlers for the filter module.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{}
}

// defaultOwnerID is a placeholder until authentication is implemented.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// createFilterRequest is the expected JSON body for POST /repos/{repo_ref}/filters.
type createFilterRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Mode        core.FilterMode  `json:"mode"`
	Conditions  []core.Condition `json:"conditions"`
}

// createFilterHandler handles POST /api/v1/repos/{repo_ref}/filters.
func (m *Module) createFilterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createFilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
			return
		}
		if req.Mode == "" {
			req.Mode = core.FilterModeAll
		}
		if req.Mode != core.FilterModeAll && req.Mode != core.FilterModeAny {
			api.WriteError(w, http.StatusBadRequest, "invalid_field", "mode must be 'all' or 'any'")
			return
		}

		if err := core.ValidateConditions(req.Conditions); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_conditions", err.Error())
			return
		}

		filter := &core.Filter{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Name:        req.Name,
			Description: req.Description,
			Mode:        req.Mode,
			Conditions:  req.Conditions,
			CreatedBy:   defaultOwnerID,
		}

		if err := m.archive.SaveFilter(filter); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create filter")
			return
		}

		m.publishEvent(repo.ID, core.EventFilterCreated, core.EventFilterPayload{Filter: filter})

		api.SetETag(w, filter.Version)
		api.WriteJSON(w, http.StatusCreated, filter)
	}
}

// listFiltersHandler handles GET /api/v1/repos/{repo_ref}/filters.
func (m *Module) listFiltersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.FilterListOptions{ListOptions: api.ParsePagination(r)}
		filters, total, err := m.archive.ListFilters(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list filters")
			return
		}

		api.WriteList(w, filters, total, opts.ListOptions)
	}
}

// testFilterHandler handles POST /api/v1/repos/{repo_ref}/filters/{filter_ref}/test.
func (m *Module) testFilterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		filterRef := r.PathValue("filter_ref")
		filter, err := m.resolveFilter(w, repo.ID, filterRef)
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		result := Evaluate(filter, payload)
		api.WriteJSON(w, http.StatusOK, result)
	}
}

// resolveRepo resolves a repo_ref and writes an error response if resolution fails.
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

// resolveFilter resolves a filter ref (short ID or UUID) within a repo.
// Returns 404 if the filter does not exist or does not belong to the given repo.
func (m *Module) resolveFilter(w http.ResponseWriter, repoID uuid.UUID, ref string) (*core.Filter, error) {
	// Try as UUID first.
	if id, err := uuid.Parse(ref); err == nil {
		f, err := m.archive.GetFilter(id)
		if err != nil || f.RepoID != repoID {
			api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
			if err != nil {
				return nil, err
			}
			return nil, errors.New("filter not found in this repository")
		}
		return f, nil
	}

	// Try as short ID via ResolveRef.
	filterID, entityType, err := m.archive.ResolveRef(repoID, ref)
	if err != nil || entityType != "filter" {
		api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
		return nil, fmt.Errorf("filter not found: %w", err)
	}

	f, err := m.archive.GetFilter(filterID)
	if err != nil || f.RepoID != repoID {
		api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
		if err != nil {
			return nil, err
		}
		return nil, errors.New("filter not found in this repository")
	}

	return f, nil
}

// publishEvent publishes an event via the configured publisher. It delegates
// envelope construction to app.Publisher.PublishEvent so every module emits
// events in the same core.Event shape, consumed uniformly by the rules engine
// and the realtime fan-out.
func (m *Module) publishEvent(repoID uuid.UUID, eventType core.EventType, payload any) {
	if m.pub == nil {
		return
	}
	if err := m.pub.PublishEvent(eventType, repoID, payload); err != nil {
		slog.Error("Failed to publish event", "type", eventType, "error", err)
	}
}
