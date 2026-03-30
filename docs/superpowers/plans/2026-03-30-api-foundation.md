# API Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete Upspeak API foundation as specified in `docs/specs/api-foundation/`, transforming the existing prototype into a production-ready knowledge management API.

**Architecture:** Hybrid synchronous core + JetStream. Synchronous writes to SQLite archive, JetStream for downstream event propagation. All modules mount at `/api/v1`. NATS code isolated in a dedicated `nats/` module. UUID v7 replaces xid for all entity identifiers.

**Tech Stack:** Go 1.25+, SQLite (via mattn/go-sqlite3), embedded NATS JetStream, google/uuid (v7)

**Spec reference:** `docs/specs/api-foundation/00-index.md` (18 files)

---

## Phase Overview & Dependency Map

```
Phase 1: Foundation
  ├── Core domain models (UUID v7, ShortID, Version, UpdatedAt)
  ├── NATS isolation (app/nats.go → nats/ module)
  ├── Archive interface + SQLite rewrite (sequences, pagination, versioning)
  ├── API response envelope + HTTP helpers
  ├── Repository CRUD with slugs + redirects
  └── Remove ui/ module, update build.sh + main.go

Phase 2: Knowledge Graph API  [depends on Phase 1]
  ├── Node CRUD (batch, PATCH, metadata merge)
  ├── Edge CRUD (batch)
  ├── Thread CRUD (add/remove nodes)
  ├── Annotation CRUD (W3C selectors)
  ├── Flat URL routing + entity ref resolution
  ├── Optimistic concurrency middleware (ETag/If-Match)
  └── Cascading deletes via JetStream consumers

Phase 3: Filters + Jobs  [depends on Phase 1]
  ├── Filter CRUD + condition evaluation engine
  ├── Filter test endpoint
  ├── Job tracking + status + cancellation
  └── JetStream JOBS stream + job-runner consumer

Phase 4: Connectors + Schedules  [depends on Phase 2, Phase 3]
  ├── Source CRUD + rate limiting
  ├── Sink CRUD + rate limiting
  ├── Collect endpoint (one-shot ingestion)
  ├── Repo connector + cycle detection
  ├── Schedule CRUD + cron execution
  └── Schedule trigger/pause/resume

Phase 5: Rules + Search  [depends on Phase 2, Phase 3]
  ├── Rule CRUD + evaluation engine
  ├── Rule test endpoint + pause/resume
  ├── Full-text search (SQLite FTS5)
  ├── Browse feed
  └── Graph traversal

Phase 6: Real-time + Sync  [depends on Phase 2, Phase 3]
  ├── WebSocket connection management
  ├── Channel subscriptions + event filtering
  ├── Sync status + trigger
  ├── Conflict detection + resolution
  └── Peer management
```

Phases 3, 4, 5, and 6 can be parallelised after Phase 2, with the constraint that Phase 4 and 5 need Phase 3 (filters, jobs).

---

## Phase 1: Foundation

**Goal:** Replace the prototype internals with production-ready domain models, storage, and the first API surface (repositories). After this phase, the binary starts, serves `/api/v1/repos` endpoints, uses UUID v7, has isolated NATS, and all existing tests pass or are updated.

### File Structure

```
Files to CREATE:
  core/identity.go          — UUID v7 generation, short ID formatting, entity prefix constants
  core/shared_types.go      — Shared typed constants (ConnectorType, FilterMode, JobStatus, etc.)
  core/version.go           — VersionConflictError type
  core/list.go              — ListOptions, query option types, ListResult
  nats/nats.go              — Module implementing app.Module, embedded server, connection
  nats/publisher.go         — Publisher wrapping nats.Conn, event publishing
  nats/streams.go           — JetStream stream lifecycle (create/delete per repo)
  api/envelope.go           — Response envelope types (Success, Error, Meta)
  api/http.go               — HTTP helper functions (writeJSON, writeError, parsePagination, parseRef)
  api/middleware.go          — ETag/If-Match middleware, request ID middleware
  repo/handlers_repo.go     — Repository HTTP handlers (CRUD + slug redirect)
  repo/resolve.go           — Entity ref resolution logic (short ID, UUID, slug)
  archive/schema.go         — SQLite schema DDL as constants
  archive/sequences.go      — Short ID sequence generation (repo, user, global scopes)
  archive/repo_store.go     — Repository persistence (save, get, list, delete, slug redirects)

Files to MODIFY:
  go.mod                    — Add google/uuid, remove rs/xid
  main.go                   — New module composition, remove ui
  build.sh                  — Remove ui build steps
  upspeak.sample.yaml       — Update module list
  core/core.go              — Rewrite with UUID v7 types, new fields
  core/events.go            — New event types, input/output split, new subject format
  core/errors.go            — Update to uuid.UUID, add new error types
  core/archive.go           — Completely rewrite Archive interface
  core/repo.go              — Rewrite Repository as domain model (not aggregate)
  core/annotation.go        — Update with UUID, ShortID, Version, RepoID
  core/thread.go            — Update with UUID, ShortID, Version, RepoID
  archive/archive.go        — Rewrite module, new schema init, new store composition
  archive/local.go          — Rewrite SQLite implementation for new interface
  repo/repo.go              — Rewrite module for /api/v1 mounting, new handler registration
  repo/handlers_node.go     — Update to use new types (minimal — full rewrite in Phase 2)
  repo/handlers_edge.go     — Update to use new types (minimal — full rewrite in Phase 2)
  repo/handlers_thread.go   — Update to use new types (minimal — full rewrite in Phase 2)
  app/app.go                — Remove NATS imports, receive publisher via DI
  app/config.go             — Add nats module config section

Files to DELETE:
  ui/                       — Entire directory (API-first, no bundled UI)
  app/nats.go               — Moved to nats/ module

Files to UPDATE (tests):
  app/app_test.go           — Update for NATS-unaware app
  app/nats_test.go          — Move to nats/ module as nats/nats_test.go
  repo/repo_test.go         — Update for new types and handlers
  archive/local_test.go     — New comprehensive tests for rewritten archive
```

