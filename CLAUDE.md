# Claude Instructions for Upspeak

## Project Overview

Upspeak is a personal-first, federated knowledge infrastructure designed to collect, organise, and synthesise data from web sources and your own inputs. It follows a modular, event-driven architecture built on domain-driven design principles.

**Architecture:**
- **API-first**: Pure API server, no bundled UI. Clients connect over HTTP
- **Modular design**: Each module implements the `app.Module` interface for HTTP and message handlers
- **Hybrid sync core + NATS JetStream**: Synchronous writes to archive (SQLite + files), NATS JetStream for downstream events
- **Hexagonal architecture**: Domain layer (`core/`) separated from infrastructure (`archive/`, `nats/`)
- **NATS isolation**: All NATS code lives in `nats/` — no other package imports nats-io
- **Local/remote archive split**: `core.Archive` interface supports both local (SQLite + files) and remote (Postgres + object storage) implementations
- **Knowledge graph**: Nodes, Edges, Threads, and Annotations form a structured graph with UUID v7 identifiers and human-friendly short IDs

**Key packages:**
- `app/`: Micro-framework for composing modules, HTTP routing, and application lifecycle. NATS-unaware — receives Publisher/Subscriber interfaces via DI
- `core/`: Domain models (Node, Edge, Thread, Annotation, User, Repository), Archive sub-interfaces, event types, identity system
- `archive/`: Local archive implementation (SQLite metadata + filesystem body storage). Implements `core.Archive`
- `nats/`: NATS JetStream infrastructure — embedded server, publisher, subscriber, stream lifecycle. Isolated from all other packages
- `repo/`: Repository CRUD and knowledge graph API module. Mounted at `/api/v1`
- `api/`: Response envelope, HTTP helpers, middleware (ETag, RequestID)

## Critical Rules

1. **ALWAYS** follow patterns established in `app/` and `core/` packages
2. **ALWAYS** add GoDoc-style comments for all public functions and types
3. **ALWAYS** add comments for longer private methods (>20 lines)
4. **ALWAYS** write documentation in en-IN (Indian English: "organise", "behaviour", "colour")
5. **ALWAYS** make small commits per logical chunk of work, not monolithic batches
6. **NEVER** respond with summaries unless explicitly requested
7. **NEVER** skip error handling — check and handle all errors immediately
8. **NEVER** use `panic` for normal error conditions
9. **NEVER** create deep nesting — extract functions or use early returns
10. **NEVER** add repository directory structure to README — structure is ephemeral
11. **NEVER** put NATS imports in any package other than `nats/`

## Build Commands

```bash
# Build the binary
./build.sh build

# Development mode (requires upspeak.yaml)
./build.sh dev

# Clean artifacts
./build.sh cleanup

# Run tests
go test ./...
```

## Identity System

All entities use **UUID v7** as primary key (time-ordered, via `google/uuid`). Each entity also carries a **short ID** — a human-friendly `{PREFIX}-{SEQ}` identifier:

- `REPO-1`, `NODE-42`, `EDGE-15`, `THREAD-7`, `ANNO-3`
- Short ID sequences are scoped: per-repo (nodes, edges, threads, annotations), per-user (repos), or global (jobs, schedules, users)
- `core.NewID()` generates a UUID v7. `core.FormatShortID(prefix, seq)` formats a short ID
- `core.ParseShortID(s)` extracts prefix and sequence number
- Sequence generation is internal to `archive/` — not exposed through `core.Archive`

## Archive Interface

`core.Archive` is composed of sub-interfaces that both local and remote implementations can satisfy:

```go
type Archive interface {
    RepositoryStore   // SaveRepository, GetRepository, ListRepositories, DeleteRepository, slug management
    NodeStore         // SaveNode, SaveBatchNodes, GetNode, DeleteNode, ListNodes, GetNodeEdges, GetNodeAnnotations
    EdgeStore         // SaveEdge, SaveBatchEdges, GetEdge, DeleteEdge, ListEdges
    ThreadStore       // SaveThread, GetThread, DeleteThread, ListThreads, AddNodeToThread, RemoveNodeFromThread
    AnnotationStore   // SaveAnnotation, GetAnnotation, DeleteAnnotation, ListAnnotations
    RefResolver       // ResolveRef — resolves short ID or UUID to (uuid, entityType, error)
}
```

