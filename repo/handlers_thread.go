package repo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// createThreadRequest is the expected JSON body for POST /repos/{repo_ref}/threads.
type createThreadRequest struct {
	Node     createNodeRequest `json:"node"`
	Metadata []core.Metadata   `json:"metadata"`
}

// createThreadHandler handles POST /api/v1/repos/{repo_ref}/threads.
func (m *Module) createThreadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createThreadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if req.Node.Type == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "node.type is required")
			return
		}
		if req.Node.Subject == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "node.subject is required")
			return
		}
		if req.Node.ContentType == "" {
			req.Node.ContentType = "text/plain"
		}

		thread := &core.Thread{
			ID:     core.NewID(),
			RepoID: repo.ID,
			Node: core.Node{
				ID:          core.NewID(),
				RepoID:      repo.ID,
				Type:        req.Node.Type,
				Subject:     req.Node.Subject,
				ContentType: req.Node.ContentType,
				Body:        req.Node.Body,
				Metadata:    req.Node.Metadata,
				CreatedBy:   defaultOwnerID,
			},
			Metadata:  req.Metadata,
			CreatedBy: defaultOwnerID,
		}

		if err := m.archive.SaveThread(thread); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create thread")
			return
		}

		api.SetETag(w, thread.Version)
		api.WriteJSON(w, http.StatusCreated, thread)
	}
}

// listThreadsHandler handles GET /api/v1/repos/{repo_ref}/threads.
func (m *Module) listThreadsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := api.ParsePagination(r)
		threads, total, err := m.archive.ListThreads(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list threads")
			return
		}

		api.WriteList(w, threads, total, opts)
	}
}

// updateThreadFromRequest handles PUT on a thread (updates metadata only).
func (m *Module) updateThreadFromRequest(w http.ResponseWriter, r *http.Request, repo *core.Repository, entityID string) {
	thread, err := m.archive.GetThread(safeParseUUID(entityID))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}

	if err := m.checkIfMatch(r, &core.Repository{Version: thread.Version}, w); err != nil {
		return
	}

	r = api.LimitedBody(w, r)
	var req struct {
		Metadata []core.Metadata `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	thread.Metadata = req.Metadata

	if err := m.archive.SaveThread(thread); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update thread")
		return
	}

	api.SetETag(w, thread.Version)
	api.WriteJSON(w, http.StatusOK, thread)
}

// addNodeToThreadHandler handles POST /api/v1/repos/{repo_ref}/{thread_ref}/nodes.
func (m *Module) addNodeToThreadHandler(w http.ResponseWriter, r *http.Request, threadID string) {
	tid := safeParseUUID(threadID)

	r = api.LimitedBody(w, r)
	var req struct {
		NodeID   string `json:"node_id"`
		EdgeType string `json:"edge_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.NodeID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing_field", "node_id is required")
		return
	}

	// Resolve the thread's repo to resolve the node ref.
	thread, err := m.archive.GetThread(tid)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}

	nodeID, _, err := m.archive.ResolveRef(thread.RepoID, req.NodeID)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve node reference")
		return
	}

	edgeType := req.EdgeType
	if edgeType == "" {
		edgeType = "contains"
	}

	if err := m.archive.AddNodeToThread(tid, nodeID, edgeType); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to add node to thread")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// removeNodeFromThreadHandler handles DELETE /api/v1/repos/{repo_ref}/{thread_ref}/{node_ref}.
func (m *Module) removeNodeFromThreadHandler(w http.ResponseWriter, r *http.Request, threadID, nodeRef string) {
	tid := safeParseUUID(threadID)

	thread, err := m.archive.GetThread(tid)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}

	nodeID, _, err := m.archive.ResolveRef(thread.RepoID, nodeRef)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, "invalid_reference", "Could not resolve node reference")
		return
	}

	if err := m.archive.RemoveNodeFromThread(tid, nodeID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "remove_failed", "Failed to remove node from thread")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