### Task 1: Dependencies and Project Cleanup

**Files:**
- Modify: `go.mod`
- Delete: `ui/` directory

- [ ] **Step 1: Add UUID dependency, remove xid**

```bash
cd /Users/kaustavdm/src/upspeak/upspeak
go get github.com/google/uuid
```

Note: Don't remove xid yet — we'll do that after migrating all code.

- [ ] **Step 2: Remove ui/ module**

Delete the entire `ui/` directory. The spec mandates API-first with no bundled UI.

- [ ] **Step 3: Update build.sh**

Remove `build-ui` function and the ui build step from the `build` function. The `build` function should only run `go build`. Remove the `dev` function's ui reference.

- [ ] **Step 4: Verify project compiles**

```bash
go build ./...
```

This will fail because main.go references ui — that's expected. We'll fix it in Task 11.

- [ ] **Step 5: Commit**

```
chore: remove ui module, add uuid dependency
```

---

### Task 2: Core Identity System

**Files:**
- Create: `core/identity.go`
- Create: `core/identity_test.go`

Implements UUID v7 generation and short ID formatting. This is the foundation for all entity identifiers.

**Key design:**
- `NewID()` generates a UUID v7
- `FormatShortID(prefix string, seq int)` returns e.g. `"NODE-42"`
- `ParseShortID(s string)` returns `(prefix, seq, error)`
- `IsValidSlug(s string)` validates repo slug format
- Entity prefix constants: `PrefixRepo`, `PrefixNode`, `PrefixEdge`, etc.
- `EntityPrefixToType` map for resolving short ID prefix to entity type name

- [ ] **Step 1: Write tests for identity functions**

Test cases: UUID v7 generation (time-ordered, valid format), short ID formatting (various prefixes, sequences), short ID parsing (valid, invalid, edge cases), slug validation (valid patterns, too long, invalid chars).

- [ ] **Step 2: Implement identity functions**

- [ ] **Step 3: Run tests**

```bash
go test ./core/ -run TestIdentity -v
```

- [ ] **Step 4: Commit**

```
feat: add UUID v7 identity system with short IDs
```

---

### Task 3: Shared Types and Errors

**Files:**
- Create: `core/shared_types.go`
- Create: `core/version.go`
- Modify: `core/errors.go`

**Key design:**
- `shared_types.go`: All typed string constants from spec §16 — `EventType`, `InputEventType`, `ConnectorType`, `JobType`, `ActionType`, `FilterMode`, `ConditionOp`, `ResourceStatus`, `JobStatus`, `RateLimit` struct, `Metadata` type (key + json.RawMessage value)
- `version.go`: `VersionConflictError` with EntityType, EntityID, Expected, Actual fields
- `errors.go`: Update existing errors to use `uuid.UUID` instead of `xid.ID`. Add `ErrorSlugConflict`, `ErrorSlugRedirect`.

- [ ] **Step 1: Create shared_types.go with all typed constants**

- [ ] **Step 2: Create version.go with VersionConflictError**

- [ ] **Step 3: Update errors.go — replace xid references with uuid**

- [ ] **Step 4: Run tests**

```bash
go test ./core/ -v
```

- [ ] **Step 5: Commit**

```
feat: add shared types, version conflict error, update errors to UUID
```

---

### Task 4: Core Domain Models Rewrite

**Files:**
- Modify: `core/core.go`
- Modify: `core/repo.go`
- Modify: `core/thread.go`
- Modify: `core/annotation.go`

Rewrite all domain models per spec §16 (Internal Architecture). Every model gains: `uuid.UUID` ID, `ShortID string`, `Version int`, `UpdatedAt time.Time`. Node/Edge/Thread/Annotation gain `RepoID uuid.UUID`.

**Key changes from existing code:**
- `Repository`: Was an aggregate with archive + event handling. Becomes a plain domain model (ID, ShortID, Slug, Name, Description, OwnerID, Version, CreatedAt, UpdatedAt). The aggregate behaviour moves to the repo module's service layer.
- `Node`: Gains ShortID, RepoID, Version, UpdatedAt. Metadata becomes `[]Metadata` (slice of key-value pairs with json.RawMessage values).
- `Edge`: Gains ShortID, RepoID, Version, UpdatedAt. Source/Target become `uuid.UUID`.
- `Thread`: Gains own ID, ShortID, RepoID, Version, UpdatedAt. Becomes first-class entity.
- `Annotation`: Gains own ID, ShortID, RepoID, Version, UpdatedAt. Keeps embedded Node + Edge.
- `User`: New model with ID, ShortID, Username, Hostname, DisplayName, Source, CreatedAt, UpdatedAt.

- [ ] **Step 1: Rewrite core/core.go**

Replace all `xid.ID` with `uuid.UUID`. Add new fields per spec. The `Metadata` type changes from `struct{Key string; Value json.RawMessage}` (already exists) — keep the shape but ensure the type is exported and reusable.

- [ ] **Step 2: Rewrite core/repo.go**

Strip out the aggregate logic (HandleInputEvent, publishEvent, archive field). Repository becomes a pure data model. The event handling logic will be rebuilt in the repo module.

