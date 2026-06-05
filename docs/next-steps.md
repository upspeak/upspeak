# Upspeak — API Completion Status & Next Steps

**Last updated:** 2026-06-05

This is the single source of planning truth for finishing Upspeak's initial API. It
replaces the earlier `docs/specs/` and `docs/superpowers/` planning artefacts (which had
drifted from the shipped code). Two things outrank this document:

1. **The shipped code** under `app/`, `core/`, and the module packages — the source of
   truth for any API contract or behaviour.
2. **`assets/high-level-concepts-0.1.png`** — the defining data-flow architecture
   (Repository ↔ local/remote Archives; Nodes/Edges/Threads/Annotations; Filters/Rules;
   Connectors). Everything below builds toward that diagram.

Nothing here proposes new features; it only tracks what the original plan already scoped
and where the code currently stubs functionality.

---

## Implementation status

| Phase | Status | Scope |
|-------|--------|-------|
| 1. Foundation | **Shipped** | UUID v7, NATS isolation, repo CRUD, API envelope |
| 2. Knowledge Graph | **Shipped** | Nodes, edges, threads, annotations, flat URL routing, optimistic concurrency |
| 3. Filters + Jobs | **Shipped** | Filter CRUD + condition engine, job tracking, `JOBS` runner |
| 4. Connectors + Schedules | **Shipped** | Sources, sinks, rate limiting, cycle detection, cron scheduling, history |
| 5. Rules + Search | **Shipped** | Rules engine on global `REPO_EVENTS`, FTS5 search, recursive-CTE graph traversal |
| 6a. Real-time | **Shipped** | WebSocket `GET /api/v1/ws`, Hub fan-out, server-side filtering, allow-all auth seam |
| 6b. Sync | **Not started** | Multi-device sync — see "Remaining work" below |

Write path (as built): HTTP handler → `core.Archive` (synchronous write, confirmed to
client) → `app.Publisher.PublishEvent(...)` (async domain event on `REPO_EVENTS`).

---

## Remaining work to initial API completion

Ordered roughly by dependency. Each item names the files the original plan targets.

### 1. Phase 6b — Multi-device sync (the only unbuilt phase)

The plan scopes a full single-user, multi-device sync system. Nothing exists yet.

- **Create `sync/` module:**
  - `sync.go` — module implementing `app.Module`
  - `handlers.go` — sync status, trigger, conflict CRUD, peer CRUD
  - `engine.go` — incremental event exchange since the last sync cursor
  - `conflict.go` — conflict detection + resolution strategies
- **Create `archive/sync_store.go`** — tombstones, conflict records, peer records
  (plus `core` types and an `Archive` sub-interface following the existing pattern).
- **Behaviour:** incremental exchange (events since last cursor); tombstones with
  90-day retention; version-based conflict detection; default Last-Write-Wins with
  per-conflict override; peer registration + health monitoring with exponential backoff.
- **Emit** `EventSyncCompleted` and `EventConflictDetected` (declared in
  `core/shared_types.go`, currently never published).

### 2. Connector backends

Phase 4 built the connector framework plus the `webhook` and `repo` connector types.
The remaining declared `ConnectorType`s are deferred to "incrementally later" by the
plan and still need execution backends: **rss, discourse, matrix, fediverse, email,
webpage, upspeak**. Until implemented, `connector/handlers_source.go`
`isSupportedConnector()` rejects them.

### 3. Job execution backends

`jobs/runner.go` wires the full job lifecycle but the executors are placeholders that
record success without doing real work:

- `executeCollect` — must dispatch to the connector backend for the source's type.
- `executePublish` — must dispatch to the sink's connector backend.
- `executeWebhook` — must fetch the URL, parse content, and create nodes.
- `executeSync` — returns `{"status":"not_implemented"}`; awaits Phase 6b.

These unblock once the connector backends (item 2) and sync (item 1) exist.

### 4. Operational events

