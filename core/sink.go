package core

import (
	"time"

	"github.com/google/uuid"
)

// Sink defines where content is published to. Each sink belongs to a
// repository and uses a connector type to push data to an external system
// or another Upspeak repository.
type Sink struct {
	ID      uuid.UUID `json:"id"`
	ShortID string    `json:"short_id"`
	RepoID  uuid.UUID `json:"repo_id"`
	// ConnectionID links this Sink to a configured Connection. Nil for
	// connection-less adapters (RSS, webhook). When set, the Connector must
	// match the Connection's.
	ConnectionID    *uuid.UUID     `json:"connection_id,omitempty"`
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
