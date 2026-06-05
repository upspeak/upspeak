# Design: Connector adapterisation — Connections, Adapters, and the ingestion pipeline

**Date:** 2026-06-05
**Authors:** Kaustav Das Modak, Claude
**Status:** Approved (design); implementation pending
**Supersedes (in part):** the Phase-4 inline connector-config approach in `connector/`

This document is the agreed design for Upspeak's **ingestion track** — the work that
turns the stubbed left edge of `assets/high-level-concepts-0.1.png` (Input / Import →
filter → populate Repository) and the right edge (Export / Publish) into real
behaviour. It is a forward-looking design spec, distinct from the decision records in
`docs/adr/` and the status tracker in `docs/next-steps.md`. The shipped code under
`app/`, `core/`, and the module packages remains the source of truth; where this design
and the code disagree, the code wins and this document is updated.

---

## 1. Context

The knowledge-graph spine (repos, nodes, edges, threads, annotations, filters, rules,
search, realtime) is shipped and event-driven. What is hollow is the **connector**
layer: every external connector backend is rejected by `isSupportedConnector()`, and
`jobs/runner.go`'s `executeCollect` / `executePublish` / `executeWebhook` record success
without doing real work. Upspeak can hold and reason over a graph but cannot yet pull
data into one or push it back out.

The Phase-4 connector model is **flat and credential-duplicating**:

- `Source` and `Sink` each carry a `Connector ConnectorType` and a single opaque
  `Config map[string]any` that must hold endpoint, credentials, *and* operation
  settings together.
- There is no shared notion of "a configured connection to an external system", so
  credentials would be re-entered per source and per sink.
- Validation is an inline `switch` in the connector module (`validateSourceConfig`,
  `isSupportedConnector`) — it grows with every new connector type.
- `connector/` imports `jobs/` to create jobs, so the job runner **cannot** import
  `connector/` to execute backends without forming an import cycle.

### Goals

1. Make connectors **adapterised**: each integration is a self-contained unit that
   declares its capabilities and maps external data ↔ the Upspeak domain.
2. Introduce a first-class, **configure-once Connection** reused across Sources, Sinks
   (and, by reference, the entities they feed).
3. Keep **all data modelling in `core`** — the domain package. Behaviour and
   infrastructure live outside it.
4. Prove the abstraction against three distinct **connection modes** plus OAuth, so it
   is not secretly shaped to a single integration.
5. Break the `connector ↔ jobs` import cycle cleanly, following the codebase's existing
   dependency-injection grammar.

### Non-goals (this track)

- Multi-device sync (Phase 6b) — `IngestCursor` is deliberately named to avoid colliding
  with that work.
- Real authentication / multi-tenancy — the `OwnerID` seam is used but auth stays
  stubbed (`defaultOwnerID`).
- Connector-specific filter engines — filters stay generic (ADR-0010) and operate on
  normalised payloads; adapters may later ship filter presets.

---

## 2. The connection modes (validation set)

The first integrations are chosen to span the modes the abstraction must survive. If all
four fall out of one model with no per-mode special-casing in `core`, the design holds.

| Integration | Mode | Auth | Direction | Execution |
|---|---|---|---|---|
| **RSS** | pull, public HTTP | none | read-only (Source) | JOBS collect job |
| **Discourse** | request/response REST, paginated | API key | read + write (Source + Sink) | JOBS collect/publish job |
| **Matrix** | streaming, long-lived | access token | read + write (Source + Sink) | **stream supervisor** (not a job) |
| **Mastodon / Fediverse** | request/response REST | **OAuth2** | read + write | JOBS + OAuth flow |

Matrix is the forcing function: a long-lived `/sync` loop does not fit the one-shot job
model and needs a dedicated long-running runner. Mastodon is the OAuth exerciser so the
OAuth machinery is not shipped untested.

---

## 3. Core model

All types below live in `core/`. Adapters import `core` only.

### 3.1 Connection (owner-scoped)

