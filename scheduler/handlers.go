package scheduler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/jobs"
)

// defaultOwnerID is a placeholder until authentication is implemented.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// createScheduleRequest is the JSON body for POST /schedules.
type createScheduleRequest struct {
	Name   string              `json:"name"`
	Cron   string              `json:"cron"`
	Action core.ScheduleAction `json:"action"`
}

// updateScheduleRequest is the JSON body for PUT /schedules/{schedule_ref}.
type updateScheduleRequest struct {
	Name    *string              `json:"name,omitempty"`
	Cron    *string              `json:"cron,omitempty"`
	Action  *core.ScheduleAction `json:"action,omitempty"`
	Enabled *bool                `json:"enabled,omitempty"`
}

// createScheduleHandler handles POST /api/v1/schedules.
func (m *Module) createScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = api.LimitedBody(w, r)

		var req createScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, "name_required", "Name is required")
			return
		}

		if req.Cron == "" {
			api.WriteError(w, http.StatusBadRequest, "cron_required", "Cron expression is required")
			return
		}

		// Validate cron expression.
		cs, err := ParseCron(req.Cron)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_cron", "Invalid cron expression: "+err.Error())
			return
		}

		if err := validateAction(req.Action); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_action", err.Error())
			return
		}

		now := time.Now().UTC()
		nextRun := cs.Next(now)

		schedule := &core.Schedule{
			ID:        core.NewID(),
			Name:      req.Name,
			Cron:      req.Cron,
			Action:    req.Action,
			Enabled:   true,
			NextRun:   &nextRun,
			CreatedBy: defaultOwnerID,
		}

		if err := m.archive.SaveSchedule(schedule); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create schedule")
			return
		}

		m.publishEvent(core.EventScheduleCreated, core.EventSchedulePayload{Schedule: schedule})

		api.SetETag(w, schedule.Version)
		api.WriteJSON(w, http.StatusCreated, schedule)
	}
}

// listSchedulesHandler handles GET /api/v1/schedules.
func (m *Module) listSchedulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := core.ScheduleListOptions{
			ListOptions: api.ParsePagination(r),
		}

		if v := r.URL.Query().Get("enabled"); v == "true" {
			enabled := true
			opts.Enabled = &enabled
		} else if v == "false" {
			enabled := false
			opts.Enabled = &enabled
		}

		if v := r.URL.Query().Get("action_type"); v != "" {
			opts.ActionType = v
		}

		schedules, total, err := m.archive.ListSchedules(opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list schedules")
			return
		}

		api.WriteList(w, schedules, total, opts.ListOptions)
	}
}

// getScheduleHandler handles GET /api/v1/schedules/{schedule_ref}.
func (m *Module) getScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		api.SetETag(w, schedule.Version)
		api.WriteJSON(w, http.StatusOK, schedule)
	}
}

// updateScheduleHandler handles PUT /api/v1/schedules/{schedule_ref}.
func (m *Module) updateScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		// Require If-Match for optimistic concurrency.
		ifMatch := api.ParseIfMatch(r)
		if ifMatch == 0 {
			api.WriteError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match header is required")
			return
		}
		if ifMatch == -1 {
			api.WriteError(w, http.StatusBadRequest, "invalid_if_match", "Invalid If-Match header")
			return
		}

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		if schedule.Version != ifMatch {
			api.WriteError(w, http.StatusPreconditionFailed, "version_mismatch", "Schedule has been modified")
			return
		}

		r = api.LimitedBody(w, r)
		var req updateScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name != nil {
			if *req.Name == "" {
				api.WriteError(w, http.StatusBadRequest, "name_required", "Name cannot be empty")
				return
			}
			schedule.Name = *req.Name
		}

		cronChanged := false
		if req.Cron != nil {
			if *req.Cron == "" {
				api.WriteError(w, http.StatusBadRequest, "cron_required", "Cron expression cannot be empty")
				return
			}
			if _, err := ParseCron(*req.Cron); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid_cron", "Invalid cron expression: "+err.Error())
				return
			}
			schedule.Cron = *req.Cron
			cronChanged = true
		}

		if req.Action != nil {
			if err := validateAction(*req.Action); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid_action", err.Error())
				return
			}
			schedule.Action = *req.Action
		}

		if req.Enabled != nil {
			schedule.Enabled = *req.Enabled
		}

		// Recompute NextRun if cron changed or schedule was re-enabled.
		if cronChanged || (req.Enabled != nil && *req.Enabled) {
			m.computeNextRun(schedule)
		}

		// Clear NextRun if disabled.
		if !schedule.Enabled {
			schedule.NextRun = nil
		}

		if err := m.archive.SaveSchedule(schedule); err != nil {
			if isVersionConflict(err) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_mismatch", "Schedule has been modified")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update schedule")
			return
		}

		m.publishEvent(core.EventScheduleUpdated, core.EventSchedulePayload{Schedule: schedule})

		api.SetETag(w, schedule.Version)
		api.WriteJSON(w, http.StatusOK, schedule)
	}
}

// deleteScheduleHandler handles DELETE /api/v1/schedules/{schedule_ref}.
func (m *Module) deleteScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		if err := m.archive.DeleteSchedule(schedule.ID); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete schedule")
			return
		}

		m.publishEvent(core.EventScheduleDeleted, core.EventSchedulePayload{Schedule: schedule})

		w.WriteHeader(http.StatusNoContent)
	}
}

