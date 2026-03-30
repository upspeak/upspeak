package repo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"

)

// createEdgeRequest is the expected JSON body for POST /repos/{repo_ref}/edges.
type createEdgeRequest struct {
	Type   string  `json:"type"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

// createEdgeHandler handles POST /api/v1/repos/{repo_ref}/edges.
func (m *Module) createEdgeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		var req createEdgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if req.Type == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "type is required")
			return
		}
		if req.Source == "" || req.Target == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "source and target are required")
			return
		}

		// Resolve source and target refs.
		sourceID, _, err := m.archive.ResolveRef(repo.ID, req.Source)
		if err != nil {
			api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve source reference")
			return
		}
		targetID, _, err := m.archive.ResolveRef(repo.ID, req.Target)
		if err != nil {
			api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve target reference")
			return
		}

		weight := req.Weight
		if weight == 0 {
			weight = 1.0
		}

		edge := &core.Edge{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Type:      req.Type,
			Source:    sourceID,
			Target:    targetID,
			Label:     req.Label,
			Weight:    weight,
			CreatedBy: defaultOwnerID,
		}

		if err := m.archive.SaveEdge(edge); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create edge")
			return
		}

		api.SetETag(w, edge.Version)
		api.WriteJSON(w, http.StatusCreated, edge)
	}
}

// batchCreateEdgesRequest is the expected JSON body for POST /repos/{repo_ref}/edges/batch.
type batchCreateEdgesRequest struct {
	Edges []createEdgeRequest `json:"edges"`
}

// batchCreateEdgesHandler handles POST /api/v1/repos/{repo_ref}/edges/batch.
func (m *Module) batchCreateEdgesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		var req batchCreateEdgesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if len(req.Edges) == 0 {
			api.WriteError(w, http.StatusBadRequest, "empty_batch", "At least one edge is required")
			return
		}
		if len(req.Edges) > 100 {
			api.WriteError(w, http.StatusBadRequest, "batch_too_large", "Maximum 100 items per batch")
			return
		}

		edges := make([]*core.Edge, len(req.Edges))
		for i, e := range req.Edges {
			if e.Type == "" || e.Source == "" || e.Target == "" {
				api.WriteError(w, http.StatusUnprocessableEntity, "batch_validation_failed", "type, source, and target are required for all items")
				return
			}
			sourceID, _, err := m.archive.ResolveRef(repo.ID, e.Source)
			if err != nil {
				api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve source reference")
				return
			}
			targetID, _, err := m.archive.ResolveRef(repo.ID, e.Target)
			if err != nil {
				api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve target reference")
				return
			}
			weight := e.Weight
			if weight == 0 {
				weight = 1.0
			}
			edges[i] = &core.Edge{
				ID:        core.NewID(),
				RepoID:    repo.ID,
				Type:      e.Type,
				Source:    sourceID,
				Target:    targetID,
				Label:     e.Label,
				Weight:    weight,
				CreatedBy: defaultOwnerID,
			}
		}

		if err := m.archive.SaveBatchEdges(edges); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create edges")
			return
		}

		result := map[string]any{
			"created": len(edges),
			"edges":   edges,
		}
		api.WriteJSON(w, http.StatusCreated, result)
	}
}

// listEdgesHandler handles GET /api/v1/repos/{repo_ref}/edges.
func (m *Module) listEdgesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.EdgeListOptions{
			Source:      r.URL.Query().Get("source"),
			Target:      r.URL.Query().Get("target"),
			Type:        r.URL.Query().Get("type"),
			ListOptions: api.ParsePagination(r),
		}

		edges, total, err := m.archive.ListEdges(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list edges")
			return
		}

		api.WriteList(w, edges, total, opts.ListOptions)
	}
}

// updateEdgeFromRequest handles PUT on an edge.
func (m *Module) updateEdgeFromRequest(w http.ResponseWriter, r *http.Request, repo *core.Repository, entityID string) {
	edge, err := m.archive.GetEdge(mustParseUUID(entityID))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Edge not found")
		return
	}

	if err := m.checkIfMatch(r, &core.Repository{Version: edge.Version}, w); err != nil {
		return
	}

	var req createEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Type != "" {
		edge.Type = req.Type
	}
	edge.Label = req.Label
	if req.Weight != 0 {
		edge.Weight = req.Weight
	}

	if err := m.archive.SaveEdge(edge); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update edge")
		return
	}

	api.SetETag(w, edge.Version)
	api.WriteJSON(w, http.StatusOK, edge)
}