**Local archive storage split:**
- **Metadata** (SQLite): type, subject, content_type, edges, config — everything queryable
- **Node body content** (filesystem): stored at `{archive_path}/content/{node_id}` as files
- This mirrors the high-level architecture: local = SQLite + files, remote = Postgres + object storage

**Optimistic concurrency:** All entities carry a `Version` field (integer, starts at 1). Write methods check `Version` — if stored version doesn't match, returns `VersionConflictError`. HTTP layer maps this to ETag/If-Match headers and 412 responses.

**Batch methods** take `[]*Node` or `[]*Edge` — each entity has `RepoID` already set by the caller.

**List methods** use typed option structs: `NodeListOptions{Type, ListOptions}`, `EdgeListOptions{Source, Target, Type, ListOptions}`.

## Module Development

All modules implement the `app.Module` interface:

```go
type Module interface {
    Name() string
    Init(config map[string]any) error
    HTTPHandlers() []HTTPHandler   // No parameters — dependencies via setters
    MsgHandlers() []MsgHandler     // No parameters — dependencies via setters
}
```

Dependencies (archive, publisher, consumer) are injected via setter methods (e.g., `SetArchive()`, `SetPublisher()`), not via handler method parameters.

**Module mounting:** All API modules mount at `/api/v1`. Multiple modules can share the same mount path — `http.ServeMux` resolves by method+path specificity.

**Lifecycle:** Call `up.InitModules()` to initialise all modules, then wire cross-module dependencies (e.g. `SetArchive()`), then `up.Start()` to start HTTP. This avoids a race where handlers serve requests before dependencies are wired.

## NATS Communication

All NATS code is isolated in the `nats/` package. Other modules interact via interfaces defined in `app/`:

```go
// Fire-and-forget publishing (JetStream-backed, delivery confirmed).
type Publisher interface {
    Publish(subject string, data []byte) error
}

// Core NATS fan-out subscriptions (push-based).
type Subscriber interface {
    Subscribe(subject string, handler func(subject string, data []byte)) error
}

// Work queue consumption (JetStream pull consumer, explicit ack).
type Consumer interface {
    Fetch(maxMsgs int, timeout time.Duration) ([]*Msg, error)
}
```

**Publisher** uses `js.Publish()` (not `nc.Publish()`) so the server confirms persistence before returning. **Subscriber** uses core NATS for simple fan-out. **Consumer** wraps a JetStream durable pull subscription with `app.Msg` providing Ack/Nak/InProgress/Term methods.

**Event subject format:** `repo.{repo_id}.events.{EventType}` (e.g., `repo.{uuid}.events.NodeCreated`)

**JetStream streams:**
- Per-repo: `REPO_{repo_id}_EVENTS` — Limits retention, captures `repo.{repo_id}.events.>`
- Jobs: `JOBS` — WorkQueue retention, captures `jobs.>`
- Schedules: `SCHEDULES` — WorkQueue retention, captures `schedules.trigger.>` (planned, Phase 4)

**Consumers:** Durable pull consumers with explicit ack. `job-runner` on JOBS stream (MaxDeliver=5, AckWait=30s). Additional consumers (rules-engine, scheduler, connector-repo, realtime-ws, sync-outbound) added as phases implement them.

**Connection management:** Drain() on shutdown (not Close()), infinite reconnect with jitter, handler callbacks for disconnect/reconnect/error logging.

## HTTP API Conventions

**Response envelope:** All responses use `{"data": ..., "meta": {...}, "error": {...}}`

**Flat URL routing:** Entities are accessed at `/api/v1/repos/{repo_ref}/{entity_ref}` — the short ID prefix encodes the type. Collection endpoints use typed paths (`/nodes`, `/edges`, `/threads`, `/annotations`).