- [ ] **Step 3: Update core/thread.go and core/annotation.go**

Add UUID, ShortID, RepoID, Version, UpdatedAt, CreatedBy to both. Thread gets its own ID distinct from its root Node. Annotation gets its own ID distinct from its embedded Node.

- [ ] **Step 4: Verify compilation**

```bash
go build ./core/
```

This will break downstream packages (archive, repo) — that's expected. We fix them in subsequent tasks.

- [ ] **Step 5: Commit**

```
feat: rewrite core domain models with UUID v7, versioning, short IDs
```

---

### Task 5: Events Rewrite

**Files:**
- Modify: `core/events.go`

**Key changes:**
- Split into InputEventType and EventType (output)
- Add all event types from spec §15 and §16
- Update Event struct: add RepoID, use canonical subject format `repo.{repo_id}.events.{EventType}`
- Keep NewEvent factory but update to use uuid.UUID
- Add event payload types for new operations (PatchNode, Thread operations, Annotation operations, Repo operations)

- [ ] **Step 1: Rewrite events.go with new type system**

- [ ] **Step 2: Run core tests**

```bash
go test ./core/ -v
```

- [ ] **Step 3: Commit**

```
feat: rewrite event types with input/output split, new subject format
```

---

### Task 6: Archive Interface Rewrite

**Files:**
- Modify: `core/archive.go`
- Create: `core/list.go`

**Key design:**
- `list.go`: `ListOptions` (Limit, Offset, SortBy, Order), `EdgeQueryOptions`, `AnnotationQueryOptions`, `SearchOptions`, `GraphOptions`, `GraphResult`
- `archive.go`: Completely new interface per spec §16. Methods use `uuid.UUID`. All list methods return `([]T, int, error)` where int is total count. Write methods check Version for optimistic concurrency. `ResolveRef()` for flat URL resolution.

- [ ] **Step 1: Create core/list.go with query/result types**

- [ ] **Step 2: Rewrite core/archive.go with new interface**

Interface contract per spec §16. Include all method groups: Repository, Node, Edge, Thread, Annotation, Graph, Ref resolution.

- [ ] **Step 3: Commit**

```
feat: rewrite Archive interface with pagination, versioning, ref resolution
```

---

### Task 7: API Envelope and HTTP Helpers

**Files:**
- Create: `api/envelope.go`
- Create: `api/http.go`
- Create: `api/middleware.go`
- Create: `api/envelope_test.go`

**Key design:**
- `envelope.go`: `Response` struct with `Data any`, `Error *ErrorBody`, `Meta *Meta`. `ErrorBody` with Code, Message, Details. `Meta` with RequestID, Timestamp, Total, Limit, Offset.
- `http.go`: `WriteJSON(w, status, data)`, `WriteError(w, status, code, message)`, `WriteList(w, data, total, opts)`, `ParsePagination(r)`, `ParseRef(segment)` (determines if UUID, short ID, or slug).
- `middleware.go`: `RequestID` middleware (generates UUID per request), `ETagCheck` middleware (reads If-Match, compares to entity version).

- [ ] **Step 1: Write tests for envelope serialisation and HTTP helpers**

- [ ] **Step 2: Implement envelope types and helpers**

- [ ] **Step 3: Implement middleware (RequestID, ETag)**

- [ ] **Step 4: Run tests**

```bash
go test ./api/ -v
```

- [ ] **Step 5: Commit**

```
feat: add API response envelope, HTTP helpers, and middleware
```

---

### Task 8: SQLite Schema and Sequence System

**Files:**
- Create: `archive/schema.go`
- Create: `archive/sequences.go`
- Create: `archive/sequences_test.go`

**Key design:**
- `schema.go`: All DDL as string constants. Tables: `repositories`, `nodes`, `edges`, `threads`, `thread_edges`, `annotations`, `repo_sequences`, `global_sequences`, `user_sequences`, `repo_slug_redirects`. Includes indices and foreign keys.
- `sequences.go`: `NextRepoSequence(tx, repoID, entity)`, `NextUserSequence(tx, ownerID, entity)`, `NextGlobalSequence(tx, entity)`. Uses atomic `UPDATE ... SET next_seq = next_seq + 1 RETURNING next_seq - 1` pattern. Auto-inserts initial row if not exists.

- [ ] **Step 1: Write schema DDL constants**

Full schema covering all entity tables with UUID primary keys, version columns, updated_at columns, and the three sequence tables + redirect table.

- [ ] **Step 2: Write tests for sequence generation**

Test concurrent sequence generation, auto-initialisation, different scopes.

- [ ] **Step 3: Implement sequence functions**

- [ ] **Step 4: Run tests**

```bash
go test ./archive/ -run TestSequence -v
```

- [ ] **Step 5: Commit**

```
feat: add SQLite schema and short ID sequence system
```

---

### Task 9: Repository Archive Implementation

**Files:**
- Create: `archive/repo_store.go`
- Create: `archive/repo_store_test.go`
- Modify: `archive/archive.go` — update module init to use new schema
- Modify: `archive/local.go` — rewrite to compose store implementations

**Key design:**
- `repo_store.go` implements the Repository subset of the Archive interface:
  - `SaveRepository` — insert (Version==0) or update (Version>0 with optimistic lock)
  - `GetRepository` — by UUID
  - `GetRepositoryBySlug` — by slug + owner
  - `ListRepositories` — with pagination
  - `DeleteRepository` — deletes repo + all child data
  - `SaveSlugRedirect` — records old slug
  - `CheckSlugRedirect` — looks up redirect
  - `ResolveRepoRef` — UUID → slug → short ID → redirect chain
