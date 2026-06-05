---
name: upspeak-dev
description: Use when working on the Upspeak codebase — implementing features, fixing bugs, adding modules, or modifying the knowledge graph API. Provides architecture context, domain model reference, and implementation patterns so you can start working without reading dozens of files.
compatibility: Designed for Claude Code. Requires Go 1.25+, SQLite (mattn/go-sqlite3), google/uuid
metadata:
  author: upspeak
  version: "0.7"
---

# Upspeak Development

## Overview

Upspeak is a personal-first, federated knowledge infrastructure. API-first (no bundled UI), hybrid sync core + NATS JetStream, local-first with offline writes.

Read `CLAUDE.md` at the project root for coding conventions, naming, error handling, and testing standards. This skill covers architecture and domain knowledge that CLAUDE.md cannot capture.

## Companion Skills

When working in this repo, invoke these auto-discovered skills (via the `Skill` tool) at the right moments — they live alongside this one under `.claude/skills/`:

- **`go-style`** — idiomatic Go practices (naming, error handling, interfaces, concurrency, HTTP/JSON, testing). Consult when writing or reviewing any `*.go` file. Project conventions in `CLAUDE.md` and the patterns in `app/`/`core/` take precedence where they conflict (notably: en-IN comments, package-level `slog`, root-level module layout — no `cmd/`/`pkg/`/`internal/`).
- **`create-architectural-decision-record`** — generate a structured ADR in `docs/adr/adr-NNNN-*.md` when making a non-trivial architecture or technology decision (e.g. the global-vs-per-repo `REPO_EVENTS` choice, the typed-self-contained-URL divergence). Capture context, decision, consequences, and rejected alternatives.
- **`documentation-writer`** — a Diátaxis-framework technical writer for tutorials, how-to guides, reference, and explanation docs. Use when authoring or restructuring anything under `docs/` (write in en-IN; prefer linking to source over duplicating structure that drifts).

## Architecture

```
HTTP API (/api/v1)
  → repo module (handlers)
    → core.Archive interface (synchronous write)
      → LocalArchive (SQLite metadata + filesystem body)
    → app.Publisher interface (async event)
      → nats.publisher (JetStream)
```

**Write path:** Synchronous to archive, confirmed to client. JetStream carries consequences.

**Storage split (local archive):**
- SQLite (`{path}/.upspeak/metadata.db`): all metadata, edges, threads, annotations, sequences
- Files (`{path}/content/{node_id}`): node body content
- A remote archive would use Postgres + object storage instead

## Domain Models

All entities: UUID v7 primary key, short ID (`NODE-42`), version (optimistic concurrency), created_at, updated_at.

| Entity | Key fields | Notes |
|--------|-----------|-------|
| **Node** | type, subject, content_type, body, metadata, repo_id, created_by | Body stored as file, not in SQLite |
| **Edge** | type, source, target, label, weight, repo_id | Links two nodes |
| **Thread** | node (root), edges, metadata, repo_id | Composite: owns a root Node + Edges |
| **Annotation** | node, edge, motivation, repo_id | Composite: owns a Node + Edge linking to target |
| **Repository** | slug, name, description, owner_id | Slug is renameable (old slugs redirect) |

## core.Archive Sub-interfaces

```
Archive = RepositoryStore + NodeStore + EdgeStore + ThreadStore + AnnotationStore + FilterStore + JobStore + SourceStore + SinkStore + ConnectorHistoryStore + ScheduleStore + RuleStore + SearchStore + RefResolver
```

Modules that only need node operations can accept `core.NodeStore` instead of the full `core.Archive`. Sequence methods (`nextRepoSequence` etc.) are package-private in `archive/` — never on the interface.

## Key Patterns

**Flat URL routing:** `/api/v1/repos/{repo_ref}/{entity_ref}` — short ID prefix encodes type. Reserved segments (`nodes`, `edges`, `threads`, `annotations`) checked first.

**Ref resolution:** `{repo_ref}` accepts UUID, short ID, or slug. `{entity_ref}` accepts UUID or short ID. Old slugs return 301.

