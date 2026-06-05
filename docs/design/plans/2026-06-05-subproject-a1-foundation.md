# Connector Adapterisation — Sub-project A1 (Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `core` domain model and the encrypted archive storage for the connector-adapterisation framework — the data + contract foundation everything else in the ingestion track depends on — with no HTTP, registry, or adapter implementations yet.

**Architecture:** All new domain types live in `core/` (entities, the Adapter contract, ingest data-flow types, and the `SecretCipher` port). A new `secrets/` package implements AES-GCM encryption keyed from the environment. The `archive/` SQLite layer gains a `connections` table (credentials encrypted at rest), node provenance columns (`source_id`, `external_id`), `connection_id` on sources/sinks, and an `ingest_cursors` table, exposed through new `core.Archive` sub-interfaces.

**Tech Stack:** Go, SQLite (`mattn/go-sqlite3`), `google/uuid` (UUID v7), Go stdlib `crypto/aes`+`crypto/cipher`, table-driven tests with `setupTestArchive(t)`.

---

## Plan family (context)

This is **A1 of the ingestion track** (`docs/design/2026-06-05-connector-adapterisation.md`, ADR-0013/0014/0015). The track decomposes as:

- **A — framework** (this spine), itself sliced into:
  - **A1 (this plan): foundation** — core model + secure storage + archive.
  - **A2: adapter registry + ingest pipeline** — registry, identity-resolving pipeline reusing the write/event path, jobs/connector DI, webhook/repo migration.
  - **A3: connection module** — `/api/v1/connections` CRUD + `TestConnection` + events.
  - **A4: OAuth flow + Mastodon adapter** — authorize/callback/lazy-refresh, the OAuth exerciser.
- **B — pull execution + RSS & Discourse.**
- **C — streaming + Matrix.**

Each slice ends with a green build and passing tests.

---

## File map for A1

| File | Responsibility | Action |
|---|---|---|
| `core/identity.go` | add `PrefixConnection` | Modify |
| `core/shared_types.go` | add `AuthType` + consts, `ConnectorMastodon` | Modify |
| `core/connection.go` | `Connection` entity | Create |
| `core/core.go` | `Node.SourceID`, `Node.ExternalID` | Modify |
| `core/source.go`, `core/sink.go` | `ConnectionID *uuid.UUID` | Modify |
| `core/ingest.go` | `IngestBatch`/`IngestThread`/`IngestItem`/`IngestAnnotation`/`ExternalActor`/`IngestCursor` | Create |
| `core/adapter.go` | `Adapter` + role interfaces, capabilities, schema, request/result types | Create |
| `core/secret.go` | `SecretCipher` port + `ErrSecretKeyMissing` | Create |
| `core/list.go` | `ConnectionListOptions` | Modify |
| `core/archive.go` | `ConnectionStore`, `IngestCursorStore`, `NodeStore.GetNodeBySourceExternalID`; compose | Modify |
| `secrets/cipher.go` | AES-GCM `SecretCipher` impl + env key loader | Create |
| `secrets/cipher_test.go` | cipher tests | Create |
| `archive/schema.go` | `connections`, `ingest_cursors` tables; node + source/sink columns | Modify |
| `archive/node_store.go` | thread provenance; `getNodeBySourceExternalID` | Modify |
| `archive/source_store.go`, `archive/sink_store.go` | thread `connection_id` | Modify |
| `archive/connection_store.go` | connection CRUD + encryption + references | Create |
| `archive/connection_store_test.go` | connection store tests | Create |
| `archive/ingest_cursor_store.go` | cursor upsert/get | Create |
| `archive/ingest_cursor_store_test.go` | cursor tests | Create |
| `archive/local.go` | cipher field + setter + facade methods | Modify |

---

## Task 1: Core domain types (declarations)

Pure type/interface declarations. No behaviour, so verification is `go build` + `go vet` and an interface-compliance assertion added later. Write all of the following, then build.

**Files:**
- Modify: `core/identity.go`
- Modify: `core/shared_types.go`
- Create: `core/connection.go`
- Modify: `core/core.go`
- Modify: `core/source.go`, `core/sink.go`
- Create: `core/ingest.go`
- Create: `core/adapter.go`
- Create: `core/secret.go`
- Modify: `core/list.go`

- [ ] **Step 1: Add the connection short-ID prefix**

In `core/identity.go`, add `PrefixConnection` to the prefix const block and the `EntityPrefixToType` map:

```go
	PrefixUser       = "USER"
	PrefixConnection = "CONN"
```

```go
	PrefixUser:       "user",
	PrefixConnection: "connection",
```

- [ ] **Step 2: Add AuthType and the Mastodon connector type**

In `core/shared_types.go`, after the `ConnectorType` const block add the Mastodon type:

```go
	ConnectorMastodon  ConnectorType = "mastodon"
```

And add a new `AuthType` declaration block:

```go
// AuthType identifies how a Connection authenticates to its external system.
type AuthType string

// Auth type constants.
const (
	AuthNone   AuthType = "none"
	AuthAPIKey AuthType = "api_key"
	AuthToken  AuthType = "token"
	AuthOAuth2 AuthType = "oauth2"
)
```

Add a new resource status for OAuth connections awaiting authorisation. In the `ResourceStatus` const block:

```go
	StatusPendingAuth ResourceStatus = "pending_auth"
```

- [ ] **Step 3: Create the Connection entity**

Create `core/connection.go`:

```go
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
```

- [ ] **Step 4: Add node provenance fields**

In `core/core.go`, add two nullable fields to `Node` (after `Metadata`):

```go
	// SourceID and ExternalID record ingestion provenance: which Source produced
	// this node and its stable identifier in the external system. Both nil for
	// manually-created nodes. Together they give idempotent re-collection,
	// container grouping (all nodes from a room/category), and search attribution.
	SourceID   *uuid.UUID `json:"source_id,omitempty"`
	ExternalID *string    `json:"external_id,omitempty"`
```

