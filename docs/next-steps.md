# Upspeak — API Completion Status & Next Steps

**Last updated:** 2026-06-06

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

### 2. Ingestion track — connector adapterisation

This cross-cutting track replaces the original "one backend per job type" design with a
first-class **adapter + registry + pipeline** model (ADR-0013, ADR-0014, ADR-0015).
Each integration lives in `integrations/<name>/`, imports only `core`, and is wired by
`main.go` via DI — breaking the `connector ↔ jobs` import cycle.

| Sub-project | Status | Scope |
|-------------|--------|-------|
| A1 — Foundation | **Shipped** | `Connection` entity + store, `core.Adapter` contract, ingest types (`Item`, `Cursor`, `IngestCursor`), `GetNodeBySourceExternalID`, node provenance, `secrets.SecretCipher` port + AES-256-GCM implementation |
| A2 — Registry + pipeline + webhook adapter | **Shipped** | `app.AdapterRegistry` lookup interface; `ingest.Registry` + Item-path `Pipeline` (map → filter → persist → dedup → events); `integrations/webhook` connection-less `Collector`; `jobs/runner.go` `executeWebhook` dispatches via registry+pipeline; runner gains DI of `app.Publisher` + `app.AdapterRegistry`, eliminating the `jobs ↔ connector` import cycle; webhook URLs redacted in logs, persisted error strings, and the job result |
| A2b — repo→repo adapter | **Pending** | Net-new internal data movement; the adapter reads from a source repository's archive (a documented internal exception — adapters otherwise never touch the archive); must interact with `connector/cycle.go` to prevent self-loops; split out of A2 deliberately |
| A3 — Cipher + connection enforcement | **Pending** | Wire `secrets.NewCipherFromEnv()` / `LocalArchive.SetSecretCipher` at runtime; enforce connector↔connection type match on source/sink create/update; return 409 when deleting an in-use connection; credential update is load-modify-save |
| B — Full collect/publish wiring | **Pending** | Wire `executeCollect`/`executePublish` through registry+pipeline (cursors via `GetIngestCursor`/`SaveIngestCursor`, filter-on-normalised, dedup); emit `CollectionCompleted`/`PublishCompleted`; extend pipeline beyond Items: Threads (need thread provenance — not in the A1 schema), Annotations, Tombstones, reply Edges, author→User resolution (needs a user store) |

**Watch-outs for Sub-project A3:** All items in the A3 row above are carry-forwards
from A1's review — none of A1 or A2 wired them. Specifically: `LocalArchive.SetSecretCipher`
exists but is never called at startup; `isSupportedConnector` does not yet validate that
the source's `connection_id` references a connection whose type matches the connector type.

**Watch-outs for Sub-project B:** `executeCollect` and `executePublish` in
`jobs/runner.go` still record success without doing real work. They need the full
registry+pipeline dispatch path that `executeWebhook` now uses, plus cursor persistence
and `CollectionCompleted`/`PublishCompleted` event emission. The pipeline currently only
handles `Item`; Threads will require provenance tracking (not yet in the A1 archive
schema), and author→User resolution requires a user store.

### 3. Connector backends

Phase 4 built the connector framework plus the `webhook` and `repo` connector types.
The remaining declared `ConnectorType`s are deferred to "incrementally later" by the
plan and still need adapter implementations: **rss, discourse, matrix, fediverse, email,
webpage, upspeak**. Until implemented, `connector/handlers_source.go`
`isSupportedConnector()` rejects them. The `repo→repo` adapter (Sub-project A2b above)
is the next planned addition.

### 4. Job execution backends

`jobs/runner.go` wires the full job lifecycle. Since A2, `executeWebhook` dispatches
via the adapter registry and ingest pipeline. The remaining executors are still
placeholders that record success without doing real work:

- `executeCollect` — must dispatch to the connector backend for the source's type (Sub-project B).
- `executePublish` — must dispatch to the sink's connector backend (Sub-project B).
- `executeSync` — returns `{"status":"not_implemented"}`; awaits Phase 6b.

These unblock once the connector adapters (item 3) and sync (item 1) exist.

### 5. Operational events

Publish `EventCollectionCompleted` and `EventPublishCompleted` from job completion.
They are declared in `core` but emitted nowhere, so rules and realtime clients cannot
react to collection/publish outcomes yet. These are wired as part of Sub-project B
above (see §2, Watch-outs for Sub-project B).

### 6. Endpoint gaps left stubbed by shipped phases

- **Async repo delete** — `repo/handlers_repo.go` still deletes synchronously and
  returns `204`. The design calls for creating a delete job and returning `202 Accepted`
  with a job ID (the code comment names "Phase 3", which has since shipped).
- **Thread publish** — `POST /repos/{repo_ref}/{thread_ref}/publish`
  (`repo/handlers_entity.go`) returns `501 Not Implemented`.

### 7. Realtime stub-channel backings

The realtime module accepts all six spec channels; three never emit because their
matcher falls through to `false` in `realtime/match.go`:

- `repos.{repo}.rules.{rule}.actions` — wire `channelRuleActions` to the
  **already-published** `core.EventRuleTriggered` (smallest remaining gap).
- `jobs.{job}` — needs a new `JobUpdated` event published on job state changes.
- `sync` — needs the Phase 6b events (item 1).

### 8. Search source filtering

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
| `realtime/match.go` (`matchChannel` default) | `channelRuleActions` / `channelJob` / `channelSync` never match | Wire to backing events | §7 / `JobUpdated` / Phase 6b |
| `jobs/runner.go` `executeCollect` | Records success without fetching | Dispatch via registry+pipeline; cursors, filter-on-normalised, dedup | Sub-project B |
| `jobs/runner.go` `executePublish` | Records success without publishing | Dispatch via registry+pipeline to sink adapter | Sub-project B |
| `jobs/runner.go` `executeSync` | Returns `not_implemented` | Real sync run | Phase 6b |
| `connector/handlers_source.go` `isSupportedConnector` | Only `webhook` + `repo` accepted | Allow each type as its adapter lands | §3 |
| `repo/handlers_repo.go` delete handler | Synchronous delete, `204` | Async delete job, `202` | — |
| `repo/handlers_entity.go` thread publish | `501 Not Implemented` | Thread publication logic | §3 (sink adapters) |
| `archive/search_store.go` browse | `SourceID` filter disabled | Node→source tracking | §8 |
| `core/shared_types.go` | `EventCollectionCompleted` / `EventPublishCompleted` declared, never published | Emit on job completion | Sub-project B |
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