// triggerScheduleHandler handles POST /api/v1/schedules/{schedule_ref}/trigger.
// It manually fires the schedule's action by creating a job.
func (m *Module) triggerScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		jobType, ok := core.ScheduleActionJobType(schedule.Action.Type)
		if !ok {
			api.WriteError(w, http.StatusBadRequest, "invalid_action_type", "Cannot determine job type from schedule action")
			return
		}

		repoID := uuid.Nil
		if schedule.Action.RepoID != nil {
			repoID = *schedule.Action.RepoID
		}

		job, err := jobs.CreateJob(m.archive, m.pub, repoID, defaultOwnerID, jobType, buildScheduleParams(schedule))
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "job_create_failed", "Failed to create job for schedule trigger")
			return
		}

		// Update LastRun.
		now := time.Now().UTC()
		schedule.LastRun = &now
		m.computeNextRun(schedule)
		_ = m.archive.SaveSchedule(schedule)

		api.WriteJSON(w, http.StatusAccepted, job)
	}
}

// pauseScheduleHandler handles POST /api/v1/schedules/{schedule_ref}/pause.
func (m *Module) pauseScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		if !schedule.Enabled {
			api.WriteJSON(w, http.StatusOK, schedule)
			return
		}

		schedule.Enabled = false
		schedule.NextRun = nil

		if err := m.archive.SaveSchedule(schedule); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to pause schedule")
			return
		}

		m.publishEvent(core.EventScheduleUpdated, core.EventSchedulePayload{Schedule: schedule})

		api.SetETag(w, schedule.Version)
		api.WriteJSON(w, http.StatusOK, schedule)
	}
}

// resumeScheduleHandler handles POST /api/v1/schedules/{schedule_ref}/resume.
func (m *Module) resumeScheduleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("schedule_ref")

		schedule, err := m.resolveSchedule(ref)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Schedule not found")
			return
		}

		if schedule.Enabled {
			api.WriteJSON(w, http.StatusOK, schedule)
			return
		}

		schedule.Enabled = true
		m.computeNextRun(schedule)

		if err := m.archive.SaveSchedule(schedule); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to resume schedule")
			return
		}

		m.publishEvent(core.EventScheduleUpdated, core.EventSchedulePayload{Schedule: schedule})

		api.SetETag(w, schedule.Version)
		api.WriteJSON(w, http.StatusOK, schedule)
	}
}

// resolveSchedule resolves a schedule ref (UUID or short ID) to a Schedule.
func (m *Module) resolveSchedule(ref string) (*core.Schedule, error) {
	// Try as UUID.
	if id, err := uuid.Parse(ref); err == nil {
		return m.archive.GetSchedule(id)
	}

	// Try as short ID (e.g. SCHED-1).
	if core.IsShortID(ref) {
		return m.archive.GetScheduleByShortID(ref)
	}

	return nil, errors.New("invalid schedule reference")
}

// computeNextRun calculates and sets the next run time for a schedule.
func (m *Module) computeNextRun(schedule *core.Schedule) {
	cs, err := ParseCron(schedule.Cron)
	if err != nil {
		schedule.NextRun = nil
		return
	}
	next := cs.Next(time.Now().UTC())
	if next.IsZero() {
		schedule.NextRun = nil
		return
	}
	schedule.NextRun = &next
}

// publishEvent publishes a schedule event. Schedule events derive their repoID
// from the action's RepoID when present; otherwise uuid.Nil is used since
// schedules are global (not repo-scoped). Errors are logged rather than
// swallowed so failures are visible in the application log.
func (m *Module) publishEvent(eventType core.EventType, payload any) {
	if m.pub == nil {
		return
	}
	repoID := uuid.Nil
	if p, ok := payload.(core.EventSchedulePayload); ok && p.Schedule != nil && p.Schedule.Action.RepoID != nil {
		repoID = *p.Schedule.Action.RepoID
	}
	if err := m.pub.PublishEvent(eventType, repoID, payload); err != nil {
		slog.Error("Failed to publish event", "type", eventType, "error", err)
	}
}

// validateAction checks that a ScheduleAction has valid and complete fields.
func validateAction(action core.ScheduleAction) error {
	if _, ok := core.ScheduleActionJobType(action.Type); !ok {
		return errors.New("action type must be \"collect\", \"publish\", or \"webhook\"")
	}

	switch action.Type {
	case "collect":
		if action.SourceID == nil {
			return errors.New("action.source_id is required for collect actions")
		}
		if action.RepoID == nil {
			return errors.New("action.repo_id is required for collect actions")
		}
	case "publish":
		if action.SinkID == nil {
			return errors.New("action.sink_id is required for publish actions")
		}
		if action.RepoID == nil {
			return errors.New("action.repo_id is required for publish actions")
		}
	case "webhook":
		if action.Params == nil {
			return errors.New("action.params is required for webhook actions")
		}
		if _, ok := action.Params["url"]; !ok {
			return errors.New("action.params.url is required for webhook actions")
		}
	}

	return nil
}

// buildScheduleParams constructs the JSON params for a job created by a schedule.
// It includes source_id, sink_id, and any extra params from the action.
func buildScheduleParams(schedule *core.Schedule) json.RawMessage {
	p := make(map[string]any)
	if schedule.Action.SourceID != nil {
		p["source_id"] = schedule.Action.SourceID.String()
	}
	if schedule.Action.SinkID != nil {
		p["sink_id"] = schedule.Action.SinkID.String()
	}
	for k, v := range schedule.Action.Params {
		p[k] = v
	}
	if len(p) == 0 {
		return nil
	}
	data, _ := json.Marshal(p)
	return data
}

// isVersionConflict checks if an error is a VersionConflictError.
func isVersionConflict(err error) bool {
	var vce *core.VersionConflictError
	return errors.As(err, &vce)
}