Publish `EventCollectionCompleted` and `EventPublishCompleted` from job completion.
They are declared in `core` but emitted nowhere, so rules and realtime clients cannot
react to collection/publish outcomes yet.

### 5. Endpoint gaps left stubbed by shipped phases

- **Async repo delete** — `repo/handlers_repo.go` still deletes synchronously and
  returns `204`. The design calls for creating a delete job and returning `202 Accepted`
  with a job ID (the code comment names "Phase 3", which has since shipped).
- **Thread publish** — `POST /repos/{repo_ref}/{thread_ref}/publish`
  (`repo/handlers_entity.go`) returns `501 Not Implemented`.

### 6. Realtime stub-channel backings

The realtime module accepts all six spec channels; three never emit because their
matcher falls through to `false` in `realtime/match.go`:

- `repos.{repo}.rules.{rule}.actions` — wire `channelRuleActions` to the
  **already-published** `core.EventRuleTriggered` (smallest remaining gap).
- `jobs.{job}` — needs a new `JobUpdated` event published on job state changes.
- `sync` — needs the Phase 6b events (item 1).

### 7. Search source filtering

`archive/search_store.go` disables the browse `SourceID` filter because nodes have no
source attribution. Requires a node→source tracking mechanism (e.g. a `Node.source_id`
field) before the filter can be implemented.

---

## Stub inventory

Where placeholders live today, so future work can find and replace them. Code is the
source of truth — line numbers drift, so search by symbol if they don't match.

| Location | What is stubbed | Replace with | Depends on |
|----------|-----------------|--------------|------------|
| `realtime/auth.go` | `allowAllAuthenticator` permits every WS upgrade (identity `"local"`) | Real token/identity verification | Auth (deferred, Phase 7+) |
| `realtime/match.go` (`matchChannel` default) | `channelRuleActions` / `channelJob` / `channelSync` never match | Wire to backing events | §6 / `JobUpdated` / Phase 6b |
| `jobs/runner.go` `executeCollect` | Records success without fetching | Dispatch to connector backend | §2 |
| `jobs/runner.go` `executePublish` | Records success without publishing | Dispatch to sink backend | §2 |
| `jobs/runner.go` `executeWebhook` | Records completion without fetching/parsing | Fetch URL → parse → create nodes | — |
| `jobs/runner.go` `executeSync` | Returns `not_implemented` | Real sync run | Phase 6b |
| `connector/handlers_source.go` `isSupportedConnector` | Only `webhook` + `repo` accepted | Allow each type as its backend lands | §2 |
| `repo/handlers_repo.go` delete handler | Synchronous delete, `204` | Async delete job, `202` | — |
| `repo/handlers_entity.go` thread publish | `501 Not Implemented` | Thread publication logic | §2 (sinks) |
| `archive/search_store.go` browse | `SourceID` filter disabled | Node→source tracking | §7 |
| `core/shared_types.go` | `EventCollectionCompleted` / `EventPublishCompleted` declared, never published | Emit on job completion | §4 |
| `scheduler/handlers.go` `defaultOwnerID` | Fixed owner UUID until auth exists | Per-user ownership | Auth (deferred) |
| `archive/local.go` | FTS5 graceful-degrade without `sqlite_fts5` build tag | Ensure prod builds set the tag | build config |

---

## Explicitly out of scope (deferred beyond the initial API)

Per the original plan's "Known Gap: Social Features and Federation", the following are
**Phase 7+** and deliberately excluded from initial API completion:

- Social knowledge sharing (publishing curated threads/collections, follow/pull).
- Cross-user / cross-instance federation beyond single-user multi-device sync.
- Real authentication and multi-tenancy.

The seams already exist for these and should not be removed: `OwnerID` on repositories,
`CreatedBy` on entities, `scheduler/handlers.go` `defaultOwnerID`, and the realtime
`Authenticator` interface (allow-all today). They make federation/auth additive later
without breaking the current APIs.
