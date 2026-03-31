package repo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// createAnnotationRequest is the expected JSON body for POST /repos/{repo_ref}/annotations.
type createAnnotationRequest struct {
	TargetNodeID string            `json:"target_node_id"`
	Motivation   string            `json:"motivation"`
	Node         createNodeRequest `json:"node"`
}

// createAnnotationHandler handles POST /api/v1/repos/{repo_ref}/annotations.
func (m *Module) createAnnotationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createAnnotationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if req.TargetNodeID == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "target_node_id is required")
			return
		}
		if req.Motivation == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "motivation is required")
			return
		}
		if req.Node.Type == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "node.type is required")
			return
		}

		// Resolve target node.
		targetID, _, err := m.archive.ResolveRef(repo.ID, req.TargetNodeID)
		if err != nil {
			api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve target node reference")
			return
		}

		if req.Node.ContentType == "" {
			req.Node.ContentType = "text/plain"
		}

		annoNodeID := core.NewID()
		annotation := &core.Annotation{
			ID:     core.NewID(),
			RepoID: repo.ID,
			Node: core.Node{
				ID:          annoNodeID,
				RepoID:      repo.ID,
				Type:        req.Node.Type,
				Subject:     req.Node.Subject,
				ContentType: req.Node.ContentType,
				Body:        req.Node.Body,
				Metadata:    req.Node.Metadata,
				CreatedBy:   defaultOwnerID,
			},
			Edge: core.Edge{
				ID:        core.NewID(),
				RepoID:    repo.ID,
				Type:      "annotates",
				Source:    annoNodeID,
				Target:    targetID,
				Label:     req.Motivation,
				Weight:    1.0,
				CreatedBy: defaultOwnerID,
			},
			Motivation: req.Motivation,
			CreatedBy:  defaultOwnerID,
		}

		if err := m.archive.SaveAnnotation(annotation); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create annotation")
			return
		}

		api.SetETag(w, annotation.Version)
		api.WriteJSON(w, http.StatusCreated, annotation)
	}
}

// listAnnotationsHandler handles GET /api/v1/repos/{repo_ref}/annotations.
func (m *Module) listAnnotationsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := api.ParsePagination(r)
		annotations, total, err := m.archive.ListAnnotations(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list annotations")
			return
		}

		api.WriteList(w, annotations, total, opts)
	}
}

// updateAnnotationFromRequest handles PUT on an annotation.
func (m *Module) updateAnnotationFromRequest(w http.ResponseWriter, r *http.Request, repo *core.Repository, entityID string) {
	annotation, err := m.archive.GetAnnotation(safeParseUUID(entityID))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Annotation not found")
		return
	}

	if err := m.checkIfMatch(r, &core.Repository{Version: annotation.Version}, w); err != nil {
		return
	}

	r = api.LimitedBody(w, r)
	var req struct {
		Motivation string            `json:"motivation"`
		Node       createNodeRequest `json:"node"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.Motivation != "" {
		annotation.Motivation = req.Motivation
	}
	if req.Node.Subject != "" {
		annotation.Node.Subject = req.Node.Subject
	}
	if req.Node.Body != nil {
		annotation.Node.Body = req.Node.Body
	}

	if err := m.archive.SaveAnnotation(annotation); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update annotation")
		return
	}

	api.SetETag(w, annotation.Version)
	api.WriteJSON(w, http.StatusOK, annotation)
}