- `local.go` rewritten to compose repo_store + future entity stores. Opens single SQLite db, runs schema init, provides `*sql.DB` to store implementations.

- [ ] **Step 1: Write tests for repository persistence**

Cover: create with auto-generated short ID, get by UUID, get by slug, list with pagination, update with version check (success + conflict), delete, slug rename with redirect, resolve ref (UUID, short ID, slug, redirect).

- [ ] **Step 2: Implement repo_store.go**

- [ ] **Step 3: Update archive.go module and local.go composition**

- [ ] **Step 4: Run tests**

```bash
go test ./archive/ -v
```

- [ ] **Step 5: Commit**

```
feat: implement repository persistence with slugs, versioning, sequences
```

---

### Task 10: App Framework Update — Allow Shared Mount Paths

**Files:**
- Modify: `app/app.go`

The current `AddModuleOnPath` rejects two modules at the same path (line 156–162). The spec requires multiple modules at `/api/v1` (repo, connector, filter, etc.). Since `http.ServeMux` already prevents duplicate method+path registrations, the path-uniqueness check is overly strict. Remove it while keeping all other validations (duplicate module name, root-only-one, reserved paths, path traversal).

- [ ] **Step 1: Update AddModuleOnPath to allow shared mount paths**

Remove the "exact path conflicts" check. Keep all other validations.

- [ ] **Step 2: Update tests in app_test.go / module_path_test.go if any test asserts the old conflict behaviour**

- [ ] **Step 3: Run app tests**

```bash
go test ./app/ -v
```

- [ ] **Step 4: Commit**

```
fix: allow multiple modules at the same mount path
```

---

### Task 11: NATS Module

**Files:**
- Create: `nats/nats.go`
- Create: `nats/publisher.go`
- Create: `nats/streams.go`
- Create: `nats/nats_test.go`
- Delete: `app/nats.go`
- Modify: `app/app.go` — remove NATS imports, receive connection via DI

**Key design:**
- `nats.go`: `Module` struct implementing `app.Module`. `Init()` starts embedded server or connects to external. Exposes `GetConnection()` and `GetPublisher()` for DI into other modules.
- `publisher.go`: `Publisher` struct wrapping `nats.Conn`. `Publish(subject, data)` and `PublishEvent(repoID, event)` which formats the canonical subject `repo.{repo_id}.events.{EventType}`.
- `streams.go`: `CreateRepoStream(repoID)`, `DeleteRepoStream(repoID)`. Creates `REPO_{repo_id}_EVENTS` stream with `repo.{repo_id}.events.>` subject filter.
- `app/app.go`: The `App` no longer starts NATS itself. The nats module is registered like any other module. `App.Publisher` is set after nats module init.

**Important design consideration:** The current `app.Module` interface passes `Publisher` to `HTTPHandlers()` and `MsgHandlers()`. With NATS isolation, the `Publisher` type moves from `app` to the `nats` package. The `app.Module` interface needs to be updated — either `Publisher` becomes an interface in `app` that `nats.Publisher` implements, or modules receive their dependencies via Init/setter methods rather than via the handler methods. The recommended approach is to keep `Publisher` as an interface in `app` (so `app` remains NATS-unaware) and have `nats.Publisher` implement it.

- [ ] **Step 1: Create nats/nats.go with module implementation**

Move logic from app/nats.go. The module's Init() starts the embedded server or connects to external based on config. HTTPHandlers returns empty. MsgHandlers returns empty (the module provides infrastructure, not handlers).

- [ ] **Step 2: Create nats/publisher.go**

- [ ] **Step 3: Create nats/streams.go**

- [ ] **Step 4: Update app/app.go — remove direct NATS management**

Convert `Publisher` from a concrete struct to an interface: `type Publisher interface { Publish(subject string, data []byte) error }`. Remove startEmbeddedNatsServer, connectToEmbeddedNATS, connectToExternalNATS. Remove nats-io imports. The Start() method no longer calls NATS startup — the nats module handles this in Init(). The App receives the publisher via a setter method `SetPublisher(pub Publisher)`.

- [ ] **Step 5: Delete app/nats.go, move tests to nats/nats_test.go**

- [ ] **Step 6: Run all tests**

```bash
go test ./... -v
```

- [ ] **Step 7: Commit**

```
refactor: isolate NATS code into dedicated nats/ module
```

---

### Task 12: Repository HTTP Handlers

**Files:**
- Create: `repo/handlers_repo.go`
- Modify: `repo/repo.go` — update module for /api/v1 mounting, new handler set

**Key design:**
- `handlers_repo.go`: HTTP handlers for repository CRUD:
  - `POST /repos` — create repo (generates UUID, short ID, validates slug)
  - `GET /repos` — list repos (pagination via api.ParsePagination)
  - `GET /repos/{repo_ref}` — get repo (resolve ref: UUID, short ID, slug, redirect → 301)
  - `PUT /repos/{repo_ref}` — full update (slug rename → redirect)
  - `PATCH /repos/{repo_ref}` — partial update
  - `DELETE /repos/{repo_ref}` — async delete (returns 202 + job ID — stubbed until Phase 3)
- All handlers use `api.WriteJSON`, `api.WriteError`, `api.WriteList` from the envelope package.
- Slug redirect: when GET resolves via redirect table, return 301 with Location header pointing to new slug.

- [ ] **Step 1: Write tests for repo handlers**

Use httptest. Cover: create repo (201), list repos (200 with pagination), get by slug (200), get by UUID (200), get by short ID (200), slug redirect (301), update with If-Match (200), update version conflict (412), PATCH partial update (200), delete (202).