- [ ] **Step 5: Add ConnectionID to Source and Sink**

In `core/source.go` and `core/sink.go`, add (after `RepoID`):

```go
	// ConnectionID links this Source/Sink to a configured Connection. Nil for
	// connection-less adapters (RSS, webhook). When set, the Connector must
	// match the Connection's.
	ConnectionID *uuid.UUID `json:"connection_id,omitempty"`
```

- [ ] **Step 6: Create the ingest data-flow types**

Create `core/ingest.go`:

```go
package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// IngestBatch is what an adapter emits from a collect or stream call. Adapters
// speak external IDs only; they never touch internal UUIDs or the archive. The
// ingest pipeline owns identity resolution and persistence.
type IngestBatch struct {
	Threads     []IngestThread     `json:"threads,omitempty"`     // conversations → Threads
	Items       []IngestItem       `json:"items,omitempty"`       // messages/posts → Nodes (+ reply Edges)
	Annotations []IngestAnnotation `json:"annotations,omitempty"` // reactions/likes → Annotations
	Tombstones  []string           `json:"tombstones,omitempty"`  // external IDs to redact/delete
	Cursor      *IngestCursor      `json:"cursor,omitempty"`      // advanced cursor (nil = unchanged)
}

// IngestThread describes a conversation (Discourse topic, Matrix m.thread) that
// the pipeline resolves to a Thread by (SourceID, ExternalThreadID).
type IngestThread struct {
	ExternalThreadID string     `json:"external_thread_id"`
	Subject          string     `json:"subject"`
	Metadata         []Metadata `json:"metadata,omitempty"`
}

// IngestItem is one external message/post projected toward the graph. The
// adapter fills content + external identity; the pipeline assigns IDs, applies
// filters, resolves the thread/parent, and persists.
type IngestItem struct {
	ExternalID       string         `json:"external_id"`
	ThreadExternalID string         `json:"thread_external_id,omitempty"` // empty → attach at Source level
	ParentExternalID string         `json:"parent_external_id,omitempty"` // → reply Edge
	Node             *Node          `json:"node"`                         // content only (no ID/RepoID/ShortID)
	Author           *ExternalActor `json:"author,omitempty"`
}

// IngestAnnotation is a reaction/like/highlight targeting a node.
type IngestAnnotation struct {
	ExternalID       string          `json:"external_id"`
	TargetExternalID string          `json:"target_external_id"`
	Motivation       string          `json:"motivation"`
	Body             json.RawMessage `json:"body,omitempty"`
	Author           *ExternalActor  `json:"author,omitempty"`
}

// ExternalActor is an external author resolved to a User (username@hostname).
type ExternalActor struct {
	ExternalID  string `json:"external_id"`
	Username    string `json:"username"`
	Hostname    string `json:"hostname"`
	DisplayName string `json:"display_name"`
}

// IngestCursor persists an adapter-defined resumption point per Source. The
// Cursor payload is opaque to core (RSS: etag+last-guid; Discourse: high-water
// post id; Matrix: since-token). Distinct from Phase 6b multi-device sync.
type IngestCursor struct {
	SourceID  uuid.UUID       `json:"source_id"`
	Cursor    json.RawMessage `json:"cursor"`
	UpdatedAt time.Time       `json:"updated_at"`
}
```

- [ ] **Step 7: Create the Adapter contract**

Create `core/adapter.go`:

```go
package core

import "context"

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

// Request and result types for adapter calls.

type CollectRequest struct {
	Connection *Connection
	Source     *Source
	Cursor     *IngestCursor
}

type StreamRequest struct {
	Connection *Connection
	Source     *Source
	Cursor     *IngestCursor
}

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

type PublishResult struct {
	ExternalIDs []string
}

type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       []string
}
```

> Note: `OAuthTokens` uses `time.Time`; add `"time"` to the import block alongside `"context"`.

- [ ] **Step 8: Create the SecretCipher port**

Create `core/secret.go`:

```go
package core

import "errors"

// ErrSecretKeyMissing indicates that credential encryption was requested but no
// master key is configured. Operations that must read or write credentials fail
// closed with this error rather than handling secrets in plaintext.
var ErrSecretKeyMissing = errors.New("secret encryption key is not configured")

// SecretCipher encrypts and decrypts credential material at rest. The
// implementation lives outside core (see the secrets package) and is injected
// into the archive, keeping cryptography out of the domain layer.
type SecretCipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
```

- [ ] **Step 9: Add ConnectionListOptions**

In `core/list.go`, add (mirroring `SourceListOptions`):

```go
// ConnectionListOptions filters connections in list operations.
type ConnectionListOptions struct {
	Connector ConnectorType
	Status    ResourceStatus
	ListOptions
}
```

- [ ] **Step 10: Build and vet**

Run: `go build ./core/... && go vet ./core/...`
Expected: builds cleanly, no vet warnings. (The `core.Archive` interface is not yet extended, so the archive package still compiles.)

- [ ] **Step 11: Commit**

```bash
git add core/
git commit -m "feat(core): add Connection, Adapter contract, ingest types, SecretCipher port

Domain model for the connector-adapterisation framework (ADR-0013/0014/0015):
owner-scoped Connection with split config/credentials and AuthType; node
provenance fields; Source/Sink ConnectionID; the external-ID-keyed ingest
data-flow types; the capability-based Adapter contract with optional role
interfaces; and the SecretCipher port. Declarations only — storage follows.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Secrets package — AES-GCM cipher

**Files:**
- Create: `secrets/cipher.go`
- Test: `secrets/cipher_test.go`

- [ ] **Step 1: Write the failing tests**

Create `secrets/cipher_test.go`:

```go
package secrets

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestCipher_RoundTrip(t *testing.T) {
	c, err := NewCipher(testKey())
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	plain := []byte(`{"api_key":"secret-123"}`)

	ct, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ct, []byte("secret-123")) {
		t.Fatal("ciphertext leaks plaintext")
	}

	got, err := c.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestCipher_NonceIsRandom(t *testing.T) {
	c, _ := NewCipher(testKey())
	plain := []byte("same input")
	a, _ := c.Encrypt(plain)
	b, _ := c.Encrypt(plain)
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same plaintext are identical; nonce not random")
	}
}

