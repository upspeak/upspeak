package repo

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/xid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// createNodeHandler handles POST /repo/{repo_id}/nodes
func (m *ModuleRepo) createNodeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var node core.Node
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Generate Node ID if not provided
		if node.ID.IsNil() {
			node.ID = xid.New()
		}

		// Create input event
		event, err := core.NewEvent(core.EventCreateNode, core.EventNodeCreatePayload{Node: &node})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create event: %v", err), http.StatusInternalServerError)
			return
		}

		// Publish to NATS
		eventData, err := json.Marshal(event)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal event: %v", err), http.StatusInternalServerError)
			return
		}

		inSubject := fmt.Sprintf("repo.%s.in", repoID)
		if err := pub.Publish(inSubject, eventData); err != nil {
			http.Error(w, fmt.Sprintf("Failed to publish event: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(node)
	}
}

// getNodeHandler handles GET /repos/{repo_id}/nodes/{id}
func (m *ModuleRepo) getNodeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		repo, err := m.validateRepoID(repoID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		nodeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid node ID", http.StatusBadRequest)
			return
		}

		node, err := repo.GetNode(nodeID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Node not found: %v", err), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(node)
	}
}

// updateNodeHandler handles PUT /repos/{repo_id}/nodes/{id}
func (m *ModuleRepo) updateNodeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		nodeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid node ID", http.StatusBadRequest)
			return
		}

		var node core.Node
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Create input event
		event, err := core.NewEvent(core.EventUpdateNode, core.EventNodeUpdatePayload{
			NodeId:      nodeID,
			UpdatedNode: &node,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create event: %v", err), http.StatusInternalServerError)
			return
		}

		// Publish to NATS
		eventData, err := json.Marshal(event)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal event: %v", err), http.StatusInternalServerError)
			return
		}

		inSubject := fmt.Sprintf("repo.%s.in", repoID)
		if err := pub.Publish(inSubject, eventData); err != nil {
			http.Error(w, fmt.Sprintf("Failed to publish event: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(node)
	}
}

// deleteNodeHandler handles DELETE /repos/{repo_id}/nodes/{id}
func (m *ModuleRepo) deleteNodeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		nodeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid node ID", http.StatusBadRequest)
			return
		}

		// Create input event
		event, err := core.NewEvent(core.EventDeleteNode, core.EventNodeDeletePayload{
			NodeId: nodeID,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create event: %v", err), http.StatusInternalServerError)
			return
		}

		// Publish to NATS
		eventData, err := json.Marshal(event)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to marshal event: %v", err), http.StatusInternalServerError)
			return
		}

		inSubject := fmt.Sprintf("repo.%s.in", repoID)
		if err := pub.Publish(inSubject, eventData); err != nil {
			http.Error(w, fmt.Sprintf("Failed to publish event: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
