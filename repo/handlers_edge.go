package repo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/xid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// createEdgeHandler handles POST /repos/{repo_id}/edges
func (m *ModuleRepo) createEdgeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var edge core.Edge
		if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Generate Edge ID if not provided
		if edge.ID.IsNil() {
			edge.ID = xid.New()
		}

		// Create input event
		event, err := core.NewEvent(core.EventCreateEdge, core.EventEdgeCreatePayload{Edge: &edge})
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
		json.NewEncoder(w).Encode(edge)
	}
}

// getEdgeHandler handles GET /repos/{repo_id}/edges/{id}
func (m *ModuleRepo) getEdgeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		repo, err := m.validateRepoID(repoID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		edgeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid edge ID", http.StatusBadRequest)
			return
		}

		edge, err := repo.GetEdge(edgeID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Edge not found: %v", err), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(edge)
	}
}

// updateEdgeHandler handles PUT /repos/{repo_id}/edges/{id}
func (m *ModuleRepo) updateEdgeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		edgeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid edge ID", http.StatusBadRequest)
			return
		}

		var edge core.Edge
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
			return
		}

		if err := json.Unmarshal(body, &edge); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Ensure the edge ID matches
		edge.ID = edgeID

		// Create input event
		event, err := core.NewEvent(core.EventUpdateEdge, core.EventEdgeCreatePayload{Edge: &edge})
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
		json.NewEncoder(w).Encode(edge)
	}
}

// deleteEdgeHandler handles DELETE /repos/{repo_id}/edges/{id}
func (m *ModuleRepo) deleteEdgeHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		edgeID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid edge ID", http.StatusBadRequest)
			return
		}

		// Create input event
		event, err := core.NewEvent(core.EventDeleteEdge, core.EventEdgeDeletePayload{
			EdgeId: edgeID,
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