```go
// Connection is a configured, reusable link to one external system (a Discourse
// site, a Matrix homeserver account, a Mastodon account). Sources and Sinks
// reference a Connection for endpoint + credentials, so the secret is configured
// once and shared across every Source/Sink that uses that system.
type Connection struct {
    ID            uuid.UUID
    ShortID       string         // CONN-{seq}, per-user sequence
    OwnerID       uuid.UUID      // owner-scoped (not repo-scoped)
    Name          string
    Connector     ConnectorType
    AuthType      AuthType       // none | api_key | token | oauth2
    Config        map[string]any // NON-secret only; returned in API responses
    // Credentials: secret material (api_key, access_token, refresh_token).
    // Stored encrypted at rest, NEVER serialised to API responses (json:"-").
    Status           ResourceStatus // active | paused | error | rate_limited | pending_auth
    RateLimit        *RateLimit     // ONE shared budget for this external system
    CredentialExpiry *time.Time     // OAuth token expiry (the token itself is secret)
    LastCheckedAt    *time.Time     // last successful TestConnection (health)
    LastError        *string
    Version          int
    CreatedBy        uuid.UUID
    CreatedAt, UpdatedAt time.Time
}
```

- **Scope:** owner-scoped. Configure a Discourse account once, reference it from Sources
  and Sinks in any of the owner's repos. Managed at `/api/v1/connections` (owner-scoped
  via `defaultOwnerID` until auth lands) — **not** under `/repos/`, like jobs.
- **Short ID:** `CONN-{seq}` from the per-user sequence (like repos).
- **Delete integrity:** `GetConnectionReferences(connID)` returns referencing
  Sources/Sinks; delete returns `409` if any exist (mirrors filter delete).
- **New status:** `pending_auth` — an OAuth connection created but not yet authorised.

### 3.2 Source / Sink changes

```go
type Source struct {
    // ... existing fields ...
    ConnectionID *uuid.UUID     // nil for connection-less adapters (RSS, webhook)
    Config       map[string]any // ONLY operation-specific: feed_url, category_id, room_id …
}
```

- `ConnectionID` is **nullable**: RSS and webhook need no shared Connection (RSS's
  `feed_url` lives in `Source.Config`); Discourse/Matrix/Mastodon require one. The
  adapter declares this via `Capabilities().RequiresConnection`, enforced at create time.
- When a Connection is set, the Source's `Connector` must match the Connection's.
- **Rate-limit moves up to the Connection** — one external system, one shared budget. A
  per-source override may remain as a secondary cap.
- **A Source represents one container** — a Matrix room or a Discourse category — or a
  single feed/URL. See §5.

### 3.3 Node provenance

```go
type Node struct {
    // ... existing fields ...
    SourceID   *uuid.UUID // which Source ingested this node (nil for manual nodes)
    ExternalID *string    // stable id in the external system
}
```

One nullable pair, unique together where set, indexed. It does three jobs:

1. **Idempotent re-collection** — `GetNodeBySourceExternalID(sourceID, externalID)`
   finds an existing node so re-polling updates instead of duplicating.
2. **Container grouping** — "all nodes from this room/category" is `SourceID = S`.
3. **Search attribution** — closes `archive/search_store.go` stub #7 (browse `SourceID`
   filter) directly.

### 3.4 Adapter contract

Capability-based: a base contract every adapter satisfies, plus optional role interfaces
the registry type-asserts. An adapter implements only the roles it supports.

```go
type AdapterCapabilities struct {
    Collect, Publish, Stream, OAuth, RequiresConnection bool
}

type Adapter interface {
    Type() ConnectorType
    Capabilities() AdapterCapabilities
    ConnectionSchema() ConnectionSchema           // declarative field specs (see §4)
    ValidateConnectionConfig(cfg map[string]any) error
    ValidateSourceConfig(cfg map[string]any) error
    ValidateSinkConfig(cfg map[string]any) error
    TestConnection(ctx context.Context, conn *Connection) error
}

// Optional role interfaces.
type Collector interface { Collect(ctx context.Context, req CollectRequest) (*IngestBatch, error) }
type Publisher interface { Publish(ctx context.Context, req PublishRequest) (*PublishResult, error) }
type Streamer  interface { Stream(ctx context.Context, req StreamRequest, emit func(*IngestBatch) error) error }
type OAuthProvider interface {
    AuthCodeURL(conn *Connection, redirectURI, state string) (string, error)
    Exchange(ctx context.Context, conn *Connection, code, redirectURI string) (OAuthTokens, error)
    Refresh(ctx context.Context, conn *Connection, refreshToken string) (OAuthTokens, error)
}
```

> **Naming note:** `core.Publisher` is a different package from `app.Publisher` (the
> event bus). They compile side-by-side; if the overlap proves confusing in review, the
> adapter role may be renamed (e.g. `Emitter`).

