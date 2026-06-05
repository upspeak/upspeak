# Design: Sub-project A2b — repo→repo over NATS (Source/Sink rails + pipeline completion)

**Date:** 2026-06-06
**Authors:** Kaustav Das Modak, Claude
**Status:** Approved (design); implementation pending
**Builds on:** A2 (adapter registry + ingest pipeline, shipped). See
`docs/design/2026-06-05-connector-adapterisation.md` (the track design) and
`docs/design/plans/2026-06-05-subproject-a2-registry-pipeline.md`.

This spec covers **A2b**: moving a knowledge graph between two Upspeak repositories
entirely over NATS, with each repository controlling **both** ends of the flow — what it
publishes (its **Sink**) and what it ingests (its **Source**). repo→repo is the identity
(zero-transformation, no-network, no-credential) connector, which makes it the ideal
proving ground for two pieces of shared infrastructure that every later connector reuses:

1. The **Source/Sink supervisor rails** and the curated outbound **`SINK_EVENTS`** bus.
2. The **completion of the ingest pipeline** beyond Items — Threads, Edges, and
   Annotations with cross-repo provenance.

Where this design and the shipped code disagree, the code wins and this document is
updated.

---

## 1. Context and the corrected model

The original track design (`…connector-adapterisation.md` §8, Sub-project A) said "migrate
webhook **+ repo** onto adapters". A2 migrated webhook. A2b was expected to be a
"repo→repo adapter" needing **archive-read access** — a documented exception to the
hexagonal rule that "adapters never touch the archive" (§3.5).

That premise is now **retired**. Upspeak's spine is "Hybrid sync core + NATS JetStream":
every repository write already publishes a domain event to `REPO_EVENTS`
(`repo.{repo_id}.events.{EventType}`), and those event payloads carry the **full entity**
— `EventNodeCreatePayload{Node}` includes `Node.Body` (`json:"body"`). So a subscriber
needs **nothing** from the producing repo's archive: the data travels as NATS events.
repo→repo becomes a **streaming** flow, not a pull, and the archive-read exception simply
**does not exist**.

### The Source / Sink / Archive model

- **Source** — a repository's **inbound** configuration: what it ingests. Many per repo.
  Filtered on the way in (what the repo *accepts*).
- **Archive** — where the repository's data lands (SQLite + files locally).
- **Sink** — a repository's **outbound** configuration: what it publishes. This is the
  repository's **publication control** — *each repo decides what leaves it.*
- **repo→repo** — repo B's **Source** subscribes to repo A's **Sink**. B receives only
  what A has chosen to publish, never A's internal stream.

This maps level-for-level onto `assets/high-level-concepts-0.1.png`, which has a **filter
diamond on both sides**: `Import → filter → populate` (the Source) and
`Graph → filter → Export/Publish` (the Sink).

### Goals

1. Move nodes **and their edges, threads, and annotations** between repos over NATS, with
   provenance, idempotent re-delivery, and both-sided filtering.
2. Build the **reusable Source/Sink rails** (two supervisors + the `SINK_EVENTS` bus) that
   external adapters (B/C) plug into without change.
3. **Complete the ingest pipeline** beyond Items, proven against the identity connector so
   the hard reference-resolution work is done without external-parsing noise.
4. Keep **all data modelling in `core`**; keep NATS isolated in `nats/`.

### Non-goals (this slice)

- **Deletes / tombstones / retraction.** A node edited to *stop* matching a Sink filter
  stays stale in subscribers. Tombstones land with the pipeline's delete path (B).
- **External adapters** — RSS/Discourse/Matrix, outbound-webhook delivery, inbound-webhook
  endpoint (B/C).
- **Author → User preservation** — needs the user store; ingested entities are attributed
  to the destination repo owner for now (§6).
- **Selective rule-driven repo-publish** ("send this one node to repo X") — a different
  mechanism (rules action), a later slice.
- **Cursors and `CollectionCompleted`/`PublishCompleted`** — those belong to the
  job/poll path (B); the repo flow is event-driven and has no JOB lifecycle.

---

## 2. Architecture and data flow

Two hops over NATS, with publication control enforced on the producing side:

