package core

import (
	"time"

	"github.com/google/uuid"
)

// Source defines where content is collected from. Each source belongs to a
// repository and uses a connector type to fetch data from an external system
// or another Upspeak repository.
type Source struct {
	ID              uuid.UUID      `json:"id"`
	ShortID         string         `json:"short_id"`
	RepoID          uuid.UUID      `json:"repo_id"`
	Name            string         `json:"name"`
	Connector       ConnectorType  `json:"connector"`
	Config          map[string]any `json:"config"`
	FilterIDs       []uuid.UUID    `json:"filter_ids,omitempty"`
	FilterChainMode FilterMode     `json:"filter_chain_mode"`
	RateLimit       *RateLimit     `json:"rate_limit,omitempty"`
	Status          ResourceStatus `json:"status"`
	Version         int            `json:"version"`
	CreatedBy       uuid.UUID      `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
