package repo

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/filter"
)

// reservedSegments are collection and action path segments that take priority
// over entity ref resolution in flat URLs.
var reservedSegments = map[string]bool{
	// Collection names.
	"nodes": true, "edges": true, "threads": true, "annotations": true,
	"filters": true, "sources": true, "sinks": true, "rules": true,
	// Action names.
	"search": true, "browse": true, "graph": true, "collect": true,
	"batch": true, "publish": true, "history": true, "test": true,
	"trigger": true, "pause": true, "resume": true,
}

// entityHandler handles GET/PUT/PATCH/DELETE /api/v1/repos/{repo_ref}/{entity_ref}.
// It resolves the entity ref via the archive and dispatches to the appropriate
// entity-type handler.
func (m *Module) entityHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoRef := r.PathValue("repo_ref")
		entityRef := r.PathValue("entity_ref")

		repo, err := m.resolveRepo(w, repoRef)
		if err != nil {
			return
		}

		// Resolve entity ref within the repo.
		entityID, entityType, err := m.archive.ResolveRef(repo.ID, entityRef)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Entity not found")
			return
		}

		idStr := entityID.String()

		switch r.Method {
		case http.MethodGet:
			m.getEntity(w, r, entityType, idStr, repo)
		case http.MethodPut:
			m.putEntity(w, r, entityType, idStr, repo)
		case http.MethodPatch:
			m.patchEntity(w, r, entityType, idStr, repo)
		case http.MethodDelete:
			m.deleteEntity(w, r, entityType, idStr, repo)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		}
	}
}

// entitySubHandler handles GET /api/v1/repos/{repo_ref}/{entity_ref}/{sub}.
// Supports sub-resources like /NODE-42/edges, /NODE-42/annotations, /THREAD-7/nodes.
func (m *Module) entitySubHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoRef := r.PathValue("repo_ref")
		entityRef := r.PathValue("entity_ref")
		sub := r.PathValue("sub")

		repo, err := m.resolveRepo(w, repoRef)
		if err != nil {
			return
		}

		entityID, entityType, err := m.archive.ResolveRef(repo.ID, entityRef)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Entity not found")
			return
		}

		idStr := entityID.String()

		switch {
		case entityType == "node" && sub == "edges":
			m.nodeEdgesHandler(w, r, idStr)
		case entityType == "node" && sub == "annotations":
			m.nodeAnnotationsHandler(w, r, idStr)
		case entityType == "thread" && sub == "nodes" && r.Method == http.MethodPost:
			m.addNodeToThreadHandler(w, r, idStr)
		case entityType == "thread" && sub == "publish" && r.Method == http.MethodPost:
			// Stub for Phase 4.
			api.WriteError(w, http.StatusNotImplemented, "not_implemented", "Thread publish is not yet implemented")
		case entityType == "filter" && sub == "test" && r.Method == http.MethodPost:
			m.testFilterViaFlatURL(w, r, entityID)
		default:
			api.WriteError(w, http.StatusNotFound, "not_found", "Sub-resource not found")
		}
	}
}

// threadNodeDeleteHandler handles DELETE /api/v1/repos/{repo_ref}/{thread_ref}/{node_ref}
// for removing a node from a thread.
func (m *Module) threadNodeDeleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoRef := r.PathValue("repo_ref")
		threadRef := r.PathValue("entity_ref")
		nodeRef := r.PathValue("sub")

		repo, err := m.resolveRepo(w, repoRef)
		if err != nil {
			return
		}

		threadID, entityType, err := m.archive.ResolveRef(repo.ID, threadRef)
		if err != nil || entityType != "thread" {
			api.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
			return
		}

		// Check if sub is a reserved segment — if so, this is not a thread node delete.
		if reservedSegments[nodeRef] {
			api.WriteError(w, http.StatusNotFound, "not_found", "Entity not found")
			return
		}

		m.removeNodeFromThreadHandler(w, r, threadID.String(), nodeRef)
	}
}

func (m *Module) getEntity(w http.ResponseWriter, r *http.Request, entityType, idStr string, repo *core.Repository) {
	id := safeParseUUID(idStr)

	switch entityType {
	case "node":
		node, err := m.archive.GetNode(id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Node not found")
			return
		}
		api.SetETag(w, node.Version)
		api.WriteJSON(w, http.StatusOK, node)

	case "edge":
		edge, err := m.archive.GetEdge(id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Edge not found")
			return
		}
		api.SetETag(w, edge.Version)
		api.WriteJSON(w, http.StatusOK, edge)

	case "thread":
		thread, err := m.archive.GetThread(id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
			return
		}
		api.SetETag(w, thread.Version)
		api.WriteJSON(w, http.StatusOK, thread)

	case "annotation":
		annotation, err := m.archive.GetAnnotation(id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Annotation not found")
			return
		}
		api.SetETag(w, annotation.Version)
		api.WriteJSON(w, http.StatusOK, annotation)

	case "filter":
		filter, err := m.archive.GetFilter(id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
			return
		}
		api.SetETag(w, filter.Version)
		api.WriteJSON(w, http.StatusOK, filter)

	default:
		api.WriteError(w, http.StatusNotFound, "not_found", "Unknown entity type")
	}
}

