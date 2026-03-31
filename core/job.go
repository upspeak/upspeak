package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Job tracks an asynchronous operation (collect, publish, sync, webhook).
// Jobs are created by modules that trigger async work and are processed by
// the job runner consuming from the JOBS JetStream stream.
type Job struct {
	ID          uuid.UUID       `json:"id"`
	ShortID     string          `json:"short_id"`
	RepoID      uuid.UUID       `json:"repo_id"`
	Type        JobType         `json:"type"`
	Status      JobStatus       `json:"status"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       *string         `json:"error,omitempty"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