- [ ] **Step 2: Implement handlers_repo.go**

- [ ] **Step 3: Update repo/repo.go module**

The module now registers handlers relative to `/api/v1` mount point. Remove old handler registrations for now (node/edge/thread handlers will be rebuilt in Phase 2). Keep the module minimal — just repo CRUD.

- [ ] **Step 4: Run tests**

```bash
go test ./repo/ -v
```

- [ ] **Step 5: Commit**

```
feat: add repository CRUD handlers with slug management
```

---

### Task 13: Main.go, Config, and Integration

**Files:**
- Modify: `main.go`
- Modify: `upspeak.sample.yaml`
- Modify: `app/config.go` — update config structure if needed

**Key design:**
- `main.go`: New module composition per spec §16:
  ```go
  up.AddModuleOnPath(&natsmod.Module{}, "/nats")  // infrastructure, no HTTP handlers
  up.AddModuleOnPath(&archive.Module{}, "/archive") // infrastructure, no HTTP handlers
  up.AddModuleOnPath(&repo.Module{}, "/api/v1")     // repo CRUD
  ```
  Wire dependencies: nats module provides publisher → repo module. Archive module provides archive → repo module.
- Remove all ui references.
- Update `upspeak.sample.yaml` to reflect new module structure.

- [ ] **Step 1: Update main.go with new module wiring**

- [ ] **Step 2: Update upspeak.sample.yaml**

- [ ] **Step 3: Verify full build**

```bash
./build.sh build
```

- [ ] **Step 4: Run all tests**

```bash
go test ./... -v
```

- [ ] **Step 5: Commit**

```
feat: wire new module composition, update config, remove ui references
```

---

### Task 14: Remove xid Dependency

**Files:**
- Modify: `go.mod`

After all code has been migrated to uuid.UUID, remove the xid dependency.

- [ ] **Step 1: Verify no xid imports remain**

```bash
grep -r "rs/xid" --include="*.go" .
```

- [ ] **Step 2: Remove xid dependency**

```bash
go mod tidy
```

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

- [ ] **Step 4: Commit**

```
chore: remove xid dependency, complete UUID v7 migration
```

---

## Phase 2: Knowledge Graph API

**Goal:** Full CRUD for Nodes, Edges, Threads, and Annotations with flat URL routing, batch operations, PATCH semantics, optimistic concurrency, and cascading deletes.

**Depends on:** Phase 1

### File Structure

```
Files to CREATE:
  archive/node_store.go       — Node persistence (CRUD, batch, search, metadata)
  archive/edge_store.go       — Edge persistence (CRUD, batch, query by node)
  archive/thread_store.go     — Thread persistence (CRUD, add/remove nodes)
  archive/annotation_store.go — Annotation persistence (CRUD, selectors)
  archive/resolve.go          — Entity ref resolution (short ID prefix → table lookup)
  repo/handlers_node.go       — Rewrite: Node HTTP handlers with envelope, pagination
  repo/handlers_edge.go       — Rewrite: Edge HTTP handlers with envelope, pagination
  repo/handlers_thread.go     — Rewrite: Thread HTTP handlers with envelope, pagination
  repo/handlers_annotation.go — Annotation HTTP handlers
  repo/handlers_entity.go     — Flat URL entity handler (dispatches by ref type)
  repo/service.go             — Domain service: HandleInputEvent (moved from old core/repo.go)

Files to CREATE (tests):
  archive/node_store_test.go
  archive/edge_store_test.go
  archive/thread_store_test.go
  archive/annotation_store_test.go
  archive/resolve_test.go
  repo/handlers_node_test.go
  repo/handlers_edge_test.go
  repo/handlers_thread_test.go
  repo/handlers_annotation_test.go
  repo/handlers_entity_test.go
  repo/service_test.go
```

### Key Implementation Notes

- **Flat URL routing:** `GET /api/v1/repos/{repo_ref}/{segment}` — the handler first checks reserved path segments (nodes, edges, threads, etc.), then tries short ID parsing, then UUID lookup via `archive.ResolveRef()`.
- **Batch operations:** Atomic via SQLite transactions. `SaveBatchNodes` wraps all inserts in a single transaction; any validation failure rolls back the entire batch.
- **PATCH metadata merge:** New keys added, existing keys updated, `null` value deletes the key. Implemented in the archive layer.
- **Cascading deletes:** NodeDeleted events trigger JetStream consumers that delete related edges and annotations. Implemented as NATS message handlers in the repo module.
- **Optimistic concurrency:** All write handlers read `If-Match` header, pass expected version to archive. Archive returns `VersionConflictError` on mismatch → handler returns 412.

---

## Phase 3: Filters + Jobs

**Goal:** Reusable filter condition sets and async job tracking. Both are prerequisites for connectors, schedules, and rules.

**Depends on:** Phase 1

### File Structure

```
Files to CREATE:
  filter/                     — New module directory
    filter.go                 — Module implementing app.Module
    handlers.go               — Filter CRUD handlers
    engine.go                 — Condition evaluation engine
    engine_test.go
    handlers_test.go

  archive/filter_store.go     — Filter persistence
  archive/filter_store_test.go

  jobs/                       — New module directory
    jobs.go                   — Module implementing app.Module
    handlers.go               — Job list, get, cancel handlers
    runner.go                 — JetStream consumer that picks up and runs jobs
    handlers_test.go

  archive/job_store.go        — Job persistence
  archive/job_store_test.go

  core/filter.go              — Filter, Condition domain models (already in core/shared_types.go partially)
  core/job.go                 — Job domain model
```

### Key Implementation Notes