func TestCipher_TamperDetected(t *testing.T) {
	c, _ := NewCipher(testKey())
	ct, _ := c.Encrypt([]byte("data"))
	ct[len(ct)-1] ^= 0xFF // flip a byte
	if _, err := c.Decrypt(ct); err == nil {
		t.Fatal("expected GCM authentication failure on tampered ciphertext")
	}
}

func TestNewCipher_BadKeyLength(t *testing.T) {
	if _, err := NewCipher([]byte("too-short")); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./secrets/...`
Expected: FAIL — `secrets` package / `NewCipher` undefined.

- [ ] **Step 3: Implement the cipher**

Create `secrets/cipher.go`:

```go
// Package secrets provides the AES-GCM implementation of core.SecretCipher used
// to encrypt connection credentials at rest. The master key is sourced from the
// environment; cryptography is kept out of the core domain package.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/upspeak/upspeak/core"
)

// EnvKeyName is the environment variable holding the base64-encoded 32-byte
// master key.
const EnvKeyName = "UPSPEAK_SECRET_KEY"

// Cipher is an AES-256-GCM implementation of core.SecretCipher.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher creates a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromEnv builds a Cipher from the base64-encoded key in UPSPEAK_SECRET_KEY.
// Returns (nil, nil) when the variable is unset, so callers can run without
// credential encryption until a connection actually needs it (fail-closed at use).
func NewCipherFromEnv() (*Cipher, error) {
	raw := os.Getenv(EnvKeyName)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded: %w", EnvKeyName, err)
	}
	return NewCipher(key)
}

// Encrypt seals plaintext with a fresh random nonce, returning nonce||ciphertext.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens nonce||ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, body := ciphertext[:ns], ciphertext[ns:]
	plain, err := c.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return plain, nil
}

// Ensure Cipher satisfies the core port.
var _ core.SecretCipher = (*Cipher)(nil)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./secrets/...`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add secrets/
git commit -m "feat(secrets): add AES-256-GCM SecretCipher with env key (ADR-0014)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Schema — tables and columns

**Files:**
- Modify: `archive/schema.go`

- [ ] **Step 1: Add node provenance columns and index**

In `archive/schema.go`, in the `nodes` table definition, add two columns before `created_by`:

```go
	metadata     TEXT,
	source_id    TEXT,
	external_id  TEXT,
	created_by   TEXT NOT NULL,
```

After the existing `idx_nodes_repo_id` index line add:

```go
CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_source_external ON nodes(source_id, external_id) WHERE source_id IS NOT NULL;
```

- [ ] **Step 2: Add connection_id to sources and sinks**

In both the `sources` and `sinks` table definitions, add after `repo_id`:

```go
	connection_id    TEXT,