```go
type CollectRequest struct { Connection *Connection; Source *Source; Cursor *IngestCursor }
type StreamRequest  struct { Connection *Connection; Source *Source; Cursor *IngestCursor }
type PublishRequest struct {
    Connection *Connection
    Sink       *Sink
    Items      []PublishItem // each node + its resolved reply-in-context target
}
type PublishItem  struct { Node *Node; InReplyToExternalID, ThreadExternalID string }
type PublishResult struct { ExternalIDs []string }
type OAuthTokens   struct { AccessToken, RefreshToken string; Expiry time.Time; Scopes []string }
```

### 3.5 Data-flow types

Adapters speak **external IDs only** — they never see internal UUIDs or the archive. The
pipeline owns identity. This is the hexagonal boundary (ADR-0002) for ingestion.

```go
type IngestBatch struct {
    Threads     []IngestThread      // conversations (Discourse topic, Matrix m.thread) → Threads
    Items       []IngestItem        // messages/posts → Nodes (+ reply Edges)
    Annotations []IngestAnnotation  // reactions/likes → Annotations
    Tombstones  []string            // external IDs to redact/delete
    Cursor      *IngestCursor       // advanced cursor after this batch (nil = unchanged)
}

type IngestThread struct {
    ExternalThreadID string
    Subject          string
    Metadata         []Metadata
}

type IngestItem struct {
    ExternalID       string
    ThreadExternalID string         // optional; absent → attach to the Source (flat timeline)
    ParentExternalID string         // optional → reply Edge to the parent item's node
    Node             *Node          // content only: Type/Subject/ContentType/Body/Metadata
    Author           *ExternalActor // → find/create User
}

type IngestAnnotation struct {
    ExternalID       string
    TargetExternalID string         // the node this annotates
    Motivation       string         // "like", an emoji, "assessing" …
    Body             json.RawMessage
    Author           *ExternalActor
}

type ExternalActor struct { ExternalID, Username, Hostname, DisplayName string }

// IngestCursor persists an adapter-defined resumption point per Source. The Cursor
// payload is opaque to core — RSS stores etag+last-guid, Discourse a high-water post
// id, Matrix a since-token. Distinct from Phase 6b multi-device sync.
type IngestCursor struct {
    SourceID  uuid.UUID
    Cursor    json.RawMessage
    UpdatedAt time.Time
}
```

### 3.6 Ports (interfaces in `core`, implementations in infra)

```go
// SecretCipher encrypts/decrypts credential material at rest. Implemented outside
// core (AES-GCM, master key from config/env); injected into the archive.
type SecretCipher interface {
    Encrypt(plaintext []byte) ([]byte, error)
    Decrypt(ciphertext []byte) ([]byte, error)
}
```

A small adapter-registry lookup interface (`AdapterFor(ConnectorType) (Adapter, bool)`)
is consumed by the job runner and connector module; it lives alongside `app.Publisher` /
`app.Consumer` (behaviour interface) and is injected via setter.

---

## 4. Secure credentials & OAuth

### 4.1 Storage

- **Split is structural.** `Connection.Config` (non-secret, returned in API) is separate
  from `Credentials` (secret, `json:"-"`, encrypted at rest in a dedicated column/table).
- **At rest:** AES-GCM via `core.SecretCipher`; master key from config/env
  (`UPSPEAK_SECRET_KEY`). The cipher sits above storage, so encrypted credentials are
  portable across the local (SQLite) / remote (Postgres) archive split (ADR-0005) with
  no re-keying.
- **API contract:** create/update accept credentials write-only; `GET` returns `Config`
  + a `credential_status` (set/unset, optional last-4) but never the secret. Logs redact.
- **Adapters declare their fields** via `ConnectionSchema` — a list of
  `FieldSpec{Name, Secret, Required, AuthType}`. One declaration drives validation,
  Config-vs-Credentials routing, and redaction, replacing the inline per-connector
  `switch`.

### 4.2 OAuth lifecycle (shipped in the framework spine)

1. Create connection → `pending_auth`.
2. `GET /api/v1/connections/{id}/authorize` → `AuthCodeURL` → 302 to provider (with a
   short-lived `state` CSRF token bound to `(connection, owner)`).
3. Provider redirects to `GET /api/v1/connections/oauth/callback?code&state` → `Exchange`
   → store tokens encrypted → `active`.
4. **Lazy refresh:** before any adapter call, if `AuthType==oauth2` and near
   `CredentialExpiry`, call `Refresh` and persist new tokens.

Mastodon/Fediverse is the first real `OAuthProvider`, so the flow ships exercised.

---

## 5. External → Upspeak mapping

The external systems share a four-level shape; it maps level-for-level. **A container
(room/category) is a Source, not a Thread.**