func (m *Module) putEntity(w http.ResponseWriter, r *http.Request, entityType, idStr string, repo *core.Repository) {
	switch entityType {
	case "node":
		m.updateNodeFromRequest(w, r, repo, idStr)
	case "edge":
		m.updateEdgeFromRequest(w, r, repo, idStr)
	case "thread":
		m.updateThreadFromRequest(w, r, repo, idStr)
	case "annotation":
		m.updateAnnotationFromRequest(w, r, repo, idStr)
	case "filter":
		m.updateFilterFromRequest(w, r, repo, idStr)
	default:
		api.WriteError(w, http.StatusNotFound, "not_found", "Unknown entity type")
	}
}

func (m *Module) patchEntity(w http.ResponseWriter, r *http.Request, entityType, idStr string, repo *core.Repository) {
	switch entityType {
	case "node":
		m.patchNodeFromRequest(w, r, repo, idStr)
	default:
		// Only nodes support PATCH currently.
		api.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "PATCH is only supported for nodes")
	}
}

// updateFilterFromRequest handles PUT on a filter via flat URL entity dispatch.
func (m *Module) updateFilterFromRequest(w http.ResponseWriter, r *http.Request, _ *core.Repository, idStr string) {
	id := safeParseUUID(idStr)

	existing, err := m.archive.GetFilter(id)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
		return
	}

	// Check If-Match for optimistic concurrency.
	expected := api.ParseIfMatch(r)
	if expected == -1 {
		api.WriteError(w, http.StatusBadRequest, "invalid_if_match", "Invalid If-Match header value")
		return
	}
	if expected > 0 && expected != existing.Version {
		api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified by another request")
		return
	}

	r = api.LimitedBody(w, r)
	var req struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Mode        core.FilterMode  `json:"mode"`
		Conditions  []core.Condition `json:"conditions"`
	}
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

	existing.Name = req.Name
	existing.Description = req.Description
	existing.Mode = req.Mode
	existing.Conditions = req.Conditions

	if err := m.archive.SaveFilter(existing); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified by another request")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update filter")
		return
	}

	api.SetETag(w, existing.Version)
	api.WriteJSON(w, http.StatusOK, existing)
}

// testFilterViaFlatURL handles POST /repos/{repo_ref}/FILTER-3/test via entity sub dispatch.
func (m *Module) testFilterViaFlatURL(w http.ResponseWriter, r *http.Request, filterID uuid.UUID) {
	f, err := m.archive.GetFilter(filterID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Filter not found")
		return
	}

	r = api.LimitedBody(w, r)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	result := filter.Evaluate(f, payload)
	api.WriteJSON(w, http.StatusOK, result)
}

func (m *Module) deleteEntity(w http.ResponseWriter, r *http.Request, entityType, idStr string, repo *core.Repository) {
	id := safeParseUUID(idStr)
	var err error

	switch entityType {
	case "node":
		err = m.archive.DeleteNode(id)
	case "edge":
		err = m.archive.DeleteEdge(id)
	case "thread":
		err = m.archive.DeleteThread(id)
	case "annotation":
		err = m.archive.DeleteAnnotation(id)
	case "filter":
		// Check referential integrity before deleting.
		refs, refErr := m.archive.GetFilterReferences(id)
		if refErr != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to check filter references")
			return
		}
		if len(refs) > 0 {
			api.WriteErrorWithDetails(w, http.StatusConflict, "referenced",
				"Filter is referenced by other entities and cannot be deleted", refs)
			return
		}
		err = m.archive.DeleteFilter(id)
	default:
		api.WriteError(w, http.StatusNotFound, "not_found", "Unknown entity type")
		return
	}

	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete entity")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// safeParseUUID parses a UUID string that has already been validated (e.g. from
// ResolveRef). Returns uuid.Nil if parsing fails, which should not happen under
// normal operation since callers only pass validated UUIDs.
func safeParseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// isReservedSegment checks if a path segment is a reserved collection or action name.
func isReservedSegment(s string) bool {
	return reservedSegments[strings.ToLower(s)]
}