```

- [ ] **Step 3: Add the connections table**

After the `sinks` table block (and its indexes), add:

```go
-- Connections: owner-scoped, configure-once links to external systems.
-- Credentials are stored encrypted (BLOB) and never returned in API responses.
CREATE TABLE IF NOT EXISTS connections (
	id                TEXT PRIMARY KEY,
	short_id          TEXT NOT NULL,
	owner_id          TEXT NOT NULL,
	name              TEXT NOT NULL,
	connector         TEXT NOT NULL,
	auth_type         TEXT NOT NULL DEFAULT 'none',
	config            TEXT NOT NULL DEFAULT '{}',
	credentials       BLOB,
	status            TEXT NOT NULL DEFAULT 'active',
	rate_limit        TEXT,
	credential_expiry TEXT,
	last_checked_at   TEXT,
	last_error        TEXT,
	version           INTEGER NOT NULL DEFAULT 1,
	created_by        TEXT NOT NULL,
	created_at        TEXT NOT NULL,
	updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_connections_owner ON connections(owner_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_connections_owner_short_id ON connections(owner_id, short_id);
```

- [ ] **Step 4: Add the ingest_cursors table**

```go
-- Ingest cursors: per-source resumption point (opaque, adapter-defined).
CREATE TABLE IF NOT EXISTS ingest_cursors (
	source_id  TEXT PRIMARY KEY,
	cursor     TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
```

- [ ] **Step 5: Build**

Run: `go build -tags sqlite_fts5 ./archive/...`
Expected: builds (schema is a const string; behaviour comes in later tasks).

- [ ] **Step 6: Commit**

```bash
git add archive/schema.go
git commit -m "feat(archive): schema for connections, ingest cursors, node provenance

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Node provenance — persistence and lookup

Thread `source_id`/`external_id` through node create, batch-create, and all node SELECTs, and add the dedup lookup. Provenance is set on create only (re-collection updates reuse the existing row found by lookup).

**Files:**
- Modify: `archive/node_store.go`
- Modify: `core/archive.go` (extend `NodeStore`)
- Modify: `archive/local.go` (facade)
- Test: `archive/node_store_test.go`

- [ ] **Step 1: Write the failing test**

Add to `archive/node_store_test.go`:

```go
func TestSaveNode_Provenance_AndLookup(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	srcID := core.NewID()
	ext := "discourse:post:42"
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "post",
		Subject:     "Hello",
		ContentType: "text/plain",
		CreatedBy:   repo.OwnerID,
		SourceID:    &srcID,
		ExternalID:  &ext,
	}
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	got, err := a.GetNodeBySourceExternalID(srcID, ext)
	if err != nil {
		t.Fatalf("GetNodeBySourceExternalID: %v", err)
	}
	if got.ID != node.ID {
		t.Fatalf("got node %s, want %s", got.ID, node.ID)
	}
	if got.SourceID == nil || *got.SourceID != srcID {
		t.Fatalf("source_id not persisted: %v", got.SourceID)
	}
	if got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("external_id not persisted: %v", got.ExternalID)
	}

	// Unknown external id → not found.
	_, err = a.GetNodeBySourceExternalID(srcID, "nope")
	if !errors.As(err, new(*core.ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound, got %v", err)
	}
}
```

> If `createTestRepo` / the `errors` import are not already present in this test file, mirror the helpers used by the sibling archive tests (see `archive/repo_store_test.go`) and add `"errors"` to the imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags sqlite_fts5 ./archive/ -run TestSaveNode_Provenance_AndLookup`
Expected: FAIL — `GetNodeBySourceExternalID` undefined.

- [ ] **Step 3: Persist provenance on create**

In `archive/node_store.go` `saveNode`, replace the create-branch INSERT with one that includes the two columns:

```go
		_, err = a.db.Exec(`
			INSERT INTO nodes (id, short_id, repo_id, type, subject, content_type, metadata, source_id, external_id, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, node.ID.String(), node.ShortID, node.RepoID.String(), node.Type, node.Subject,
			node.ContentType, string(metadataJSON), uuidPtrString(node.SourceID), strPtrAny(node.ExternalID),
			node.CreatedBy.String(),
			node.Version, node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
```

Do the same for the `saveBatchNodes` INSERT (the `tx.Exec` block), adding `source_id, external_id` columns and the same two argument expressions.

Add these helpers at the bottom of `archive/node_store.go`:

```go
// uuidPtrString returns the UUID string for a non-nil pointer, or nil for SQL.
func uuidPtrString(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

// strPtrAny returns the string for a non-nil pointer, or nil for SQL.
func strPtrAny(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
```

- [ ] **Step 4: Scan provenance in all node SELECTs**

Update the three node SELECT column lists — in `getNode`, `listNodes`, and (new) `getNodeBySourceExternalID` — to read:

```
SELECT id, short_id, repo_id, type, subject, content_type, metadata, source_id, external_id, created_by, version, created_at, updated_at
```

Update both scan helpers to read the new columns. In `scanNodeFromSingleRow` and `scanNodeFromRow`, add two `sql.NullString` locals and include them in `Scan` between `metadataStr` and `createdByStr`:

```go
	var metadataStr, sourceIDStr, externalIDStr sql.NullString

	err := row.Scan(&idStr, &node.ShortID, &repoIDStr, &node.Type, &node.Subject,
		&node.ContentType, &metadataStr, &sourceIDStr, &externalIDStr, &createdByStr,
		&node.Version, &createdAt, &updatedAt)
```

(Apply the equivalent change in `scanNodeFromRow` using `rows.Scan`.)

Extend `parseNodeFields` to accept and parse them:

```go
func parseNodeFields(node *core.Node, idStr, repoIDStr, createdByStr string, metadataStr, sourceIDStr, externalIDStr sql.NullString, createdAt, updatedAt string) (*core.Node, error) {
```

and before the `return node, nil`:

```go
	if sourceIDStr.Valid && sourceIDStr.String != "" {
		sid, err := uuid.Parse(sourceIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node source_id: %w", err)
		}
		node.SourceID = &sid
	}
	if externalIDStr.Valid {
		ext := externalIDStr.String
		node.ExternalID = &ext
	}
```

Update both call sites of `parseNodeFields` to pass `sourceIDStr, externalIDStr`.

- [ ] **Step 5: Implement getNodeBySourceExternalID**

Add to `archive/node_store.go`:

```go
// getNodeBySourceExternalID finds a node by its ingestion provenance, used for
// idempotent re-collection. Returns ErrorNotFound when no matching node exists.
func (a *LocalArchive) getNodeBySourceExternalID(sourceID uuid.UUID, externalID string) (*core.Node, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, type, subject, content_type, metadata, source_id, external_id, created_by, version, created_at, updated_at
		FROM nodes WHERE source_id = ? AND external_id = ?
	`, sourceID.String(), externalID)

	node, err := scanNodeFromSingleRow(row)
	if err != nil {
		return nil, err
	}

	body, err := a.readNodeBody(node.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to read node body: %w", err)
	}
	node.Body = body
	return node, nil
}
```

- [ ] **Step 6: Extend the interface and facade**

In `core/archive.go`, add to `NodeStore`:

```go
	// GetNodeBySourceExternalID finds a node by ingestion provenance for
	// idempotent re-collection. Returns ErrorNotFound when absent.
	GetNodeBySourceExternalID(sourceID uuid.UUID, externalID string) (*Node, error)
```

In `archive/local.go`, in the `core.NodeStore implementation` block, add:

```go
func (a *LocalArchive) GetNodeBySourceExternalID(sourceID uuid.UUID, externalID string) (*core.Node, error) {
	return a.getNodeBySourceExternalID(sourceID, externalID)
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./archive/ -run TestSaveNode`
Expected: PASS (the new test and existing node tests).

- [ ] **Step 8: Commit**

```bash
git add core/archive.go archive/node_store.go archive/local.go archive/node_store_test.go
git commit -m "feat(archive): persist node provenance and add GetNodeBySourceExternalID

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Source/Sink ConnectionID persistence

Thread the nullable `connection_id` through source and sink create/update/scan.

**Files:**
- Modify: `archive/source_store.go`, `archive/sink_store.go`
- Test: `archive/source_store_test.go` (create if absent), `archive/sink_store_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Create `archive/source_store_test.go` (or add to it):

```go
package archive

import (
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestSaveSource_ConnectionID(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	connID := core.NewID()
	src := &core.Source{
		ID:           core.NewID(),
		RepoID:       repo.ID,
		Name:         "discourse-cat",
		Connector:    core.ConnectorDiscourse,
		ConnectionID: &connID,
		Config:       map[string]any{"category_id": "5"},
		CreatedBy:    repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	got, err := a.GetSource(src.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.ConnectionID == nil || *got.ConnectionID != connID {
		t.Fatalf("connection_id not persisted: %v", got.ConnectionID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags sqlite_fts5 ./archive/ -run TestSaveSource_ConnectionID`
Expected: FAIL — scanned/inserted columns mismatch (`ConnectionID` stays nil or insert error).

- [ ] **Step 3: Thread connection_id through source_store.go**

In `saveSource` create-branch INSERT, add `connection_id` after `repo_id` in both the column list and values, passing `uuidPtrString(source.ConnectionID)`:

```go
			INSERT INTO sources (id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, created_by, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
```

```go
		`, source.ID.String(), source.ShortID, source.RepoID.String(), uuidPtrString(source.ConnectionID), source.Name,
			string(source.Connector), string(configJSON), string(filterIDsJSON),
			string(source.FilterChainMode), stringOrNil(rateLimitJSON), string(source.Status),
			source.CreatedBy.String(), source.Version,
			source.CreatedAt.Format(time.RFC3339), source.UpdatedAt.Format(time.RFC3339))
```

In the update-branch, add `connection_id = ?` to the `SET` list with `uuidPtrString(source.ConnectionID)` as its argument.

Update the SELECT column lists in `getSource` and `listSources` to include `connection_id` after `repo_id`. In both scan helpers add a `var connectionIDStr sql.NullString` and read it after `repoIDStr` in `Scan`; pass it to `parseSourceFields`. Extend `parseSourceFields` to accept it and set:

```go
	if connectionIDStr.Valid && connectionIDStr.String != "" {
		cid, err := uuid.Parse(connectionIDStr.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse source connection_id: %w", err)
		}
		source.ConnectionID = &cid
	}
```

- [ ] **Step 4: Apply the identical change to sink_store.go**

Mirror Step 3 in `archive/sink_store.go` (`saveSink`, `getSink`, `listSinks`, sink scan helpers, `parseSinkFields`), using `sink.ConnectionID`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./archive/ -run 'TestSaveSource_ConnectionID|TestSaveSink'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add archive/source_store.go archive/sink_store.go archive/source_store_test.go
git commit -m "feat(archive): persist Source/Sink connection_id

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Ingest cursor store

**Files:**
- Create: `archive/ingest_cursor_store.go`
- Modify: `core/archive.go` (add `IngestCursorStore`, compose into `Archive`)
- Modify: `archive/local.go` (facade)
- Test: `archive/ingest_cursor_store_test.go`

- [ ] **Step 1: Write the failing test**

Create `archive/ingest_cursor_store_test.go`:

```go
package archive

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestIngestCursor_UpsertAndGet(t *testing.T) {
	a := setupTestArchive(t)
	srcID := core.NewID()

	// Missing → ErrorNotFound.
	if _, err := a.GetIngestCursor(srcID); !errors.As(err, new(*core.ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound, got %v", err)
	}

	// Save.
	c := &core.IngestCursor{SourceID: srcID, Cursor: json.RawMessage(`{"since":"a"}`)}
	if err := a.SaveIngestCursor(c); err != nil {
		t.Fatalf("SaveIngestCursor: %v", err)
	}
	got, err := a.GetIngestCursor(srcID)
	if err != nil {
		t.Fatalf("GetIngestCursor: %v", err)
	}
	if string(got.Cursor) != `{"since":"a"}` {
		t.Fatalf("cursor mismatch: %s", got.Cursor)
	}

	// Upsert (overwrite).
	c.Cursor = json.RawMessage(`{"since":"b"}`)
	if err := a.SaveIngestCursor(c); err != nil {
		t.Fatalf("SaveIngestCursor (upsert): %v", err)
	}
	got, _ = a.GetIngestCursor(srcID)
	if string(got.Cursor) != `{"since":"b"}` {
		t.Fatalf("upsert failed: %s", got.Cursor)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags sqlite_fts5 ./archive/ -run TestIngestCursor`
Expected: FAIL — `SaveIngestCursor`/`GetIngestCursor` undefined.

- [ ] **Step 3: Implement the store**

Create `archive/ingest_cursor_store.go`:

```go
package archive

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveIngestCursor upserts the per-source ingestion cursor.
func (a *LocalArchive) saveIngestCursor(c *core.IngestCursor) error {
	if c == nil {
		return fmt.Errorf("ingest cursor is nil")
	}
	now := time.Now().UTC()
	c.UpdatedAt = now
	_, err := a.db.Exec(`
		INSERT INTO ingest_cursors (source_id, cursor, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (source_id) DO UPDATE SET cursor = excluded.cursor, updated_at = excluded.updated_at
	`, c.SourceID.String(), string(c.Cursor), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to save ingest cursor: %w", err)
	}
	return nil
}

// getIngestCursor returns the cursor for a source, or ErrorNotFound.
func (a *LocalArchive) getIngestCursor(sourceID uuid.UUID) (*core.IngestCursor, error) {
	var cursorStr, updatedAt string
	err := a.db.QueryRow(`
		SELECT cursor, updated_at FROM ingest_cursors WHERE source_id = ?
	`, sourceID.String()).Scan(&cursorStr, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("ingest_cursor", sourceID.String())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ingest cursor: %w", err)
	}
	c := &core.IngestCursor{SourceID: sourceID, Cursor: []byte(cursorStr)}
	c.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ingest cursor updated_at: %w", err)
	}
	return c, nil
}
```

- [ ] **Step 4: Interface and facade**

In `core/archive.go`, add the sub-interface and add it to the `Archive` composition:

```go
// IngestCursorStore handles per-source ingestion cursor persistence.
type IngestCursorStore interface {
	SaveIngestCursor(c *IngestCursor) error
	GetIngestCursor(sourceID uuid.UUID) (*IngestCursor, error)
}
```

```go
	SourceStore
	SinkStore
	IngestCursorStore
	ConnectorHistoryStore
```

In `archive/local.go`, add a facade block:

```go
// --- core.IngestCursorStore implementation ---

func (a *LocalArchive) SaveIngestCursor(c *core.IngestCursor) error {
	return a.saveIngestCursor(c)
}

func (a *LocalArchive) GetIngestCursor(sourceID uuid.UUID) (*core.IngestCursor, error) {
	return a.getIngestCursor(sourceID)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./archive/ -run TestIngestCursor && go build ./...`
Expected: PASS, and the whole module builds (Archive interface satisfied).

- [ ] **Step 6: Commit**

```bash
git add core/archive.go archive/ingest_cursor_store.go archive/local.go archive/ingest_cursor_store_test.go
git commit -m "feat(archive): add per-source ingest cursor store

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Connection store with encrypted credentials

The store encrypts `Connection.Credentials` via the injected `core.SecretCipher`. Credentialed writes fail closed (`core.ErrSecretKeyMissing`) when no cipher is set.

**Files:**
- Create: `archive/connection_store.go`
- Modify: `archive/local.go` (cipher field + `SetSecretCipher` + facade)
- Modify: `core/archive.go` (add `ConnectionStore`, compose)
- Test: `archive/connection_store_test.go`

- [ ] **Step 1: Write the failing tests**

Create `archive/connection_store_test.go`:

```go
package archive

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/secrets"
)

func withCipher(t *testing.T, a *LocalArchive) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 7)
	}
	c, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	a.SetSecretCipher(c)
}

func sampleConnection(ownerID uuid.UUID) *core.Connection {
	return &core.Connection{
		ID:          core.NewID(),
		OwnerID:     ownerID,
		Name:        "my-discourse",
		Connector:   core.ConnectorDiscourse,
		AuthType:    core.AuthAPIKey,
		Config:      map[string]any{"base_url": "https://forum.example"},
		Credentials: map[string]any{"api_key": "super-secret"},
		Status:      core.StatusActive,
		CreatedBy:   ownerID,
	}
}

func TestSaveConnection_EncryptsCredentials(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	owner := core.NewID()

	conn := sampleConnection(owner)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	if conn.ShortID == "" {
		t.Fatal("expected a CONN short ID")
	}

	// Raw column must not contain the plaintext secret.
	var raw []byte
	if err := a.db.QueryRow(`SELECT credentials FROM connections WHERE id = ?`, conn.ID.String()).Scan(&raw); err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("credentials column is empty")
	}
	if string(raw) == `{"api_key":"super-secret"}` || contains(raw, "super-secret") {
		t.Fatal("credentials stored in plaintext")
	}

	// Round-trip decrypts.
	got, err := a.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if got.Credentials["api_key"] != "super-secret" {
		t.Fatalf("credentials not round-tripped: %v", got.Credentials)
	}
}

func TestSaveConnection_FailsClosedWithoutCipher(t *testing.T) {
	a := setupTestArchive(t)
	owner := core.NewID()
	if err := a.SaveConnection(sampleConnection(owner)); !errors.Is(err, core.ErrSecretKeyMissing) {
		t.Fatalf("expected ErrSecretKeyMissing, got %v", err)
	}
}

func TestGetConnectionReferences(t *testing.T) {
	a := setupTestArchive(t)
	withCipher(t, a)
	repo := createTestRepo(t, a)

	conn := sampleConnection(repo.OwnerID)
	if err := a.SaveConnection(conn); err != nil {
		t.Fatalf("SaveConnection: %v", err)
	}
	src := &core.Source{
		ID: core.NewID(), RepoID: repo.ID, Name: "s", Connector: core.ConnectorDiscourse,
		ConnectionID: &conn.ID, CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	refs, err := a.GetConnectionReferences(conn.ID)
	if err != nil {
		t.Fatalf("GetConnectionReferences: %v", err)
	}
	if len(refs) != 1 || refs[0].EntityType != "source" {
		t.Fatalf("expected one source reference, got %v", refs)
	}
}

func contains(b []byte, s string) bool {
	return len(b) >= len(s) && (string(b) == s || indexOf(b, s) >= 0)
}

func indexOf(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags sqlite_fts5 ./archive/ -run 'TestSaveConnection|TestGetConnectionReferences'`
Expected: FAIL — `SaveConnection`/`GetConnection`/`SetSecretCipher`/`GetConnectionReferences` undefined.

- [ ] **Step 3: Add the cipher to LocalArchive**

In `archive/local.go`, add a field to the struct and a setter:

```go
type LocalArchive struct {
	path         string
	contentDir   string
	db           *sql.DB
	ftsAvailable bool
	cipher       core.SecretCipher
}
```

```go
// SetSecretCipher injects the cipher used to encrypt connection credentials at
// rest. Until set, writes that carry credentials fail with core.ErrSecretKeyMissing.
func (a *LocalArchive) SetSecretCipher(c core.SecretCipher) { a.cipher = c }
```

- [ ] **Step 4: Implement the connection store**

Create `archive/connection_store.go`:

```go
package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveConnection persists a connection, encrypting its credentials at rest.
// Version == 0 creates (generates CONN short ID); Version > 0 updates with an
// optimistic concurrency check.
func (a *LocalArchive) saveConnection(conn *core.Connection) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	configJSON, err := json.Marshal(conn.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal connection config: %w", err)
	}

	credsBlob, err := a.encryptCredentials(conn.Credentials)
	if err != nil {
		return err
	}

	var rateLimitJSON []byte
	if conn.RateLimit != nil {
		rateLimitJSON, err = json.Marshal(conn.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal connection rate limit: %w", err)
		}
	}

	now := time.Now().UTC()

	if conn.Version == 0 {
		seq, err := nextUserSequence(a.db, conn.OwnerID, "connection")
		if err != nil {
			return fmt.Errorf("failed to generate connection short ID: %w", err)
		}
		conn.ShortID = core.FormatShortID(core.PrefixConnection, seq)
		conn.Version = 1
		conn.CreatedAt = now
		conn.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO connections (id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, conn.ID.String(), conn.ShortID, conn.OwnerID.String(), conn.Name,
			string(conn.Connector), string(conn.AuthType), string(configJSON), credsBlob,
			string(conn.Status), stringOrNil(rateLimitJSON), timePtrString(conn.CredentialExpiry),
			timePtrString(conn.LastCheckedAt), strPtrAny(conn.LastError), conn.Version,
			conn.CreatedBy.String(), conn.CreatedAt.Format(time.RFC3339), conn.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert connection: %w", err)
		}
		return nil
	}

	conn.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE connections
		SET name = ?, connector = ?, auth_type = ?, config = ?, credentials = ?, status = ?, rate_limit = ?, credential_expiry = ?, last_checked_at = ?, last_error = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, conn.Name, string(conn.Connector), string(conn.AuthType), string(configJSON), credsBlob,
		string(conn.Status), stringOrNil(rateLimitJSON), timePtrString(conn.CredentialExpiry),
		timePtrString(conn.LastCheckedAt), strPtrAny(conn.LastError),
		conn.UpdatedAt.Format(time.RFC3339), conn.ID.String(), conn.Version)
	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{EntityType: "connection", EntityID: conn.ID, Expected: conn.Version}
	}
	conn.Version++
	return nil
}

// encryptCredentials marshals and encrypts credentials. Empty credentials store
// as NULL; non-empty credentials without a configured cipher fail closed.
func (a *LocalArchive) encryptCredentials(creds map[string]any) (any, error) {
	if len(creds) == 0 {
		return nil, nil
	}
	if a.cipher == nil {
		return nil, core.ErrSecretKeyMissing
	}
	plain, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}
	ct, err := a.cipher.Encrypt(plain)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}
	return ct, nil
}

// getConnection retrieves a connection by UUID, decrypting credentials.
func (a *LocalArchive) getConnection(connID uuid.UUID) (*core.Connection, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at
		FROM connections WHERE id = ?
	`, connID.String())
	return a.scanConnection(row)
}

// listConnections returns paginated connections for an owner.
func (a *LocalArchive) listConnections(ownerID uuid.UUID, opts core.ConnectionListOptions) ([]core.Connection, int, error) {
	where := `WHERE owner_id = ?`
	args := []any{ownerID.String()}
	if opts.Connector != "" {
		where += ` AND connector = ?`
		args = append(args, string(opts.Connector))
	}
	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, string(opts.Status))
	}

	var total int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM connections `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count connections: %w", err)
	}

	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "name":
		sortBy = opts.SortBy
	}
	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, short_id, owner_id, name, connector, auth_type, config, credentials, status, rate_limit, credential_expiry, last_checked_at, last_error, version, created_by, created_at, updated_at
		FROM connections %s ORDER BY %s %s LIMIT ? OFFSET ?`, where, sortBy, order)
	rows, err := a.db.Query(query, append(args, opts.Limit, opts.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	var conns []core.Connection
	for rows.Next() {
		c, err := a.scanConnection(rows)
		if err != nil {
			return nil, 0, err
		}
		conns = append(conns, *c)
	}
	return conns, total, nil
}

// deleteConnection deletes a connection by UUID.
func (a *LocalArchive) deleteConnection(connID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM connections WHERE id = ?`, connID.String())
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("connection", connID.String())
	}
	return nil
}