- **Filter engine:** Evaluates `[]Condition` against an entity. Each condition has a `field` (dot-path like `node.type`), `op` (eq, contains, gt, etc.), and `value`. The engine resolves the field path on the entity, applies the operator, returns bool. `mode: "all"` = AND, `mode: "any"` = OR.
- **Filter test endpoint:** `POST /api/v1/repos/{repo_ref}/{filter_ref}/test` — accepts a sample payload, runs the filter engine, returns per-condition results.
- **Job runner:** JetStream consumer on `JOBS` stream (WorkQueue retention). Picks up pending jobs, executes them, updates status. Job types: collect, publish, sync, webhook — each dispatches to the appropriate module's handler.
- **Job cancellation:** Best-effort. Sets job status to "cancelled"; the runner checks status before each step.

---

## Phase 4: Connectors + Schedules

**Goal:** Source/sink management, one-shot collection, and cron-based scheduling.

**Depends on:** Phase 2 (knowledge graph for content creation), Phase 3 (filters for condition evaluation, jobs for async tracking)

### File Structure

```
Files to CREATE:
  connector/                  — New module directory
    connector.go              — Module implementing app.Module
    handlers_source.go        — Source CRUD + collect trigger
    handlers_sink.go          — Sink CRUD + publish trigger
    handlers_collect.go       — One-shot collect endpoint
    cycle.go                  — Repo connector cycle detection
    ratelimit.go              — Per-source/sink rate limit tracking
    handlers_test.go
    cycle_test.go

  archive/source_store.go     — Source persistence
  archive/sink_store.go       — Sink persistence
  archive/connector_history_store.go — Collection/publish history

  scheduler/                  — New module directory
    scheduler.go              — Module implementing app.Module
    handlers.go               — Schedule CRUD + trigger/pause/resume
    cron.go                   — Cron expression parsing + next-run calculation
    runner.go                 — JetStream consumer for schedule triggers
    handlers_test.go
    cron_test.go

  archive/schedule_store.go   — Schedule persistence

  core/source.go              — Source domain model
  core/sink.go                — Sink domain model
  core/schedule.go            — Schedule domain model
```

### Key Implementation Notes

- **Cycle detection:** When creating/updating a repo source or sink, traverse the dependency graph to check for cycles. `A → B → C → A` is a cycle. Return `409 Conflict` with the chain description.
- **Rate limiting:** Tracked in-memory per source/sink. When limit is hit, status changes to `rate_limited`, scheduled collections are deferred. Reset tracked by window expiry.
- **Connectors:** Phase 4 implements the framework + `webhook` and `repo` connector types. Other connectors (rss, discourse, matrix, etc.) are implemented incrementally later.
- **Schedule runner:** Listens on `SCHEDULES` stream. Cron evaluation runs on a ticker; when a schedule fires, it publishes to `schedules.trigger.{sched_id}` and the runner creates a Job.

---

## Phase 5: Rules + Search

**Goal:** Event-condition-action automation and content discovery.

**Depends on:** Phase 2 (knowledge graph events), Phase 3 (filters for condition evaluation, jobs)

### File Structure

```
Files to CREATE:
  rules/                      — New module directory
    rules.go                  — Module implementing app.Module
    handlers.go               — Rule CRUD + test/pause/resume
    engine.go                 — Rule evaluation engine (JetStream consumer)
    actions.go                — Action executors (enrich, relate, annotate, webhook, etc.)
    handlers_test.go
    engine_test.go

  archive/rule_store.go       — Rule persistence + execution history
  archive/rule_store_test.go

  search/                     — New module directory
    search.go                 — Module implementing app.Module
    handlers.go               — Search, browse, graph endpoints
    indexer.go                — FTS5 index maintenance (JetStream consumer)
    handlers_test.go

  archive/search_store.go     — FTS5 search queries, browse queries, graph traversal
  archive/search_store_test.go

  core/rule.go                — Rule domain model (already partially in shared_types)
```

### Key Implementation Notes

- **Rules engine:** JetStream consumer on `REPO_{repo_id}_EVENTS`. For each event, loads all enabled rules for the repo, evaluates filter conditions, executes matching actions. Actions that fail are logged but don't block other actions.
- **Search:** SQLite FTS5 virtual table for full-text search on node subject + body. Index maintained by a JetStream consumer reacting to Node events. Browse is a simpler query with recency ordering.
- **Graph traversal:** BFS from a starting node up to configurable depth. Returns flat arrays of nodes and edges.

---

## Phase 6: Real-time + Sync

**Goal:** WebSocket event streaming and multi-device synchronisation.

**Depends on:** Phase 2 (knowledge graph events), Phase 3 (jobs for sync operations)

### File Structure

```
Files to CREATE:
  realtime/                   — New module directory
    realtime.go               — Module implementing app.Module
    handlers.go               — WebSocket upgrade handler
    connection.go             — Connection management (ping/pong, limits)
    subscription.go           — Channel subscription + server-side filtering
    handlers_test.go

  sync/                       — New module directory
    sync.go                   — Module implementing app.Module
    handlers.go               — Sync status, trigger, conflict CRUD, peer CRUD
    engine.go                 — Sync engine (incremental event exchange)
    conflict.go               — Conflict detection + resolution strategies
    handlers_test.go

  archive/sync_store.go       — Tombstones, conflict records, peer records
  archive/sync_store_test.go
```

### Key Implementation Notes

