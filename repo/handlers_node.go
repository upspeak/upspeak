package repo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// createNodeRequest is the expected JSON body for POST /repos/{repo_ref}/nodes.
type createNodeRequest struct {
	Type        string          `json:"type"`
	Subject     string          `json:"subject"`
	ContentType string          `json:"content_type"`
	Body        json.RawMessage `json:"body"`
	Metadata    []core.Metadata `json:"metadata"`
}

// createNodeHandler handles POST /api/v1/repos/{repo_ref}/nodes.
func (m *Module) createNodeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		var req createNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if req.Type == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "type is required")
			return
		}
		if req.Subject == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "subject is required")
			return
		}
		if req.ContentType == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "content_type is required")
			return
		}

		node := &core.Node{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Type:        req.Type,
			Subject:     req.Subject,
			ContentType: req.ContentType,
			Body:        req.Body,
			Metadata:    req.Metadata,
			CreatedBy:   defaultOwnerID,
		}

		if err := m.archive.SaveNode(node); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create node")
			return
		}

		api.SetETag(w, node.Version)
		api.WriteJSON(w, http.StatusCreated, node)
	}
}

// batchCreateNodesRequest is the expected JSON body for POST /repos/{repo_ref}/nodes/batch.
type batchCreateNodesRequest struct {
	Nodes []createNodeRequest `json:"nodes"`
}

// batchCreateNodesHandler handles POST /api/v1/repos/{repo_ref}/nodes/batch.
func (m *Module) batchCreateNodesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		var req batchCreateNodesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
		if len(req.Nodes) == 0 {
			api.WriteError(w, http.StatusBadRequest, "empty_batch", "At least one node is required")
			return
		}
		if len(req.Nodes) > 100 {
			api.WriteError(w, http.StatusBadRequest, "batch_too_large", "Maximum 100 items per batch")
			return
		}

		nodes := make([]*core.Node, len(req.Nodes))
		for i, n := range req.Nodes {
			if n.Type == "" || n.Subject == "" || n.ContentType == "" {
				api.WriteError(w, http.StatusUnprocessableEntity, "batch_validation_failed", "type, subject, and content_type are required for all items")
				return
			}
			nodes[i] = &core.Node{
				ID:          core.NewID(),
				RepoID:      repo.ID,
				Type:        n.Type,
				Subject:     n.Subject,
				ContentType: n.ContentType,
				Body:        n.Body,
				Metadata:    n.Metadata,
				CreatedBy:   defaultOwnerID,
			}
		}

		if err := m.archive.SaveBatchNodes(nodes); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create nodes")
			return
		}

		result := map[string]any{
			"created": len(nodes),
			"nodes":   nodes,
		}
		api.WriteJSON(w, http.StatusCreated, result)
	}
}

// listNodesHandler handles GET /api/v1/repos/{repo_ref}/nodes.
func (m *Module) listNodesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.NodeListOptions{
			Type:        r.URL.Query().Get("type"),
			ListOptions: api.ParsePagination(r),
		}

		nodes, total, err := m.archive.ListNodes(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list nodes")
			return
		}

		api.WriteList(w, nodes, total, opts.ListOptions)
	}
}

// getNodeHandler handles GET on a resolved node entity.
func (m *Module) getNodeHandler(w http.ResponseWriter, nodeID core.Node) {
	api.SetETag(w, nodeID.Version)
	api.WriteJSON(w, http.StatusOK, nodeID)
}

// updateNodeHandler handles PUT /api/v1/repos/{repo_ref}/{node_ref}.
func (m *Module) updateNodeFromRequest(w http.ResponseWriter, r *http.Request, repo *core.Repository, entityID string) {
	node, err := m.archive.GetNode(mustParseUUID(entityID))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Node not found")
		return
	}

	if err := m.checkIfMatch(r, &core.Repository{Version: node.Version}, w); err != nil {
		return
	}

	var req createNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	node.Type = req.Type
	node.Subject = req.Subject
	node.ContentType = req.ContentType
	node.Body = req.Body
	node.Metadata = req.Metadata

	if err := m.archive.SaveNode(node); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update node")
		return
	}

	api.SetETag(w, node.Version)
	api.WriteJSON(w, http.StatusOK, node)
}