| External level | Discourse | Matrix | Upspeak |
|---|---|---|---|
| Account / server | site | homeserver | **Connection** |
| **Container** (long-lived context) | category | room | **Source** (+ `Node.SourceID` provenance) |
| **Conversation** (ordered, one subject) | topic | `m.thread` / reply-chain | **Thread** |
| Message | post | message event | **Node** |
| Relation | reply / mention | `m.in_reply_to` / `m.thread` | **Edge** |
| Reaction | like | `m.annotation` / reaction | **Annotation** (`Motivation`) |
| Author | user | sender `@u:server` | **User** (`username`@`hostname`) |
| Tags / labels | tags | — | `Node.Metadata` |
| Edit | post edit | `m.replace` | Node update (same `ExternalID`, version bump) |
| Delete | delete | `m.room.redaction` | Tombstone → delete/flag |

- **Container = Source.** One Connection fans out to many Sources (one per room/category).
  A flat Matrix room (no native threads) is just nodes grouped by `SourceID` with reply
  edges — no synthetic container node, no new entity. The Node/Edge/Thread/Annotation/User
  vocabulary stays frozen, so search/traversal/rules/realtime work unchanged.
- **Reactions = Annotations.** Matrix literally calls reactions "annotations"; Discourse
  likes are reactions; both map onto `core.Annotation.Motivation`.
- **Authors = Users.** `ExternalActor.username@hostname` populates `core.User`
  (`Hostname` was built for federated identity).

---

## 6. Execution & pipeline

### 6.1 One pipeline, two triggers

Because adapters emit external-ID-keyed `IngestBatch`es, a poll result and a stream event
are structurally identical. Both feed the **same** `ingest` pipeline:

```
pipeline.Ingest(source, batch):
  ensure Threads      (find/create by (SourceID, ExternalThreadID))
  for each Item:
    resolve node identity via (SourceID, ExternalID)          // dedup
    map Item.Node → core.Node (assign IDs, or load existing for update)
    apply Source filters on the normalised node → skip if no match
    attach to Thread (ThreadExternalID) or leave at Source level
    reply Edge to ParentExternalID's node, if any
    resolve Author → find/create User → Node.CreatedBy
    SaveBatchNodes / SaveBatchEdges
    publish NodeCreated/Updated/EdgeCreated events            // reuses the existing write path
  upsert Annotations (Edge → target node)
  apply Tombstones (delete/flag)
  persist batch.Cursor
```

**Reusing `SaveNode → PublishEvent(NodeCreated)` means rules, realtime, and FTS search
light up for ingested data with zero new wiring.** Ingestion is a non-HTTP producer of
the existing write path. Ingested events enter the rules engine at `Hops:0` (fresh
external input).

### 6.2 Pull (RSS, Discourse, Mastodon collect/publish)

`jobs/runner.go` `executeCollect` / `executePublish` resolve the Source/Sink (+
Connection), look up the adapter in the registry, and dispatch to `Collect` / `Publish`.
Cursor loaded/persisted around the call; history recorded; `CollectionCompleted` /
`PublishCompleted` events emitted (closes next-steps §4). Periodic polling reuses the
**existing scheduler** unchanged: `Schedule{cron, action:collect, source_id}` →
`ScheduleActionJobType` → JOBS.

### 6.3 Stream (Matrix)

A new **ingest supervisor** (`app.Runner`, started from `main` like the other runners)
manages one goroutine per enabled streaming Source whose Connection is `active`. Each
goroutine runs `adapter.Stream(ctx, req, emit)`, where `emit` calls `pipeline.Ingest`;
the cursor is persisted per emitted batch. Lifecycle: start on Source enable /
Connection activate; cancel on disable / pause / shutdown; reconnect with exponential
backoff + jitter on transient error; mark the Connection `error` on auth failure.

### 6.4 Rules & "reply in context"

- `collect` / `publish` / `webhook` rule actions create jobs that dispatch through the
  registry. `annotate` / `enrich` / `relate` stay pure graph operations.
- The README's "send replies back within their contexts": a `publish` action on a node
  carrying provenance resolves its `(Connection, ThreadExternalID / InReplyToExternalID)`
  so the sink adapter posts **in-reply-to** there. Provenance is bidirectional — the same
  `(SourceID, ExternalID)` that dedups on collect targets the reply on publish.
- **Loop bounding:** the rules `Hops` limit bounds rule→publish→collect→rule cascades;
  provenance dedup makes re-collecting a published post an update, not a duplicate; the
  existing connector cycle-detection covers repo→repo.

---

