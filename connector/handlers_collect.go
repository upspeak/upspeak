package connector

import (
	"encoding/json"
	"net/http"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/jobs"
)

// oneShotCollectRequest is the JSON body for one-shot URL collection.
type oneShotCollectRequest struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
}

// oneShotCollectHandler handles POST /repos/{repo_ref}/collect.
// It creates a webhook-type job for ad-hoc URL collection without a configured source.
func (m *Module) oneShotCollectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req oneShotCollectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.URL == "" {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", "URL is required")
			return
		}

		params, _ := json.Marshal(map[string]string{"url": req.URL, "content_type": req.ContentType})
		job, err := jobs.CreateJob(m.archive, m.pub, repo.ID, defaultOwnerID, core.JobWebhook, params)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "job_failed", "Failed to create collection job")
			return
		}

		api.WriteJSON(w, http.StatusAccepted, job)
	}
}