// patchNodeFromRequest handles PATCH on a node.
func (m *Module) patchNodeFromRequest(w http.ResponseWriter, r *http.Request, repo *core.Repository, entityID string) {
	node, err := m.archive.GetNode(mustParseUUID(entityID))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not_found", "Node not found")
		return
	}

	if err := m.checkIfMatch(r, &core.Repository{Version: node.Version}, w); err != nil {
		return
	}

	var patch struct {
		Type        *string          `json:"type,omitempty"`
		Subject     *string          `json:"subject,omitempty"`
		ContentType *string          `json:"content_type,omitempty"`
		Body        *json.RawMessage `json:"body,omitempty"`
		Metadata    []core.Metadata  `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if patch.Type != nil {
		node.Type = *patch.Type
	}
	if patch.Subject != nil {
		node.Subject = *patch.Subject
	}
	if patch.ContentType != nil {
		node.ContentType = *patch.ContentType
	}
	if patch.Body != nil {
		node.Body = *patch.Body
	}

	// Metadata merge: new keys added, existing keys updated, null value deletes.
	if len(patch.Metadata) > 0 {
		node.Metadata = mergeMetadata(node.Metadata, patch.Metadata)
	}

	if err := m.archive.SaveNode(node); err != nil {
		var conflict *core.VersionConflictError
		if errors.As(err, &conflict) {
			api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update node")
		return
	}

	api.SetETag(w, node.Version)
	api.WriteJSON(w, http.StatusOK, node)
}

// nodeEdgesHandler handles GET /api/v1/repos/{repo_ref}/{node_ref}/edges.
func (m *Module) nodeEdgesHandler(w http.ResponseWriter, r *http.Request, nodeID string) {
	id := mustParseUUID(nodeID)
	opts := core.EdgeQueryOptions{
		Direction:   r.URL.Query().Get("direction"),
		Type:        r.URL.Query().Get("type"),
		ListOptions: api.ParsePagination(r),
	}
	if opts.Direction == "" {
		opts.Direction = "both"
	}

	edges, total, err := m.archive.GetNodeEdges(id, opts)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list edges")
		return
	}

	api.WriteList(w, edges, total, opts.ListOptions)
}

// nodeAnnotationsHandler handles GET /api/v1/repos/{repo_ref}/{node_ref}/annotations.
func (m *Module) nodeAnnotationsHandler(w http.ResponseWriter, r *http.Request, nodeID string) {
	id := mustParseUUID(nodeID)
	opts := core.AnnotationQueryOptions{
		Motivation:  r.URL.Query().Get("motivation"),
		ListOptions: api.ParsePagination(r),
	}

	annotations, total, err := m.archive.GetNodeAnnotations(id, opts)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list annotations")
		return
	}

	api.WriteList(w, annotations, total, opts.ListOptions)
}

// mergeMetadata applies patch metadata to existing metadata.
// New keys are added, existing keys updated, null values delete the key.
func mergeMetadata(existing, patch []core.Metadata) []core.Metadata {
	// Build map from existing.
	m := make(map[string]json.RawMessage)
	order := make([]string, 0)
	for _, md := range existing {
		if _, ok := m[md.Key]; !ok {
			order = append(order, md.Key)
		}
		m[md.Key] = md.Value
	}

	// Apply patch.
	for _, md := range patch {
		if md.Value == nil || string(md.Value) == "null" {
			delete(m, md.Key)
			continue
		}
		if _, ok := m[md.Key]; !ok {
			order = append(order, md.Key)
		}
		m[md.Key] = md.Value
	}

	// Rebuild slice preserving order.
	result := make([]core.Metadata, 0, len(m))
	for _, key := range order {
		if v, ok := m[key]; ok {
			result = append(result, core.Metadata{Key: key, Value: v})
		}
	}
	return result
}