```
repoA: write node X (HTTP, ingest, rule, …)
  └─> repo.A.events.NodeCreated                      REPO_EVENTS  (internal, existing)
        │
   [Publish supervisor]   one durable consumer on REPO_EVENTS (group "sink-publisher")
        │  ListSinks(A); for each enabled Sink, evaluate Sink.FilterIDs against the
        │  entity's node(s); republish the matches
        ▼
      sink.{sinkA}.events.NodeCreated                SINK_EVENTS  (curated outbound, new)
        │
   [Ingest supervisor]    one durable consumer on SINK_EVENTS (group "repo-ingest",
        │                 subject sink.*.events.>)
        │  ListRepoSourcesForSink(sinkA); for each subscribing Source, build a
        │  single-entity IngestBatch and …
        ▼
   pipeline.Ingest(ctx{RepoID: B, Source: srcB}, batch)
        │  apply Source.FilterIDs · resolve refs via provenance · dedup · persist · emit
        ▼
   repoB: node X' created  ──> repo.B.events.NodeCreated  (REPO_EVENTS)
                                 └─> feeds B's rules, realtime, search, AND B's own Sinks
```

Reusing `pipeline.Ingest` (the sole writer) means the instant an entity lands in repo B it
emits B's *own* `REPO_EVENTS`, so B's rules, realtime, FTS index, **and B's Sinks** light
up with no extra wiring. repo→repo therefore composes into chains (A→B→C) for free.

### 2.1 Two supervisors (both `app.Runner`, started from `main`)

Both are **single global durable consumers** doing per-event lookup — the established
`REPO_EVENTS`/`rules-engine` pattern — so there is **no per-source/per-sink consumer
lifecycle to manage**; fan-out (many Sources on one Sink) falls out of the lookup.

- **Publish supervisor** — lives in `connector/` (it owns Sinks). Durable pull consumer on
  `REPO_EVENTS`. Enforces publication control (§3).
- **Ingest supervisor** — lives in `ingest/` (beside the pipeline; matches track-design §7
  placement of the stream supervisor). Durable pull consumer on `SINK_EVENTS`; dispatches
  to `pipeline.Ingest`.

Both consumers use explicit ack with `MaxDeliver=5`, `AckWait=30s` (consistent with
`job-runner`, `schedule-runner`, `rules-engine`).

### 2.2 `SINK_EVENTS` stream (new)

- **Subjects:** `sink.{sink_id}.events.{EventType}`. Subject construction lives in `core`
  (`core.SinkSubject(sinkID, eventType)`); consumer filter `sink.{sink_id}.events.>`.
  Never hand-build subject strings.
- **Retention:** **Limits** (not WorkQueue). Many Sources may subscribe to one Sink, so
  the stream is fan-out — same retention class as `REPO_EVENTS`.
- Created in `main.go` via `nats.CreateSinkEventsStream`, beside `CreateRepoEventsStream`.

---

## 3. Publication control (publish supervisor)

The Sink's `FilterIDs` are the publication filter. The filter engine is **node-shaped**
(it evaluates node fields), so for each entity kind the rule is "publish iff the node(s) it
concerns pass the filter". The supervisor evaluates against repo A's **own** archive — a
repo reading its own data is ordinary, not a hexagonal violation.

| Source event | Publish decision |
|---|---|
| `NodeCreated` / `NodeUpdated` | Evaluate the filter on the node; republish if it passes. |
| `NodePatched` | Partial update — load the full node from A's archive, evaluate the filter, and **republish normalised as `NodeUpdated`** so subscribers only ever see full-state Created/Updated. |
| `EdgeCreated` / `EdgeUpdated` | Load both endpoint nodes from A's archive; republish iff **both** pass. |
| `ThreadCreated` / `ThreadUpdated`, `ThreadNode*` | Load members; republish iff **≥1** member passes; **prune** the member list to passing nodes. |
| `AnnotationCreated` / `AnnotationUpdated` | Load the target node; republish iff it passes. |

This guarantees a private node's relations never leave A — not even their existence. The
ingest side's reference-resolution skip (§5) then becomes a belt-and-braces guard against
filter-evaluation races, not a correctness crutch.

> An empty `Sink.FilterIDs` publishes everything (the whole repo is the curated channel).
> `FilterChainMode` ("all"/"any") matches the pipeline's `applySourceFilters` semantics;
> the node-filter-chain evaluation is extracted into a shared helper (§7) used by both the
> pipeline and this supervisor.

---

## 4. Configuration model

### 4.1 Source references a specific upstream Sink

```jsonc
// repo-Source.Config
{ "sink_id": "<upstream Sink UUID>" }
```

- The upstream **repo is derived** from the referenced Sink (`Sink.RepoID`) — not stored
  redundantly. The directional edge B→A lives **entirely in the Source**.
- Referenced by **UUID** (Sink short IDs may be per-repo-scoped and so ambiguous across
  repos). The HTTP layer may accept a ref and resolve to UUID.
- Validation: `sink_id` required and must resolve to an existing Sink.