**Optimistic concurrency:** Version field + ETag/If-Match headers. Archive returns `VersionConflictError` on mismatch, handler returns 412.

**Metadata merge (PATCH):** New keys added, existing updated, `null` value deletes. See `mergeMetadata()` in `repo/handlers_node.go`.

**Batch operations:** `SaveBatchNodes(nodes)` / `SaveBatchEdges(edges)` — no separate repoID param. Each entity carries its own RepoID. Atomic via SQLite transaction; body files written after commit.

**List options:** `NodeListOptions{Type, ListOptions}`, `EdgeListOptions{Source, Target, Type, ListOptions}`. Filter params in typed structs, not positional args.

**Response envelope:** `{"data": ..., "meta": {...}, "error": {...}}` via `api.WriteJSON`, `api.WriteList`, `api.WriteError`.

## NATS Isolation

Only `nats/` imports `github.com/nats-io/*`. Other packages use interfaces from `app/`:
- `app.Publisher`: `Publish(subject, data) error` **and** `PublishEvent(eventType, repoID, payload) error` — the latter builds the `core.Event` envelope (`NewEvent` + `Subject`) and publishes it, so modules never marshal events or hand-build subjects themselves. JetStream-backed, delivery confirmed.
- `app.Subscriber`: `Subscribe(subject, handler) error` — core NATS fan-out (used by realtime for `repo.*.events.>`)
- `app.Consumer`: `Fetch(maxMsgs, timeout) ([]*Msg, error)` — JetStream pull consumer with `Msg.Ack()`/`Nak()`/`InProgress()`/`Term()`

Subject construction lives in `core`, never inline: `Event.Subject()` → `repo.{repo_id}.events.{EventType}`; `core.JobSubject`, `core.ScheduleTriggerSubject` for the work queues.

**Streams (all global, created in `main.go`):** `REPO_EVENTS` (Limits, `repo.*.events.>` — one stream serves every repo; `nats/streams.go`), `JOBS` (WorkQueue, `jobs.>`), `SCHEDULES` (WorkQueue, `schedules.trigger.>`). The per-repo `CreateRepoStream` is **tests-only** and superseded — never wire it (it overlaps `REPO_EVENTS`).
**Consumers:** Durable pull, AckExplicit, MaxDeliver=5, AckWait=30s — `job-runner` (JOBS), `schedule-runner` (SCHEDULES), `rules-engine` (REPO_EVENTS).
**Event hops:** `core.Event.Hops` bounds rule-reaction cascades; the rules engine drops events past `maxRuleHops` and stamps `Hops+1` on events it emits.
**Connection:** Drain() on shutdown, infinite reconnect with jitter, handler callbacks for logging.

**Background loops** implement `app.Runner` (`Run(ctx)`) and are started uniformly from `main` via a `[]app.Runner` (jobs runner, scheduler runner, rules engine, realtime hub).

**Logging:** one handler is set via `slog.SetDefault` in `main`; every package calls package-level `slog.Info/Error/…` directly. Never construct a logger or hold a `*slog.Logger` field/param.

## Module Wiring

```go
// main.go pattern:
bus, err := usnats.Start(config.Name, natsConfig)
// handle err...
up := app.New(*config)
up.SetSubscriber(bus.Subscriber())
repoModule.SetPublisher(bus.Publisher())
up.AddModuleOnPath(repoModule, "/api/v1")
up.InitModules()  // Init + register handlers
repoModule.SetArchive(archiveModule.GetArchive())  // Wire after Init
up.Start()  // Start HTTP
```

Dependencies injected via setters, not constructor or handler params. `HTTPHandlers()` and `MsgHandlers()` take no arguments. Always call `InitModules()` before wiring cross-module dependencies, then `Start()` for HTTP.

## Implementation Status