- **WebSocket:** Single endpoint `GET /api/v1/ws`. Authenticated during upgrade. Clients send subscribe/unsubscribe messages. Server fans out JetStream events to matching subscriptions with server-side filtering.
- **Connection limits:** Max 10 subscriptions per connection, max 5 connections per user, 1000-message buffer per connection.
- **Sync:** Incremental — exchanges events since last sync cursor. Tombstones track deletions (90-day retention). Version-based conflict detection. Default LWW resolution; user can override per-conflict.
- **Peer management:** Register remote Upspeak instances. Health monitoring with exponential backoff on failures.

---

## Cross-Phase Dependencies Summary

```
Phase 1 (Foundation) ─────────┬──────────────────────────────────────────┐
                               │                                          │
Phase 2 (Knowledge Graph) ─────┤                                          │
                               │                                          │
Phase 3 (Filters + Jobs) ──────┼──── can start after Phase 1              │
                               │                                          │
Phase 4 (Connectors + Sched) ──┤──── needs Phase 2 + Phase 3              │
                               │                                          │
Phase 5 (Rules + Search) ──────┤──── needs Phase 2 + Phase 3              │
                               │                                          │
Phase 6 (Real-time + Sync) ────┘──── needs Phase 2 + Phase 3              │
```

After Phase 1, Phases 2 and 3 can proceed in parallel. After both 2 and 3 are done, Phases 4, 5, and 6 can all proceed in parallel.

---

## Correction Pass: Archive Interface Alignment

**Goal:** Align the Archive interface and storage implementation with the high-level architecture diagram (`assets/high-level-concepts-0.1.png`). The diagram defines Archive as having distinct Local (SQLite + files) and Remote (Postgres + Object Storage) implementations. The Phase 1–2 implementation leaked SQLite-specific details into `core.Archive` and stored node body content in SQLite instead of as files.

**Depends on:** Phase 2 (completed)
**Must complete before:** Phase 3

**Branch:** `fix/archive-interface-cleanup`

### Correction 1: Split `core.Archive` into composable sub-interfaces

The monolithic `core.Archive` interface is split into composable sub-interfaces that both local and remote implementations can satisfy independently:

```go
type RepositoryStore interface { ... }
type NodeStore interface { ... }
type EdgeStore interface { ... }
type ThreadStore interface { ... }
type AnnotationStore interface { ... }
type RefResolver interface { ... }

// Archive composes all sub-interfaces.
type Archive interface {
    RepositoryStore
    NodeStore
    EdgeStore
    ThreadStore
    AnnotationStore
    RefResolver
}
```

**Files:** `core/archive.go`

### Correction 2: Remove sequence methods from `core.Archive`

`NextRepoSequence`, `NextUserSequence`, `NextGlobalSequence` are SQLite-specific implementation details. A remote archive (Postgres) would use database-native sequences internally. These methods are only called within the `archive/` package — no module ever calls them through the interface.

**Action:** Remove from `core.Archive`. Keep as private functions in `archive/sequences.go`. Remove public wrappers from `archive/local.go`.

**Files:** `core/archive.go`, `archive/local.go`, `archive/sequences.go`

### Correction 3: Fix batch and list method signatures

Batch methods should not take a separate `repoID` parameter — each entity already carries `RepoID`. List methods should move filter parameters into typed option structs:

| Before | After |
|---|---|
| `SaveBatchNodes(repoID, nodes)` | `SaveBatchNodes(nodes)` |
| `SaveBatchEdges(repoID, edges)` | `SaveBatchEdges(edges)` |
| `ListNodes(repoID, nodeType, opts)` | `ListNodes(repoID, opts NodeListOptions)` |
| `ListEdges(repoID, source, target, edgeType, opts)` | `ListEdges(repoID, opts EdgeListOptions)` |

New option types added to `core/list.go`:

```go
type NodeListOptions struct {
    Type string // filter by node type; empty means all
    ListOptions
}

type EdgeListOptions struct {
    Source string // filter by source ref; empty means all
    Target string // filter by target ref; empty means all
    Type   string // filter by edge type; empty means all
    ListOptions
}
```

**Files:** `core/list.go`, `core/archive.go`, `archive/local.go`, `archive/node_store.go`, `archive/edge_store.go`, `repo/handlers_node.go`, `repo/handlers_edge.go`

### Correction 4: Implement file-based body storage for nodes

The high-level concepts diagram specifies:
- **Local archive:** "Store Node data as files"
- **Remote archive:** "Store Node data in Object Storage"

Node body content moves from SQLite `body TEXT` column to the filesystem at `{archive_path}/content/{node_id}`. The `body` column is removed from the `nodes` table. The `Node.Body` field in the domain model remains `json.RawMessage` — the archive implementation decides where to persist it.

Storage layout:
```
{archive_path}/
  .upspeak/
    metadata.db           # SQLite — metadata, edges, config
  content/
    {node_id}             # Node body files
```

- `saveNode`: writes body to content file, metadata to SQLite (no body column)
- `getNode`: reads metadata from SQLite, body from content file
- `deleteNode`: removes SQLite row + content file
- `saveBatchNodes`: writes content files + SQLite rows in transaction

Nodes with empty/nil body produce no content file.

**Files:** `archive/schema.go`, `archive/local.go`, `archive/node_store.go`, all `archive/*_test.go`

### Correction 5: Restore high-level concepts diagram in README

The Phase 1 commit removed the "High Level Concepts" section and diagram reference from README.md. Restore the diagram and the original writing style. Do not include project structure (ephemeral). Update content to reflect current plan state.

**Files:** `README.md`

### Correction 6: Phase 5 cross-repo graph note

Update Phase 5 to note that graph traversal must support cross-repository queries, per the high-level concepts diagram which shows Graph as a layer that "queries across repositories".

---

