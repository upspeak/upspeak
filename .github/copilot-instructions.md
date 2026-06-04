# Upspeak — Copilot Instructions

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
- **Filter engine**: Reusable condition sets with 15 operators, dot-path field resolution, and AND/OR modes
- **Job system**: Async job tracking via NATS JetStream JOBS stream with durable pull consumer

**Key packages:**
- `app/`: Micro-framework for composing modules, HTTP routing, and application lifecycle
- `core/`: Domain models, Archive sub-interfaces, event types, identity system
- `archive/`: Local archive implementation (SQLite metadata + filesystem body storage)
- `nats/`: NATS JetStream infrastructure — embedded server, publisher, subscriber, stream lifecycle
- `repo/`: Repository CRUD and knowledge graph API module
- `filter/`: Filter condition evaluation engine and filter CRUD module
- `jobs/`: Job tracking, cancellation, and JetStream runner module
- `api/`: Response envelope, HTTP helpers, middleware (ETag, RequestID)

## Critical Rules

1. Follow patterns established in `app/` and `core/` packages
2. Add GoDoc-style comments for all public functions and types
3. Add comments for longer private methods (>20 lines)
4. Write documentation in en-IN (Indian English: "organise", "behaviour", "colour")
5. Make small commits per logical chunk of work, not monolithic batches
6. Never skip error handling — check and handle all errors immediately
7. Never use `panic` for normal error conditions
8. Never create deep nesting — extract functions or use early returns
9. Never add repository directory structure to README — structure is ephemeral
10. Never put NATS imports in any package other than `nats/`

## Git Commit Trailers

When creating git commits, include a `Co-authored-by` trailer with the model name that generated the code. Do not mention "Copilot" — use the model name directly:

```
Co-authored-by: Claude Opus 4.6 <noreply@anthropic.com>
Co-authored-by: Claude Sonnet 4.6 <noreply@anthropic.com>
Co-authored-by: Claude Sonnet 4.5 <noreply@anthropic.com>
Co-authored-by: Claude Haiku 4.5 <noreply@anthropic.com>
```

Use the model that actually produced the code. If multiple models contributed, list each on a separate `Co-authored-by` line.

## Build Commands

```bash
./build.sh build      # Build the binary
./build.sh dev        # Development mode (requires upspeak.yaml)
./build.sh cleanup    # Clean artefacts
go test ./...         # Run tests
```

## Identity System

All entities use UUID v7 as primary key (time-ordered, via `google/uuid`). Each entity also carries a short ID — a human-friendly `{PREFIX}-{SEQ}` identifier (e.g., `NODE-42`, `FILTER-2`, `JOB-109`).

- Short ID sequences are scoped: per-repo (nodes, edges, threads, annotations, sources, sinks), per-user (repos), or global (jobs, schedules, users)
- `core.NewID()` generates a UUID v7
- `core.FormatShortID(prefix, seq)` formats a short ID
- Sequence generation is internal to `archive/` — not exposed through `core.Archive`

## Module Development

All modules implement the `app.Module` interface:

```go
type Module interface {
    Name() string
    Init(config map[string]any) error
    HTTPHandlers() []HTTPHandler
    MsgHandlers() []MsgHandler
}
```

Dependencies (archive, publisher, consumer) are injected via setter methods, not via handler method parameters. All API modules mount at `/api/v1`.

**Lifecycle:** `up.InitModules()` → wire cross-module dependencies → `up.Start()`

## NATS Communication

All NATS code is isolated in `nats/`. Other modules interact via `app.Publisher`, `app.Subscriber`, and `app.Consumer` interfaces.

- **Publisher**: JetStream-backed, delivery confirmed via `js.Publish()`
- **Subscriber**: Core NATS fan-out (push-based)
- **Consumer**: JetStream durable pull subscription with explicit ack

Event subject format: `repo.{repo_id}.events.{EventType}`

## HTTP API Conventions

- **Response envelope**: `{"data": ..., "meta": {...}, "error": {...}}`
- **Flat URL routing**: `/api/v1/repos/{repo_ref}/{entity_ref}` — short ID prefix encodes the type
- **Ref resolution**: `{repo_ref}` accepts UUID, short ID, or slug. Old slugs return 301 redirects
- **Pagination**: `?limit=20&offset=0&sort_by=created_at&order=desc`
- **Optimistic concurrency**: Version field + ETag/If-Match headers. Mismatch returns 412

## Naming Conventions

- **Types**: PascalCase — `Node`, `Edge`, `ErrorNotFound`
- **Functions**: PascalCase exported, camelCase private. Constructor: `New<Type>()`
- **Variables**: Short for common patterns (`err`, `ctx`, `w`, `r`). Single-letter receivers
- **Constants**: Typed constants with semantic grouping (`EventType`, `ConnectorType`)

## Error Handling

Custom error types for domain errors (`ErrorNotFound`, `VersionConflictError`, `ErrorSlugRedirect`). Wrap errors with `fmt.Errorf("context: %w", err)`. Check immediately.

## Testing Standards

- Table-driven tests for multiple cases
- Meaningful test names: `TestSaveNode_VersionConflict`
- Test error cases and edge conditions
- Co-locate test files with implementation
- Use `setupTestArchive(t)` pattern for archive tests

## Common Pitfalls

1. Node body is NOT in SQLite — stored as files at `{archive_path}/content/{node_id}`
2. Sequence methods are private to `archive/` package
3. `HTTPHandlers()` and `MsgHandlers()` take no arguments
4. Always specify HTTP method in route patterns (e.g., `GET /api/nodes`)
5. Never mount modules at `/healthz` or `/readiness` (system endpoints)
6. Only `nats/` imports `github.com/nats-io/*`
7. Short IDs are immutable — sequences never reuse numbers
8. Use `Drain()` not `Close()` on NATS shutdown
9. Before deleting a filter, check references with `GetFilterReferences()`
10. Jobs and schedules use global sequences, not per-repo
