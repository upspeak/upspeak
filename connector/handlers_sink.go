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

// createSinkRequest is the JSON body for creating a sink.
type createSinkRequest struct {
	Name            string          `json:"name"`
	Connector       string          `json:"connector"`
	Config          map[string]any  `json:"config"`
	FilterIDs       []uuid.UUID     `json:"filter_ids,omitempty"`
	FilterChainMode string          `json:"filter_chain_mode,omitempty"`
	RateLimit       *core.RateLimit `json:"rate_limit,omitempty"`
}

// updateSinkRequest is the JSON body for updating a sink.
type updateSinkRequest struct {
	Name            *string         `json:"name,omitempty"`
	Config          map[string]any  `json:"config,omitempty"`
	FilterIDs       *[]uuid.UUID    `json:"filter_ids,omitempty"`
	FilterChainMode *string         `json:"filter_chain_mode,omitempty"`
	RateLimit       *core.RateLimit `json:"rate_limit,omitempty"`
	Status          *string         `json:"status,omitempty"`
}

// createSinkHandler handles POST /repos/{repo_ref}/sinks.
func (m *Module) createSinkHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createSinkRequest
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

		if err := m.validateSinkConfig(connectorType, req.Config); err != nil {
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
				api.WriteError(w, http.StatusConflict, "cycle_detected", "Creating this sink would form a circular reference")
				return
			}
		}

		filterChainMode := core.FilterModeAll
		if req.FilterChainMode == string(core.FilterModeAny) {
			filterChainMode = core.FilterModeAny
		}

		sink := &core.Sink{
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

		if err := m.archive.SaveSink(sink); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save sink")
			return
		}

		m.publishEvent(repo.ID, core.EventSinkCreated, sink)
		api.SetETag(w, sink.Version)
		api.WriteJSON(w, http.StatusCreated, sink)
	}
}

// listSinksHandler handles GET /repos/{repo_ref}/sinks.
func (m *Module) listSinksHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.SinkListOptions{ListOptions: api.ParsePagination(r)}
		if ct := r.URL.Query().Get("connector"); ct != "" {
			opts.Connector = core.ConnectorType(ct)
		}
		if st := r.URL.Query().Get("status"); st != "" {
			opts.Status = core.ResourceStatus(st)
		}

		sinks, total, err := m.archive.ListSinks(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list sinks")
			return
		}

		api.WriteList(w, sinks, total, opts.ListOptions)
	}
}

// getSinkHandler handles GET /repos/{repo_ref}/sinks/{sink_ref}.
func (m *Module) getSinkHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		sink, err := m.resolveSink(w, repo.ID, r.PathValue("sink_ref"))
		if err != nil {
			return
		}

		api.SetETag(w, sink.Version)
		api.WriteJSON(w, http.StatusOK, sink)
	}
}

// updateSinkHandler handles PUT /repos/{repo_ref}/sinks/{sink_ref}.
func (m *Module) updateSinkHandler() http.HandlerFunc {
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

		sink, err := m.resolveSink(w, repo.ID, r.PathValue("sink_ref"))
		if err != nil {
			return
		}

		if sink.Version != ifMatch {
			api.WriteError(w, http.StatusPreconditionFailed, "version_mismatch", "If-Match version does not match current version")
			return
		}

		r = api.LimitedBody(w, r)
		var req updateSinkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name != nil {
			sink.Name = *req.Name
		}
		if req.Config != nil {
			if err := m.validateSinkConfig(sink.Connector, req.Config); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid_config", err.Error())
				return
			}
			// Cycle detection for repo connector config changes.
			if sink.Connector == core.ConnectorRepo {
				if repoIDStr, ok := req.Config["repo_id"].(string); ok {
					targetRepoID, parseErr := uuid.Parse(repoIDStr)
					if parseErr == nil {
						hasCycle, cycleErr := m.detectCycle(repo.ID, targetRepoID)
						if cycleErr != nil {
							api.WriteError(w, http.StatusInternalServerError, "cycle_check_failed", "Failed to check for cycles")
							return
						}
						if hasCycle {
							api.WriteError(w, http.StatusConflict, "cycle_detected", "Updating this sink would form a circular reference")
							return
						}
					}
				}
			}
			sink.Config = req.Config
		}
		if req.FilterIDs != nil {
			sink.FilterIDs = *req.FilterIDs
		}
		if req.FilterChainMode != nil {
			sink.FilterChainMode = core.FilterMode(*req.FilterChainMode)
		}
		if req.RateLimit != nil {
			sink.RateLimit = req.RateLimit
		}
		if req.Status != nil {
			sink.Status = core.ResourceStatus(*req.Status)
		}

		if err := m.archive.SaveSink(sink); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Sink was modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save sink")
			return
		}

		m.publishEvent(repo.ID, core.EventSinkUpdated, sink)
		api.SetETag(w, sink.Version)
		api.WriteJSON(w, http.StatusOK, sink)
	}
}

// deleteSinkHandler handles DELETE /repos/{repo_ref}/sinks/{sink_ref}.
func (m *Module) deleteSinkHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		sink, err := m.resolveSink(w, repo.ID, r.PathValue("sink_ref"))
		if err != nil {
			return
		}

		if err := m.archive.DeleteSink(sink.ID); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete sink")
			return
		}

		m.publishEvent(repo.ID, core.EventSinkDeleted, sink)
		w.WriteHeader(http.StatusNoContent)
	}
}

// triggerPublishHandler handles POST /repos/{repo_ref}/sinks/{sink_ref}/publish.
func (m *Module) triggerPublishHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		sink, err := m.resolveSink(w, repo.ID, r.PathValue("sink_ref"))
		if err != nil {
			return
		}

		// Check rate limit.
		if sink.RateLimit != nil && !m.rateLimiter.Allow(sink.ID, sink.RateLimit) {
			api.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Sink is rate limited")
			return
		}

		job, err := jobs.CreateJob(m.archive, m.pub, repo.ID, defaultOwnerID, core.JobPublish)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "job_failed", "Failed to create publish job")
			return
		}

		api.WriteJSON(w, http.StatusAccepted, job)
	}
}

// sinkHistoryHandler handles GET /repos/{repo_ref}/sinks/{sink_ref}/history.
func (m *Module) sinkHistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		sink, err := m.resolveSink(w, repo.ID, r.PathValue("sink_ref"))
		if err != nil {
			return
		}

		opts := api.ParsePagination(r)
		records, total, err := m.archive.GetSinkHistory(sink.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "history_failed", "Failed to get sink history")
			return
		}

		api.WriteList(w, records, total, opts)
	}
}

// validateSinkConfig validates connector-specific configuration for sinks.
func (m *Module) validateSinkConfig(connectorType core.ConnectorType, config map[string]any) error {
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
		// Webhook sinks require no special config validation.
	}
	return nil
}