// getConnectionReferences returns sources and sinks referencing a connection,
// used to block deletion of an in-use connection.
func (a *LocalArchive) getConnectionReferences(connID uuid.UUID) ([]core.FilterReference, error) {
	var refs []core.FilterReference
	for _, q := range []struct{ table, kind string }{{"sources", "source"}, {"sinks", "sink"}} {
		rows, err := a.db.Query(
			fmt.Sprintf(`SELECT id, name FROM %s WHERE connection_id = ?`, q.table), connID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to query %s references: %w", q.table, err)
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan %s reference: %w", q.table, err)
			}
			refs = append(refs, core.FilterReference{EntityType: q.kind, EntityID: id, EntityName: name})
		}
		rows.Close()
	}
	return refs, nil
}

// rowScanner abstracts *sql.Row and *sql.Rows for a shared scan helper.
type rowScanner interface{ Scan(dest ...any) error }

func (a *LocalArchive) scanConnection(s rowScanner) (*core.Connection, error) {
	var conn core.Connection
	var idStr, ownerStr, connectorStr, authStr, configStr, statusStr, createdByStr, createdAt, updatedAt string
	var credsBlob []byte
	var rateLimitStr, credExpiry, lastChecked, lastErr sql.NullString

	err := s.Scan(&idStr, &conn.ShortID, &ownerStr, &conn.Name, &connectorStr, &authStr,
		&configStr, &credsBlob, &statusStr, &rateLimitStr, &credExpiry, &lastChecked, &lastErr,
		&conn.Version, &createdByStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("connection", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan connection: %w", err)
	}

	if conn.ID, err = uuid.Parse(idStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection ID: %w", err)
	}
	if conn.OwnerID, err = uuid.Parse(ownerStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection owner_id: %w", err)
	}
	if conn.CreatedBy, err = uuid.Parse(createdByStr); err != nil {
		return nil, fmt.Errorf("failed to parse connection created_by: %w", err)
	}
	conn.Connector = core.ConnectorType(connectorStr)
	conn.AuthType = core.AuthType(authStr)
	conn.Status = core.ResourceStatus(statusStr)

	if configStr != "" {
		if err := json.Unmarshal([]byte(configStr), &conn.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection config: %w", err)
		}
	}
	if len(credsBlob) > 0 {
		if a.cipher == nil {
			return nil, core.ErrSecretKeyMissing
		}
		plain, err := a.cipher.Decrypt(credsBlob)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
		}
		if err := json.Unmarshal(plain, &conn.Credentials); err != nil {
			return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
		}
	}
	if rateLimitStr.Valid && rateLimitStr.String != "" {
		var rl core.RateLimit
		if err := json.Unmarshal([]byte(rateLimitStr.String), &rl); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connection rate limit: %w", err)
		}
		conn.RateLimit = &rl
	}
	if conn.CredentialExpiry, err = parseNullableTime(credExpiry); err != nil {
		return nil, err
	}
	if conn.LastCheckedAt, err = parseNullableTime(lastChecked); err != nil {
		return nil, err
	}
	if lastErr.Valid && lastErr.String != "" {
		s := lastErr.String
		conn.LastError = &s
	}
	if conn.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("failed to parse connection created_at: %w", err)
	}
	if conn.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return nil, fmt.Errorf("failed to parse connection updated_at: %w", err)
	}
	return &conn, nil
}