### 4.2 Sink becomes target-agnostic

```jsonc
// repo-Sink.Config
{ }   // no repo_id target; curation IS Sink.FilterIDs
```

- **Model change:** today a repo-Sink requires `config.repo_id` (the target). Now a Sink
  just declares "publish my filtered entities onto my channel" and does not know or care
  who subscribes (federation-friendly). The existing `repo_id`-on-sink validation and the
  sink half of cycle detection are removed.

### 4.3 Cycle detection (`connector/cycle.go`)

- Edges are now **Source → upstream-repo**, resolved via
  `Source.Config.sink_id → GetSink → Sink.RepoID`.
- `repoConnectorTargets(repo)`: for each repo-type **Source** in `repo`, resolve its
  `sink_id` to the upstream Sink's `RepoID`. **Drop the sink-walk.**
- The BFS is otherwise unchanged: adding a Source in B that targets A is rejected at create
  time if A can already reach B (transitively); self-subscribe (`start == target`) is
  rejected.

`cycle.go` at **config time is the only topological loop bound.** Provenance dedup (§5)
gives re-delivery idempotency but cannot stop a true cycle — each hop re-wraps an entity
with a *new* external ID, so dedup never converges, and ingested events are fresh input
(`Hops:0`), so the rules `Hops` limit does not apply either.

---

## 5. Pipeline completion (Items → Threads, Edges, Annotations)

The supervisors translate each discrete NATS event into a **single-entity `IngestBatch`**
and call `pipeline.Ingest`. The in-order consumer chain preserves A's emission order, so an
edge's endpoint nodes are already in B by the time the edge is processed.

### 5.1 Core additions

Provenance pair on the three relational entities (mirroring `Node`):

```go
// Edge, Thread, Annotation each gain:
SourceID   *uuid.UUID `json:"source_id,omitempty"`
ExternalID *string    `json:"external_id,omitempty"`
```

General edges in the batch model (today only reply edges via `IngestItem.ParentExternalID`
are expressible):

```go
type IngestEdge struct {
    ExternalID       string
    SourceExternalID string   // external id of the source node
    TargetExternalID string   // external id of the target node
    Type             string
    Label            string
    Weight           float64
}

type IngestBatch struct {
    // … existing: Threads, Items, Annotations, Tombstones, Cursor …
    Edges []IngestEdge        // NEW: explicit, arbitrary edges
}
```

### 5.2 Archive additions

- Migrations: `source_id` + `external_id` columns on the `edges`, `threads`, and
  `annotations` tables, indexed `(source_id, external_id)`.
- `GetEdgeBySourceExternalID`, `GetThreadBySourceExternalID`,
  `GetAnnotationBySourceExternalID` (mirroring the existing `GetNodeBySourceExternalID`).
