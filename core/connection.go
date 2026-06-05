package core

import (
	"time"

	"github.com/google/uuid"
)

// Connection is a configured, reusable link to one external system (a Discourse
// site, a Matrix homeserver account, a Mastodon account). Sources and Sinks
// reference a Connection for endpoint and credentials, so the secret is
// configured once and shared across every Source/Sink that uses that system.
//
// Connections are owner-scoped (not repo-scoped): one configured Discourse
// account is reused by Sources/Sinks in any of the owner's repositories.
//
// OwnerID scopes the Connection to a user across repositories; CreatedBy records
// the creating actor for audit (the two may differ for admin-created connections).
type Connection struct {
	ID        uuid.UUID     `json:"id"`
	ShortID   string        `json:"short_id"` // CONN-{seq}, per-user sequence
	OwnerID   uuid.UUID     `json:"owner_id"`
	Name      string        `json:"name"`
	Connector ConnectorType `json:"connector"`
	AuthType  AuthType      `json:"auth_type"`
	// Config holds non-secret configuration (base_url, scopes, account handle).
	// It is returned in API responses.
	Config map[string]any `json:"config"`
	// Credentials holds secret material (api_key, access_token, refresh_token).
	// It is encrypted at rest and never serialised to API responses.
	Credentials      map[string]any `json:"-"`
	Status           ResourceStatus `json:"status"`
	RateLimit        *RateLimit     `json:"rate_limit,omitempty"`
	CredentialExpiry *time.Time     `json:"credential_expiry,omitempty"`
	LastCheckedAt    *time.Time     `json:"last_checked_at,omitempty"`
	LastError        *string        `json:"last_error,omitempty"`
	Version          int            `json:"version"`
	CreatedBy        uuid.UUID      `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}