## Phase 5 Addendum: Cross-Repository Graph Queries

The high-level concepts diagram (`assets/high-level-concepts-0.1.png`) shows Graph as a distinct component that "queries across repositories". The graph traversal implementation in Phase 5 must account for this:

- `TraverseGraph` should accept an optional `repoID` filter — when empty, traversal spans all repos accessible to the user
- `SearchNodes` should support cross-repo search with repo filtering
- `GraphResult` already contains nodes and edges from potentially multiple repos (each carries `RepoID`)

This does not require structural changes to the Phase 5 plan — the existing `GraphOptions` type can be extended with a `RepoIDs []uuid.UUID` field when Phase 5 is implemented.

---

## Known Gap: NATS JetStream Implementation vs Spec

The current `nats/` package implements the minimum needed for Phases 1-2. The spec (`docs/specs/api-foundation/18-event-bus-adapter.md`) requires more before Phases 3-6 can proceed. This section documents the gaps.

### Gap 1: Publisher/Subscriber interfaces are too minimal

The `app.Publisher` and `app.Subscriber` interfaces only support basic pub/sub. The spec (doc 18) says "exposes NATS-typed APIs to other modules" and "no abstraction layer", but the implementation creates an abstraction that hides JetStream features.

**What Phases 3-6 need:**
- **JetStream publish** with delivery confirmation (not basic `nats.Publish`)
- **Durable consumers** for rules-engine, job-runner, scheduler, sync-outbound
- **Ack/Nack semantics** for work queues (jobs, schedules)
- **Pull consumers** for job-runner and scheduler (work queue pattern)

**Resolution:** Before Phase 3, expand the `nats/` package to expose JetStream-aware APIs. The `app.Publisher`/`app.Subscriber` interfaces may need to be extended or replaced with richer types that other modules receive via DI. The key constraint is maintaining NATS isolation — only `nats/` imports nats-io.

### Gap 2: Missing consumers.go

The spec (doc 18) defines `nats/consumers.go` for creating named JetStream consumers:
- `rules-engine` — evaluates rules per repo
- `connector-repo` — repo-to-repo subscriptions
- `realtime-ws` — WebSocket fan-out
- `sync-outbound` — sync replication queue
- `job-runner` — async job execution (work queue)
- `scheduler` — cron trigger execution (work queue)

None of these exist. **Required before:** Phase 3 (job-runner), Phase 4 (scheduler, connector-repo), Phase 5 (rules-engine), Phase 6 (realtime-ws, sync-outbound).

### Gap 3: Missing JOBS and SCHEDULES streams

The spec (doc 16) defines three JetStream streams. Only one is implemented:
- `REPO_{repo_id}_EVENTS` — implemented in `nats/streams.go`
- `JOBS` (WorkQueue retention, `jobs.>` subjects) — **not implemented**
- `SCHEDULES` (WorkQueue retention, `schedules.trigger.>` subjects) — **not implemented**

**Required before:** Phase 3 (JOBS stream), Phase 4 (SCHEDULES stream).

### Gap 4: Connection management best practices

The current `nats/nats.go` is missing production-readiness features:
- No reconnection handlers (`nats.MaxReconnects`, `nats.ReconnectWait`)
- No error/disconnect/reconnect handler callbacks for logging
- Uses `Close()` instead of `Drain()` for graceful shutdown
- External connections have no timeout (`nats.ConnectWait`)

**When to fix:** Should be addressed alongside Gap 1, before Phase 3.

### Recommended approach

Address Gaps 1-4 as a **NATS hardening pass** at the start of Phase 3, before implementing the filter/job modules. This keeps NATS work scoped to the `nats/` package and unblocks all downstream phases.

---

## Known Gap: Social Features and Federation

The high-level concepts diagram and spec vision describe Upspeak as "personal-first, federated knowledge infrastructure". The 6-phase plan delivers the personal-first foundation but defers social and federation features. This section documents what exists and what's missing.

### What IS captured in the plan

- **Multi-device sync** (Phase 6): Full sync system for one user across multiple devices — version-based conflict detection, incremental exchange, peer management
- **Repo connector** (Phase 4): One repo can subscribe to another as a source/sink with filters — enables repo-to-repo knowledge pipelines on the same instance
- **Thread publishing** (Phase 2): Thread publish endpoint with visibility parameter (`public|network|private`) and `allow_follow` flag — the API surface exists
- **Sinks for external publishing** (Phase 4): Publish to Fediverse, RSS, email, Matrix, webhooks
- **User model** (core/core.go): `User` struct with `Hostname` field (federation hint), `CreatedBy` on entities

### What is NOT captured — deferred beyond Phase 6

- **Federation protocol**: The `upspeak` connector type is listed in the spec but has zero implementation detail — no config payload, no handshake, no discovery
- **Access control & permissions**: No shared repo ownership, no role-based access, no invite/grant system, no 403 enforcement
- **Social graph features**: No follow-user API, no subscribe-to-stream, no social discovery feed
- **Content discovery across users**: No global search, no trending content, no network-wide feed
- **Visibility enforcement**: The thread publish visibility parameter has no backend semantics — no filtering, no access checks
- **Federated identity**: The `User.Hostname` field exists but has no resolution mechanism

### Impact on current phases

These gaps do NOT block Phases 3-6. The architecture is being built so that federation and social features can be added as **Phase 7+** without breaking existing APIs:
- `OwnerID` on repositories enables future multi-tenant queries
- `CreatedBy` on entities enables future attribution
- Repo connector is the foundation for cross-instance subscriptions
- JetStream event streams can be replicated to peers
- The sync system's peer management is the foundation for federation