- `ListRepoSourcesForSink(sinkID)` — cross-repo: repo-type Sources whose
  `config.sink_id == sinkID`. (Today's `ListSources` is per-repo.)

### 5.3 Pipeline processing order (per batch)

1. **Threads** — find/create by `(SourceID, ExternalThreadID)`; dedup; persist; emit.
2. **Nodes (Items)** — as A2 today (map content, provenance, source filter, dedup, persist,
   emit). Thread membership (`ThreadExternalID`) attaches via the resolved Thread.
3. **Edges** — resolve `SourceExternalID`/`TargetExternalID` to B node UUIDs via
   `GetNodeBySourceExternalID(srcB, …)`; **skip if either endpoint is absent**; dedup by
   `(SourceID, ExternalID)`; persist; emit `EdgeCreated`/`EdgeUpdated`.
4. **Annotations** — resolve target node; **skip if absent**; dedup; persist; emit.

Source filters (`Source.FilterIDs`) apply to **nodes** on the way in (as today). Relations
ride along with the nodes that survived both filters; an unresolved reference is skipped.

Discrete **`ThreadNodeAdded`/`ThreadNodeRemoved`** events (membership changes after the
member node already exists) map to provenance-resolved `AddNodeToThread`/
`RemoveNodeFromThread` operations — a membership mutation, **not** an entity delete (so it
is in scope despite the deletes-deferred boundary). A removal whose node or thread cannot
be resolved in B is a no-op.

---

## 6. Provenance, identity, and attribution

- Ingested entity in B carries `SourceID = srcB.ID` and `ExternalID = <A entity UUID>`
  (taken from the event payload). The supervisor passes **content + references only**; the
  pipeline assigns B's identity, short ID, and version.
- **Re-delivery** → `GetXBySourceExternalID` finds the prior entity → update, not
  duplicate. Idempotent.
- `CreatedBy` on ingested entities = **destination repo owner** (as A2's webhook ingestion
  already does). Preserving A's original author needs the User store and is deferred (B).

---

## 7. Package layout

```
core/        + SinkSubject(sinkID, eventType); IngestEdge + IngestBatch.Edges;
             provenance fields on Edge/Thread/Annotation; repo Source/Sink config validation
nats/        + CreateSinkEventsStream (beside CreateRepoEventsStream)
archive/     + provenance columns + migrations on edges/threads/annotations;
             GetEdge/Thread/AnnotationBySourceExternalID; ListRepoSourcesForSink
connector/   + publish_supervisor.go (REPO_EVENTS → publication filter → SINK_EVENTS);
             cycle.go (Source-graph BFS); repo sink/source validation updates
ingest/      + supervisor.go (SINK_EVENTS → ListRepoSourcesForSink → pipeline.Ingest);
             pipeline.go extended (Threads/Edges/Annotations); shared node-filter helper
main.go      create SINK_EVENTS; build + start both supervisors as app.Runner
```

Adapters still import `core` only; the supervisors reach NATS solely through the injected
`app.Consumer`/`app.Publisher` ports, so NATS stays isolated in `nats/`.

---

## 8. Event coverage

Propagated (Created/Updated only): `NodeCreated`/`NodeUpdated`,
`EdgeCreated`/`EdgeUpdated`, `ThreadCreated`/`ThreadUpdated` + `ThreadNodeAdded`/
`ThreadNodeRemoved` membership, `AnnotationCreated`/`AnnotationUpdated`. `NodePatched` is
normalised to a full `NodeUpdated` on publish (§3).

Deferred: all delete events — `NodeDeleted`/`EdgeDeleted`/`ThreadDeleted`/
`AnnotationDeleted` (need the pipeline tombstone path).

---

## 9. Testing

- **Publish filter (nodes):** only Sink-filter-matching nodes reach `SINK_EVENTS`.
- **Publish filter (relations):** an edge to a filtered-out endpoint is **not** published;
  a thread is published with its member list pruned to passing nodes; an annotation on a
  filtered-out target is not published.
- **Ingest filter:** Source filter excludes non-matching nodes from the pipeline.
- **End-to-end A→B:** node + its edge + thread + annotation in A → all appear in B with
  `SourceID=srcB` and `ExternalID=<A uuid>`, references re-resolved to B's node UUIDs, both
  filters honoured.
- **Reference skip:** an edge whose endpoint was filtered out is skipped (no dangling
  edge).
- **Idempotency:** re-delivery updates in place, never duplicates (all four entity kinds).
- **Cycle prevention:** creating a Source that closes a loop is rejected at create.
- **Chain A→B→C** (optional): B's ingested node re-publishes via B's Sink to C.
- Infrastructure: `archive.NewLocalArchive(t.TempDir())`, embedded NATS test server, build
  tag `sqlite_fts5` on archive paths.

---

## 10. Decomposition impact

A2b **completes the ingestion pipeline** (Threads/Edges/Annotations + cross-repo
provenance) and ships the Source/Sink rails. Consequently **Sub-project B shrinks** to:
external pull adapters (RSS collect-only; Discourse Source+Sink), the cursor machinery
(`Get`/`SaveIngestCursor`), `CollectionCompleted`/`PublishCompleted` operational events, and
the outbound-webhook delivery worker (a `SINK_EVENTS` consumer that POSTs). The hard
pipeline reference-resolution work is no longer B's.

---

## 11. Decisions captured (resolved during design)

| Decision | Choice |
|---|---|
| repo→repo data movement | Over NATS (events carry full entities); **no archive-read exception** |
| Mechanism | Streaming: two single-global-consumer supervisors + curated `SINK_EVENTS` bus |
| Source vs Sink | **Both real.** Source = inbound control, Sink = outbound publication control |
| Topology | Two hops: `REPO_EVENTS → Sink filter → SINK_EVENTS → Source filter → pipeline` |
| repo as adapter? | **No** (`integrations/repo` not created) — internal path, identity mapping; supervisors + pipeline only |
| Source addressing | `Source.Config.sink_id` (UUID); upstream repo derived from the Sink |
| Sink target | **Target-agnostic** broadcast channel; directional edge moves to the Source |
| Relational publication control | **Publish-side strict** — load endpoints from A's archive, publish iff node(s) pass |
| Scope | Nodes **+ Edges + Threads + Annotations** (Created/Updated); deletes/tombstones deferred |
| Loop bound | `cycle.go` at config time (Source-graph BFS); provenance = idempotency only |
