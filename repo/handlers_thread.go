package repo

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/xid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// createThreadHandler handles POST /repo/{repo_id}/threads
func (m *ModuleRepo) createThreadHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var thread core.Thread
		if err := json.NewDecoder(r.Body).Decode(&thread); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Generate Thread Node ID if not provided
		if thread.Node.ID.IsNil() {
			thread.Node.ID = xid.New()
		}

		// Create input event
		event, err := core.NewEvent(core.EventCreateThread, core.EventThreadCreatePayload{Thread: &thread})
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
		json.NewEncoder(w).Encode(thread)
	}
}

// getThreadHandler handles GET /repo/{repo_id}/threads/{id}
func (m *ModuleRepo) getThreadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		repo, err := m.validateRepoID(repoID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		threadID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid thread ID", http.StatusBadRequest)
			return
		}

		thread, err := repo.GetThread(threadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Thread not found: %v", err), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(thread)
	}
}

// updateThreadHandler handles PUT /repo/{repo_id}/threads/{id}
func (m *ModuleRepo) updateThreadHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		threadID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid thread ID", http.StatusBadRequest)
			return
		}

		var thread core.Thread
		if err := json.NewDecoder(r.Body).Decode(&thread); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Ensure the thread node ID matches
		thread.Node.ID = threadID

		// Create input event
		event, err := core.NewEvent(core.EventUpdateThread, core.EventThreadUpdatePayload{
			ThreadId:      threadID,
			UpdatedThread: &thread,
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
		json.NewEncoder(w).Encode(thread)
	}
}

// deleteThreadHandler handles DELETE /repo/{repo_id}/threads/{id}
func (m *ModuleRepo) deleteThreadHandler(pub app.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repoID := r.PathValue("repo_id")

		// Validate repository
		if _, err := m.validateRepoID(repoID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		idStr := r.PathValue("id")
		threadID, err := xid.FromString(idStr)
		if err != nil {
			http.Error(w, "Invalid thread ID", http.StatusBadRequest)
			return
		}

		// Create input event
		event, err := core.NewEvent(core.EventDeleteThread, core.EventThreadDeletePayload{
			ThreadId: threadID,
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