// timePtrString formats a *time.Time as RFC3339 for SQL, or nil.
func timePtrString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// parseNullableTime parses an optional RFC3339 timestamp column.
func parseNullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	return &t, nil
}
```

- [ ] **Step 5: Interface and facade**

In `core/archive.go`, add the sub-interface and compose it into `Archive`:

```go
// ConnectionStore handles owner-scoped connection persistence. Credentials are
// encrypted at rest by the implementation.
type ConnectionStore interface {
	SaveConnection(conn *Connection) error
	GetConnection(connID uuid.UUID) (*Connection, error)
	ListConnections(ownerID uuid.UUID, opts ConnectionListOptions) ([]Connection, int, error)
	DeleteConnection(connID uuid.UUID) error
	// GetConnectionReferences returns sources/sinks referencing the connection,
	// for delete integrity. Reuses FilterReference for the {type,id,name} shape.
	GetConnectionReferences(connID uuid.UUID) ([]FilterReference, error)
}
```

```go
	SinkStore
	ConnectionStore
	IngestCursorStore
```

In `archive/local.go`, add the facade block:

```go
// --- core.ConnectionStore implementation ---

func (a *LocalArchive) SaveConnection(conn *core.Connection) error {
	return a.saveConnection(conn)
}

func (a *LocalArchive) GetConnection(connID uuid.UUID) (*core.Connection, error) {
	return a.getConnection(connID)
}