| Phase | Status | Scope |
|-------|--------|-------|
| 1. Foundation | Done | UUID v7, NATS isolation, repo CRUD, API envelope |
| 2. Knowledge Graph | Done | Nodes, edges, threads, annotations, flat URLs |
| Correction Pass | Done | Archive sub-interfaces, file-based body, signature cleanup |
| NATS Hardening | Done | JetStream publish, consumers, JOBS stream, connection management |
| 3. Filters + Jobs | Done | Filter CRUD + engine, job tracking, NATS job runner |
| 4. Connectors + Schedules | Done | Sources, sinks, rate limiting, cycle detection, cron, job execution + history |
| 5. Rules + Search | Done | Rule engine on global `REPO_EVENTS`, FTS5, graph traversal (cross-repo) |
| 6a. Real-time | Done | WebSocket `/api/v1/ws`, Hub fan-out, server-side filtering, allow-all auth seam |
| 6b. Sync | Deferred | Multi-device sync, tombstones, conflict resolution — not started |

> **Heads-up:** job executors, several connector types, async repo delete, thread publish, and three realtime channels are still **stubs**. Read `docs/next-steps.md` before assuming a path is fully implemented — it lists every stub and the remaining work. Social/federation is Phase 7+.

Status, remaining work, and the in-code stub inventory: `docs/next-steps.md`
Defining data-flow architecture: `assets/high-level-concepts-0.1.png`

## Where to Find Things

- **Domain models:** `core/core.go`, `core/repo.go`, `core/thread.go`, `core/annotation.go`, `core/filter.go`, `core/job.go`, `core/schedule.go`
- **Archive interface:** `core/archive.go` (sub-interfaces), `core/list.go` (option types)
- **Local archive:** `archive/local.go` (facade), `archive/node_store.go`, `archive/edge_store.go`, `archive/filter_store.go`, `archive/job_store.go`, `archive/source_store.go`, `archive/sink_store.go`, `archive/schedule_store.go`, `archive/connector_history_store.go`
- **HTTP handlers:** `repo/handlers_repo.go`, `repo/handlers_node.go`, `repo/handlers_entity.go` (flat URL dispatch)
- **Filter module:** `filter/filter.go` (module + handlers), `filter/engine.go` (condition evaluation)
- **Jobs module:** `jobs/jobs.go` (module + handlers + CreateJob helper), `jobs/runner.go` (JetStream consumer + execute handlers)
- **Connector module:** `connector/connector.go` (module), `connector/handlers_source.go`, `connector/handlers_sink.go`, `connector/handlers_collect.go`, `connector/ratelimit.go`, `connector/cycle.go`
- **Scheduler module:** `scheduler/scheduler.go` (module), `scheduler/handlers.go`, `scheduler/runner.go` (tick + consume loops), `scheduler/cron.go` (parser)
- **Rules module:** `rules/rules.go` (module + CRUD/pause/resume/history), `rules/engine.go` (`REPO_EVENTS` consumer, trigger evaluation via filter engine, action dispatch)
- **Search module:** `search/search.go` (module), search/browse/traverse handlers; FTS5 lives in `archive/search_store.go` (build tag `sqlite_fts5`)
- **Realtime module:** `realtime/realtime.go` (module + `app.Runner` hub), `realtime/hub.go` (dispatch), `realtime/connection.go`, `realtime/subscription.go` + `realtime/match.go` (channels + filtering), `realtime/auth.go` (allow-all `Authenticator` seam)
- **API helpers:** `api/envelope.go`, `api/http.go`, `api/middleware.go`
- **Event types:** `core/events.go`, `core/shared_types.go`
- **Identity:** `core/identity.go` (NewID, FormatShortID, ParseShortID, prefixes)
- **Schema:** `archive/schema.go` (SQLite DDL)
- **NATS bus:** `nats/nats.go` (Bus, connection), `nats/publisher.go`, `nats/subscriber.go`
- **NATS streams:** `nats/streams.go` (repo events, JOBS, SCHEDULES)
- **NATS consumers:** `nats/consumers.go` (manager, definitions), `nats/consumer.go` (app.Consumer impl)
- **High-level diagram:** `assets/high-level-concepts-0.1.png`
