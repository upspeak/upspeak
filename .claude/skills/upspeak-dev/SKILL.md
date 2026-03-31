---
name: upspeak-dev
description: Use when working on the Upspeak codebase — implementing features, fixing bugs, adding modules, or modifying the knowledge graph API. Provides architecture context, domain model reference, and implementation patterns so you can start working without reading dozens of files.
compatibility: Designed for Claude Code. Requires Go 1.25+, SQLite (mattn/go-sqlite3), google/uuid
metadata:
  author: upspeak
  version: "0.4"
---

# Upspeak Development

## Overview

Upspeak is a personal-first, federated knowledge infrastructure. API-first (no bundled UI), hybrid sync core + NATS JetStream, local-first with offline writes.

Read `CLAUDE.md` at the project root for coding conventions, naming, error handling, and testing standards. This skill covers architecture and domain knowledge that CLAUDE.md cannot capture.

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
Archive = RepositoryStore + NodeStore + EdgeStore + ThreadStore + AnnotationStore + RefResolver
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
- `app.Publisher`: `Publish(subject, data) error` — JetStream-backed, delivery confirmed
- `app.Subscriber`: `Subscribe(subject, handler) error` — core NATS fan-out
- `app.Consumer`: `Fetch(maxMsgs, timeout) ([]*Msg, error)` — JetStream pull consumer with `Msg.Ack()`/`Nak()`

Event subjects: `repo.{repo_id}.events.{EventType}` (e.g., `NodeCreated`, `EdgeDeleted`)

**Streams:** `REPO_{repo_id}_EVENTS` (Limits), `JOBS` (WorkQueue, `jobs.>`), `SCHEDULES` (Phase 4).
**Consumers:** Durable pull, AckExplicit. `job-runner` on JOBS. Others added per phase.
**Connection:** Drain() on shutdown, infinite reconnect with jitter, handler callbacks for logging.

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
| 3. Filters + Jobs | Next | Filter CRUD + engine, job tracking, JOBS stream |
| 4. Connectors + Schedules | Planned | Sources, sinks, repo connector, cron |
| 5. Rules + Search | Planned | Rule engine, FTS5, graph traversal (cross-repo) |
| 6. Real-time + Sync | Planned | WebSocket, multi-device sync, conflict resolution |

Full plan: `docs/superpowers/plans/2026-03-30-api-foundation.md`
Full spec: `docs/specs/api-foundation/00-index.md` (18 files)

## Where to Find Things

- **Domain models:** `core/core.go`, `core/repo.go`, `core/thread.go`, `core/annotation.go`
- **Archive interface:** `core/archive.go` (sub-interfaces), `core/list.go` (option types)
- **Local archive:** `archive/local.go` (facade), `archive/node_store.go`, `archive/edge_store.go`, etc.
- **HTTP handlers:** `repo/handlers_repo.go`, `repo/handlers_node.go`, `repo/handlers_entity.go` (flat URL dispatch)
- **API helpers:** `api/envelope.go`, `api/http.go`, `api/middleware.go`
- **Event types:** `core/events.go`, `core/shared_types.go`
- **Identity:** `core/identity.go` (NewID, FormatShortID, ParseShortID, prefixes)
- **Schema:** `archive/schema.go` (SQLite DDL)
- **NATS bus:** `nats/nats.go` (Bus, connection), `nats/publisher.go`, `nats/subscriber.go`
- **NATS streams:** `nats/streams.go` (repo events, JOBS)
- **NATS consumers:** `nats/consumers.go` (manager, definitions), `nats/consumer.go` (app.Consumer impl)
- **High-level diagram:** `assets/high-level-concepts-0.1.png`
