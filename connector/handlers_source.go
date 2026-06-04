package connector

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/jobs"
)

// createSourceRequest is the JSON body for creating a source.
type createSourceRequest struct {
	Name            string         `json:"name"`
	Connector       string         `json:"connector"`
	Config          map[string]any `json:"config"`
	FilterIDs       []uuid.UUID    `json:"filter_ids,omitempty"`
	FilterChainMode string         `json:"filter_chain_mode,omitempty"`
	RateLimit       *core.RateLimit `json:"rate_limit,omitempty"`
}

// updateSourceRequest is the JSON body for updating a source.
type updateSourceRequest struct {
	Name            *string         `json:"name,omitempty"`
	Config          map[string]any  `json:"config,omitempty"`
	FilterIDs       *[]uuid.UUID    `json:"filter_ids,omitempty"`
	FilterChainMode *string         `json:"filter_chain_mode,omitempty"`
	RateLimit       *core.RateLimit `json:"rate_limit,omitempty"`
	Status          *string         `json:"status,omitempty"`
}

// createSourceHandler handles POST /repos/{repo_ref}/sources.
func (m *Module) createSourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", "Name is required")
			return
		}

		connectorType := core.ConnectorType(req.Connector)
		if !isSupportedConnector(connectorType) {
			api.WriteError(w, http.StatusBadRequest, "unsupported_connector", "Only webhook and repo connector types are supported")
			return
		}

		if err := m.validateSourceConfig(connectorType, req.Config); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_config", err.Error())
			return
		}

		// Cycle detection for repo connectors.
		if connectorType == core.ConnectorRepo {
			targetRepoID, _ := uuid.Parse(req.Config["repo_id"].(string))
			hasCycle, err := m.detectCycle(repo.ID, targetRepoID)
			if err != nil {
				api.WriteError(w, http.StatusInternalServerError, "cycle_check_failed", "Failed to check for cycles")
				return
			}
			if hasCycle {
				api.WriteError(w, http.StatusConflict, "cycle_detected", "Creating this source would form a circular reference")
				return
			}
		}

		filterChainMode := core.FilterModeAll
		if req.FilterChainMode == string(core.FilterModeAny) {
			filterChainMode = core.FilterModeAny
		}

		source := &core.Source{
			ID:              core.NewID(),
			RepoID:          repo.ID,
			Name:            req.Name,
			Connector:       connectorType,
			Config:          req.Config,
			FilterIDs:       req.FilterIDs,
			FilterChainMode: filterChainMode,
			RateLimit:       req.RateLimit,
			Status:          core.StatusActive,
			Version:         0,
			CreatedBy:       defaultOwnerID,
		}

		if err := m.archive.SaveSource(source); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save source")
			return
		}

		m.publishEvent(repo.ID, core.EventSourceCreated, source)
		api.SetETag(w, source.Version)
		api.WriteJSON(w, http.StatusCreated, source)
	}
}

// listSourcesHandler handles GET /repos/{repo_ref}/sources.
func (m *Module) listSourcesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.SourceListOptions{ListOptions: api.ParsePagination(r)}
		if ct := r.URL.Query().Get("connector"); ct != "" {
			opts.Connector = core.ConnectorType(ct)
		}
		if st := r.URL.Query().Get("status"); st != "" {
			opts.Status = core.ResourceStatus(st)
		}

		sources, total, err := m.archive.ListSources(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list sources")
			return
		}

		api.WriteList(w, sources, total, opts.ListOptions)
	}
}

// getSourceHandler handles GET /repos/{repo_ref}/sources/{source_ref}.
func (m *Module) getSourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		source, err := m.resolveSource(w, repo.ID, r.PathValue("source_ref"))
		if err != nil {
			return
		}

		api.SetETag(w, source.Version)
		api.WriteJSON(w, http.StatusOK, source)
	}
}