**Ref resolution:** `{repo_ref}` can be UUID, short ID, or slug. `{entity_ref}` can be UUID or short ID. Old slugs return 301 redirects.

**Pagination:** `?limit=20&offset=0&sort_by=created_at&order=desc`

## Configuration

**YAML-based:** See `upspeak.sample.yaml` for structure.

```yaml
name: "upspeak"
nats:
  embedded: true
  private: false
  logging: false
http:
  port: 8080
modules:
  archive:
    enabled: true
    config:
      type: local
      path: ./data
```

**First-time setup:** `cp upspeak.sample.yaml upspeak.yaml`

## File Organisation

- **Logical separation**: One file per major concern or responsibility
- **Type definitions first**: Define types before functions that use them
- **Private helpers**: Use lowercase names for unexported functions
- **Co-located tests**: Place `*_test.go` files alongside implementation
- **New module location**: New modules are placed in the repo root directory

## Naming Conventions

**Types:** PascalCase — `Node`, `Edge`, `Repository`, `ErrorNotFound`, `HTTPHandler`
**Functions:** PascalCase exported, camelCase private. Constructor pattern: `New<Type>()`
**Variables:** Short for common patterns (`err`, `nc`, `ctx`, `w`, `r`). Single-letter receivers (`a *App`, `m *Module`)
**Constants:** Typed constants with semantic grouping (`EventType`, `ConnectorType`, `JobStatus`)

## Error Handling

Custom error types for domain errors (`ErrorNotFound`, `VersionConflictError`, `ErrorSlugRedirect`). Wrap errors with `fmt.Errorf("context: %w", err)`. Check immediately.

## Testing Standards

- Table-driven tests for multiple cases
- Meaningful test names: `TestSaveNode_VersionConflict`
- Test error cases and edge conditions
- Co-locate test files with implementation
- Use `setupTestArchive(t)` pattern for archive tests (creates temp dir, auto-cleanup)

## Implementation Plan

The full API foundation is implemented in 6 phases. See `docs/specs/api-foundation/` for the complete spec and `docs/superpowers/plans/2026-03-30-api-foundation.md` for the implementation plan.

**Completed:** Phase 1 (foundation), Phase 2 (knowledge graph), Correction Pass (archive interface alignment), NATS hardening pass
**Next:** Phase 3 (filters + jobs)
**After Phase 3:** Phases 4 (connectors + schedules), 5 (rules + search), 6 (real-time + sync) can proceed in parallel

## Common Pitfalls

1. **Node body is NOT in SQLite** — Body content is stored as files at `{archive_path}/content/{node_id}`. The `nodes` table has no `body` column.
2. **Sequence methods are private** — `nextRepoSequence`, `nextUserSequence`, `nextGlobalSequence` are package-private functions in `archive/sequences.go`, not on the `core.Archive` interface.
3. **Module interface has no parameters** — `HTTPHandlers()` and `MsgHandlers()` take no arguments. Dependencies injected via setter methods.
4. **HTTP method patterns** — Always specify HTTP method in route patterns (e.g., `GET /api/nodes`) to avoid conflicts.
5. **Reserved paths** — Never mount modules at `/healthz` or `/readiness` (system endpoints).
6. **NATS isolation** — Only the `nats/` package imports `github.com/nats-io/*`. All other packages use `app.Publisher`/`app.Subscriber`/`app.Consumer` interfaces.
7. **Short IDs are immutable** — Once assigned, a short ID never changes. Sequences never reuse numbers.
8. **Batch methods don't take repoID** — Each entity in the batch already has `RepoID` set by the caller.
9. **Use JetStream publish** — The publisher uses `js.Publish()`, not `nc.Publish()`. Never use `nc.Publish()` for subjects captured by JetStream streams.
10. **Use Drain() not Close()** — On shutdown, always `Drain()` the NATS connection to flush buffered messages.
