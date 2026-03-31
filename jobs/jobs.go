// Package jobs provides the jobs module for tracking and managing asynchronous
// operations. Jobs are created by modules that trigger async work (collect,
// publish, sync, webhook) and are processed by the job runner consuming from
// the JOBS JetStream stream.
package jobs

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements the app.Module interface for job tracking.
// It exposes HTTP endpoints for listing, retrieving, and cancelling jobs.
type Module struct {
	archive  core.Archive
	consumer app.Consumer
	logger   *slog.Logger
}

// Name returns the module name.
func (m *Module) Name() string {
	return "jobs"
}

// Init initialises the jobs module.
func (m *Module) Init(_ map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	m.logger.Info("Initialised jobs module")
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) {
	m.archive = archive
}

// SetConsumer injects the JetStream consumer for the JOBS stream.
func (m *Module) SetConsumer(consumer app.Consumer) {
	m.consumer = consumer
}

// HTTPHandlers returns the HTTP handlers for the jobs module.
// All paths are relative to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: "GET", Path: "/jobs", Handler: m.listJobsHandler()},
		{Method: "GET", Path: "/jobs/{job_ref}", Handler: m.getJobHandler()},
		{Method: "POST", Path: "/jobs/{job_ref}/cancel", Handler: m.cancelJobHandler()},
	}
}

// MsgHandlers returns the message handlers for the jobs module.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{}
}

// listJobsHandler handles GET /api/v1/jobs.
// Query params: ?status=pending|running|..., ?type=collect|publish|..., ?repo_id={ref}
func (m *Module) listJobsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := core.JobListOptions{
			Status:      r.URL.Query().Get("status"),
			Type:        r.URL.Query().Get("type"),
			RepoID:      r.URL.Query().Get("repo_id"),
			ListOptions: api.ParsePagination(r),
		}

		jobs, total, err := m.archive.ListJobs(opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list jobs")
			return
		}

		api.WriteList(w, jobs, total, opts.ListOptions)
	}
}

// getJobHandler handles GET /api/v1/jobs/{job_ref}.
func (m *Module) getJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("job_ref")

		job, err := m.resolveJob(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Job not found")
			return
		}

		api.WriteJSON(w, http.StatusOK, job)
	}
}

// cancelJobHandler handles POST /api/v1/jobs/{job_ref}/cancel.
// Cancellation is best-effort: the job's status is set to "cancelled" and
// the runner checks status before each step. A job that has already completed
// or failed cannot be cancelled.
func (m *Module) cancelJobHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("job_ref")

		job, err := m.resolveJob(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Job not found")
			return
		}

		// Only pending or running jobs can be cancelled.
		if job.Status != core.JobStatusPending && job.Status != core.JobStatusRunning {
			api.WriteError(w, http.StatusConflict, "invalid_status",
				"Job cannot be cancelled (status: "+string(job.Status)+")")
			return
		}

		job.Status = core.JobStatusCancelled
		now := time.Now().UTC()
		job.CompletedAt = &now

		if err := m.archive.SaveJob(job); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to cancel job")
			return
		}

		api.WriteJSON(w, http.StatusOK, job)
	}
}

// resolveJob resolves a job ref (UUID or short ID) to a Job.
func (m *Module) resolveJob(ref string) (*core.Job, error) {
	// Try as UUID.
	if id, err := uuid.Parse(ref); err == nil {
		return m.archive.GetJob(id)
	}

	// Try as short ID: parse to extract the sequence, then look up by short_id.
	// Jobs use global sequences, so we need a different resolution approach
	// than repo-scoped entities.
	if core.IsShortID(ref) {
		return m.resolveJobByShortID(ref)
	}

	return nil, errors.New("invalid job reference")
}

// resolveJobByShortID looks up a job by its short ID string (e.g. "JOB-42").
func (m *Module) resolveJobByShortID(shortID string) (*core.Job, error) {
	return m.archive.GetJobByShortID(shortID)
}

// CreateJob is a helper that other modules can use to create a job in the archive.
// It generates a UUID and sets the initial status to pending.
func CreateJob(archive core.Archive, repoID, createdBy uuid.UUID, jobType core.JobType) (*core.Job, error) {
	job := &core.Job{
		ID:        core.NewID(),
		RepoID:    repoID,
		Type:      jobType,
		Status:    core.JobStatusPending,
		CreatedBy: createdBy,
	}

	if err := archive.SaveJob(job); err != nil {
		return nil, err
	}

	// Publish job to JOBS stream.
	payload, _ := json.Marshal(job)
	subject := "jobs." + string(jobType) + "." + job.ID.String()
	_ = payload // Will be used when publisher is wired for job creation.
	_ = subject

	return job, nil
}