## 7. Package layout (breaks the `connector ↔ jobs` cycle)

```
core/                      domain types + Adapter interface + ports (no infra imports)
integrations/rss/          ─┐ implement core.Adapter, import core ONLY
integrations/discourse/     ├─ (no cross-imports between integrations)
integrations/matrix/        │
integrations/mastodon/     ─┘
ingest/                    pipeline (map→filter→persist→dedup→events) + registry + stream supervisor (app.Runner)
secrets/ (or crypto/)      SecretCipher implementation (AES-GCM, env key)
jobs/                      runner consults the injected adapter registry — no connector import
connector/                 Source/Sink CRUD; uses the registry for validation/capabilities (still imports jobs)
connection/                Connection CRUD + OAuth-flow module
main.go                    builds the registry from integrations, injects into jobs + connector, starts the supervisor
```

Adapters → `core`. The registry/pipeline live in `ingest`. `jobs` and `connector` depend
on the registry **interface** (injected), never on `integrations/`. `main.go` is the only
place that knows the concrete adapters. No cycle.

---

## 8. Decomposition

Each sub-project is its own spec → plan → implementation cycle. Order is strict: B needs
A's registry and pipeline; C needs B's proven pull pipeline plus the supervisor.

### Sub-project A — Connection + Adapter framework (the spine)

- `core`: `Connection`, `AuthType`, `Adapter` + role interfaces, `AdapterCapabilities`,
  `ConnectionSchema`/`FieldSpec`, `IngestBatch` family, `IngestCursor`, `Node` provenance
  fields, `SecretCipher` port, request/result types.
- `archive`: connection store (+ encrypted credentials), `IngestCursor` store, node
  provenance columns + `GetNodeBySourceExternalID`, `GetConnectionReferences`,
  per-user `CONN` sequence.
- `secrets`: AES-GCM cipher (env master key).
- `connection` module: CRUD + `TestConnection` + **full OAuth flow**
  (authorize/callback/lazy-refresh) + events.
- `ingest`: pipeline + registry + DI.
- `jobs` / `connector`: dispatch through the registry; **migrate `webhook` + `repo`**
  onto adapters.
- `integrations/mastodon`: the OAuth exerciser (real `OAuthProvider` + Collector/Publisher).
- **ADRs:** (1) Connection + Adapter registry (supersedes Phase-4 inline config);
  (2) secure credential storage; (3) OAuth connection lifecycle.

### Sub-project B — Pull execution + RSS & Discourse

- Wire `executeCollect` / `executePublish` cursors, filter-on-normalised, dedup.
- `integrations/rss` (collect-only, connection-less), `integrations/discourse`
  (Source + Sink, API key, pagination).
- Emit `CollectionCompleted` / `PublishCompleted`; light up the `jobs.{job}` realtime
  channel (needs a `JobUpdated` event — next-steps §6).
- Reuse the scheduler for periodic collect.

### Sub-project C — Streaming + Matrix

- The ingest supervisor `app.Runner`; Connection/Source enable-disable lifecycle;
  reconnect/backoff.
- `integrations/matrix` (sync loop, `since` token, rooms=Source, `m.thread`=Thread,
  messages=Nodes, replies=Edges, reactions=Annotations, redactions=Tombstones).
- **ADR:** streaming ingestion vs the one-shot job model.

---

## 9. Decisions captured (resolved during design)

| Decision | Choice |
|---|---|
| Track after Phase 6a | Ingestion (connectors) |
| Connector shape | Full adapterisation: Connection entity + Adapter registry |
| Validation integrations | RSS, Discourse, Matrix (+ Mastodon for OAuth) |
| Data modelling home | `core` (the domain) |
| Connection scope | Per-user (`OwnerID`), repo-referenced |
| Node provenance | Fields on `Node` (`SourceID`, `ExternalID`) |
| Secret storage | AES-GCM in DB, master key from config/env |
| OAuth in spine | Full flow (authorize/callback/refresh), exercised by Mastodon |
| Container (room/category) | A Source (provenance grouping); no synthetic node, no new entity |
| Thread-resolution | Explicit `IngestThread`; conversation = Thread, container = Source |

---

## 10. Stubs this track closes

From `docs/next-steps.md`: connector backends (§2), job executors (§3), operational
events (§4), thread publish (§5, via sinks), the `jobs.{job}` realtime channel (§6,
needs `JobUpdated`), and search source-attribution (§7, via `Node.SourceID`). The
`webhook` and `repo` connectors are migrated onto the adapter framework in Sub-project A.
