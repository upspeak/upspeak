package core

import (
	"context"
	"time"
)

// AdapterCapabilities declares which optional roles an adapter supports and
// whether it requires a configured Connection.
type AdapterCapabilities struct {
	Collect            bool
	Publish            bool
	Stream             bool
	OAuth              bool
	RequiresConnection bool
}

// FieldSpec describes one configuration field an adapter accepts. Secret fields
// are routed to encrypted Credentials and redacted from API responses.
type FieldSpec struct {
	Name     string
	Secret   bool
	Required bool
}

// ConnectionSchema is an adapter's declarative description of its Connection
// configuration fields.
type ConnectionSchema struct {
	Fields []FieldSpec
}

// Adapter is the base contract every integration implements. Optional roles
// (Collector, Publisher, Streamer, OAuthProvider) are added via the interfaces
// below and discovered by type assertion in the registry.
type Adapter interface {
	Type() ConnectorType
	Capabilities() AdapterCapabilities
	ConnectionSchema() ConnectionSchema
	ValidateConnectionConfig(cfg map[string]any) error
	ValidateSourceConfig(cfg map[string]any) error
	ValidateSinkConfig(cfg map[string]any) error
	TestConnection(ctx context.Context, conn *Connection) error
}

// Collector pulls content from a Source. Implemented by pull adapters.
type Collector interface {
	Collect(ctx context.Context, req CollectRequest) (*IngestBatch, error)
}

// Publisher pushes nodes to a Sink. Implemented by write-capable adapters.
//
// Note: this is core.Publisher (an adapter role), distinct from app.Publisher
// (the NATS event bus).
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)
}

// Streamer maintains a long-lived ingestion stream, calling emit per batch
// until ctx is cancelled. Implemented by streaming adapters (Matrix).
type Streamer interface {
	Stream(ctx context.Context, req StreamRequest, emit func(*IngestBatch) error) error
}

// OAuthProvider implements the OAuth2 authorisation-code flow.
type OAuthProvider interface {
	AuthCodeURL(conn *Connection, redirectURI, state string) (string, error)
	Exchange(ctx context.Context, conn *Connection, code, redirectURI string) (OAuthTokens, error)
	Refresh(ctx context.Context, conn *Connection, refreshToken string) (OAuthTokens, error)
}

// CollectRequest carries the inputs for a pull collection.
type CollectRequest struct {
	Connection *Connection
	Source     *Source
	Cursor     *IngestCursor
}

// StreamRequest carries the inputs for a streaming ingestion. It mirrors
// CollectRequest today but is kept separate so streaming-specific fields (e.g.
// a backfill limit) can be added later without changing the collect contract.
type StreamRequest struct {
	Connection *Connection
	Source     *Source
	Cursor     *IngestCursor
}

// PublishRequest carries the nodes to publish to a Sink.
type PublishRequest struct {
	Connection *Connection
	Sink       *Sink
	Items      []PublishItem
}

// PublishItem is a node to publish plus its resolved reply-in-context target.
type PublishItem struct {
	Node                *Node
	InReplyToExternalID string
	ThreadExternalID    string
}

// PublishResult reports the external IDs created by a publish.
type PublishResult struct {
	ExternalIDs []string
}

// OAuthTokens are the tokens returned by an OAuth exchange or refresh. They are
// internal — the adapter maps them into Connection.Credentials for encrypted
// storage — so they carry no JSON tags.
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
}
