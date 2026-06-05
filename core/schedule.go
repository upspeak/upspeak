package core

import (
	"time"

	"github.com/google/uuid"
)

// Schedule defines a cron-based trigger for automated actions. Schedules are
// global (not repo-scoped) but their actions reference specific repositories.
type Schedule struct {
	ID        uuid.UUID      `json:"id"`
	ShortID   string         `json:"short_id"`
	Name      string         `json:"name"`
	Cron      string         `json:"cron"`
	Action    ScheduleAction `json:"action"`
	Enabled   bool           `json:"enabled"`
	NextRun   *time.Time     `json:"next_run,omitempty"`
	LastRun   *time.Time     `json:"last_run,omitempty"`
	Version   int            `json:"version"`
	CreatedBy uuid.UUID      `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ScheduleAction specifies what a schedule does when it fires. The Type field
// determines which other fields are required.
type ScheduleAction struct {
	Type     string         `json:"type"`                // "collect", "publish", or "webhook"
	SourceID *uuid.UUID     `json:"source_id,omitempty"` // required for "collect"
	SinkID   *uuid.UUID     `json:"sink_id,omitempty"`   // required for "publish"
	RepoID   *uuid.UUID     `json:"repo_id,omitempty"`   // required for "collect" and "publish"
	Params   map[string]any `json:"params,omitempty"`    // extra parameters (e.g. url, method for webhook)
}

// ScheduleActionJobType maps a schedule action type to the job type it triggers,
// reporting false when the action type cannot be scheduled. The schedulable
// action vocabulary (collect, publish, webhook) and its mapping to JobType live
// here so schedulers do not re-enumerate them.
func ScheduleActionJobType(actionType string) (JobType, bool) {
	switch actionType {
	case string(JobCollect):
		return JobCollect, true
	case string(JobPublish):
		return JobPublish, true
	case string(JobWebhook):
		return JobWebhook, true
	default:
		return "", false
	}
}

// CollectionRecord captures the result of a single collection attempt on a source.
type CollectionRecord struct {
	ID           uuid.UUID      `json:"id"`
	SourceID     uuid.UUID      `json:"source_id"`
	At           time.Time      `json:"at"`
	Result       string         `json:"result"` // "success", "partial", "error"
	Details      map[string]any `json:"details,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	DurationMs   int64          `json:"duration_ms"`
}

// PublishRecord captures the result of a single publish attempt on a sink.
type PublishRecord struct {
	ID           uuid.UUID      `json:"id"`
	SinkID       uuid.UUID      `json:"sink_id"`
	At           time.Time      `json:"at"`
	Result       string         `json:"result"` // "success", "partial", "error"
	Details      map[string]any `json:"details,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	DurationMs   int64          `json:"duration_ms"`
	ExternalURL  string         `json:"external_url,omitempty"`
}