// updateSourceHandler handles PUT /repos/{repo_ref}/sources/{source_ref}.
func (m *Module) updateSourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		ifMatch := api.ParseIfMatch(r)
		if ifMatch == 0 {
			api.WriteError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match header is required for updates")
			return
		}
		if ifMatch == -1 {
			api.WriteError(w, http.StatusPreconditionFailed, "invalid_if_match", "If-Match header is malformed")
			return
		}

		source, err := m.resolveSource(w, repo.ID, r.PathValue("source_ref"))
		if err != nil {
			return
		}

		if source.Version != ifMatch {
			api.WriteError(w, http.StatusPreconditionFailed, "version_mismatch", "If-Match version does not match current version")
			return
		}

		r = api.LimitedBody(w, r)
		var req updateSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name != nil {
			source.Name = *req.Name
		}
		if req.Config != nil {
			// Validate config if connector type requires it.
			if err := m.validateSourceConfig(source.Connector, req.Config); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid_config", err.Error())
				return
			}
			// Cycle detection for repo connector config changes.
			if source.Connector == core.ConnectorRepo {
				if repoIDStr, ok := req.Config["repo_id"].(string); ok {
					targetRepoID, parseErr := uuid.Parse(repoIDStr)
					if parseErr == nil {
						hasCycle, cycleErr := m.detectCycle(repo.ID, targetRepoID)
						if cycleErr != nil {
							api.WriteError(w, http.StatusInternalServerError, "cycle_check_failed", "Failed to check for cycles")
							return
						}
						if hasCycle {
							api.WriteError(w, http.StatusConflict, "cycle_detected", "Updating this source would form a circular reference")
							return
						}
					}
				}
			}
			source.Config = req.Config
		}
		if req.FilterIDs != nil {
			source.FilterIDs = *req.FilterIDs
		}
		if req.FilterChainMode != nil {
			source.FilterChainMode = core.FilterMode(*req.FilterChainMode)
		}
		if req.RateLimit != nil {
			source.RateLimit = req.RateLimit
		}
		if req.Status != nil {
			source.Status = core.ResourceStatus(*req.Status)
		}

		if err := m.archive.SaveSource(source); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Source was modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save source")
			return
		}

		m.publishEvent(repo.ID, core.EventSourceUpdated, source)
		api.SetETag(w, source.Version)
		api.WriteJSON(w, http.StatusOK, source)
	}
}

// deleteSourceHandler handles DELETE /repos/{repo_ref}/sources/{source_ref}.
func (m *Module) deleteSourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		source, err := m.resolveSource(w, repo.ID, r.PathValue("source_ref"))
		if err != nil {
			return
		}

		if err := m.archive.DeleteSource(source.ID); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete source")
			return
		}

		m.publishEvent(repo.ID, core.EventSourceDeleted, source)
		w.WriteHeader(http.StatusNoContent)
	}
}

// triggerCollectHandler handles POST /repos/{repo_ref}/sources/{source_ref}/collect.
func (m *Module) triggerCollectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		source, err := m.resolveSource(w, repo.ID, r.PathValue("source_ref"))
		if err != nil {
			return
		}

		// Check rate limit.
		if source.RateLimit != nil && !m.rateLimiter.Allow(source.ID, source.RateLimit) {
			api.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Source is rate limited")
			return
		}

		job, err := jobs.CreateJob(m.archive, m.pub, repo.ID, defaultOwnerID, core.JobCollect)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "job_failed", "Failed to create collection job")
			return
		}

		api.WriteJSON(w, http.StatusAccepted, job)
	}
}

// sourceHistoryHandler handles GET /repos/{repo_ref}/sources/{source_ref}/history.
func (m *Module) sourceHistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		source, err := m.resolveSource(w, repo.ID, r.PathValue("source_ref"))
		if err != nil {
			return
		}

		opts := api.ParsePagination(r)
		records, total, err := m.archive.GetSourceHistory(source.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to get source history")
			return
		}

		api.WriteList(w, records, total, opts)
	}
}

// validateSourceConfig validates connector-specific configuration.
func (m *Module) validateSourceConfig(connectorType core.ConnectorType, config map[string]any) error {
	switch connectorType {
	case core.ConnectorRepo:
		repoIDStr, ok := config["repo_id"].(string)
		if !ok || repoIDStr == "" {
			return errors.New("config.repo_id is required for repo connector")
		}
		if _, err := uuid.Parse(repoIDStr); err != nil {
			return errors.New("config.repo_id must be a valid UUID")
		}
	case core.ConnectorWebhook:
		// Webhook sources require no special config validation.
	}
	return nil
}

// isSupportedConnector returns true if the connector type is supported in Phase 4.
func isSupportedConnector(ct core.ConnectorType) bool {
	return ct == core.ConnectorWebhook || ct == core.ConnectorRepo
}