func (a *LocalArchive) ListConnections(ownerID uuid.UUID, opts core.ConnectionListOptions) ([]core.Connection, int, error) {
	return a.listConnections(ownerID, opts)
}

func (a *LocalArchive) DeleteConnection(connID uuid.UUID) error {
	return a.deleteConnection(connID)
}

func (a *LocalArchive) GetConnectionReferences(connID uuid.UUID) ([]core.FilterReference, error) {
	return a.getConnectionReferences(connID)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./archive/ -run 'TestSaveConnection|TestGetConnectionReferences'`
Expected: PASS (3 tests).

- [ ] **Step 7: Full build and test sweep**

Run: `go build ./... && go test -tags sqlite_fts5 ./...`
Expected: builds; all archive/secrets/core tests pass. (`go test ./...` without the tag also passes, skipping search.)

- [ ] **Step 8: Commit**

```bash
git add core/archive.go archive/connection_store.go archive/local.go archive/connection_store_test.go
git commit -m "feat(archive): connection store with encrypted credentials (ADR-0013/0014)

Owner-scoped CONN-{seq} connections; credentials encrypted via the injected
SecretCipher (fail-closed without a key); GetConnectionReferences blocks delete
of an in-use connection.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-review

**Spec coverage (A1 scope of the design doc §3, §4.1):**
- Connection entity (owner-scoped, AuthType, split config/credentials, status incl. `pending_auth`, expiry/health) — Tasks 1, 3, 7. ✓
- Node provenance (`SourceID`/`ExternalID`, dedup lookup, search-attribution field) — Tasks 1, 3, 4. ✓
- Source/Sink `ConnectionID` — Tasks 1, 3, 5. ✓
- Adapter contract + role interfaces + capabilities + `ConnectionSchema` — Task 1. ✓ (registry/pipeline = A2.)
- Ingest data-flow types + `IngestCursor` + cursor store — Tasks 1, 3, 6. ✓
- `SecretCipher` port + AES-GCM impl + env key + fail-closed — Tasks 1, 2, 7. ✓
- Delete integrity (`GetConnectionReferences`) — Task 7. ✓
- Archive interface composition — Tasks 4, 6, 7. ✓

Deferred to later A-slices (correctly out of A1): adapter registry + ingest pipeline (A2); connection HTTP module + `TestConnection` wiring (A3); OAuth endpoints + Mastodon (A4); webhook/repo migration (A2). Noted in the plan header.

**Placeholder scan:** none — every step contains concrete code or an exact command.

**Type consistency:** `SourceID *uuid.UUID`/`ExternalID *string` (Task 1) match `GetNodeBySourceExternalID(uuid.UUID, string)` (Tasks 4, 6-interface). `Connection.Credentials map[string]any` (Task 1) matches `encryptCredentials(map[string]any)` (Task 7). `IngestCursor{SourceID, Cursor, UpdatedAt}` (Task 1) matches the store (Task 6). Helper names (`uuidPtrString`, `strPtrAny`, `timePtrString`, `stringOrNil`, `parseNullableTime`) are defined once and reused. `core.Publisher` (adapter role) is explicitly distinguished from `app.Publisher`.

> **Tag reminder:** all archive test/build commands use `-tags sqlite_fts5` (per CLAUDE.md), so the search path compiles during the sweep.
