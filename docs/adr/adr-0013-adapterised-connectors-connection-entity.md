---
title: "ADR-0013: Adapterised connectors via a Connection entity and adapter registry"
status: "Proposed"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["connectors", "ingestion", "adapters", "domain-model"]
supersedes: ""
superseded_by: ""
---

# ADR-0013: Adapterised connectors via a Connection entity and adapter registry

## Status

**Proposed**

> Approved as design (`docs/design/2026-06-05-connector-adapterisation.md`); not yet
> implemented. Becomes **Accepted** when the framework ships in code.

## Context

The knowledge-graph spine is shipped, but the connector layer is hollow: external
backends are rejected by `isSupportedConnector()` and the job executors record success
without doing real work. The Phase-4 connector model is **flat and
credential-duplicating**:

- `Source` and `Sink` each carry a `Connector` and one opaque `Config map[string]any`
  holding endpoint, credentials, *and* operation settings together. There is no shared
  "configured connection to an external system", so credentials would be re-entered per
  source and per sink.
- Validation is an inline `switch` (`validateSourceConfig`, `isSupportedConnector`) that
  grows with every connector type.
- `connector/` imports `jobs/` to create jobs, so the job runner **cannot** import
  `connector/` to execute backends — that would form an import cycle.

The ingestion track must support integrations spanning four distinct modes — pull (RSS),
request/response REST (Discourse), streaming (Matrix), and OAuth2 (Mastodon) — without
the abstraction secretly bending to one of them, and with **all data modelling in
`core`** (the domain).

## Decision

Adapterise connectors around a first-class **Connection** entity and an **adapter
registry**, with the domain model in `core` and integrations isolated in their own
packages.

- **Connection** (`core`): an **owner-scoped**, configure-once link to one external
  system (credentials, endpoint, shared rate-limit, health/status). `CONN-{seq}`,
  managed at `/api/v1/connections` (not under `/repos/`). `Source`/`Sink` gain a
  **nullable `ConnectionID`** and keep only operation-specific `Config`; the adapter's
  `Capabilities().RequiresConnection` decides whether a Connection is mandatory (RSS and
  webhook need none).
- **Adapter contract** (`core`): a base `Adapter` (`Type`, `Capabilities`,
  `ConnectionSchema`, `Validate*`, `TestConnection`) plus **optional role interfaces**
  `Collector` / `Publisher` / `Streamer` / `OAuthProvider`, type-asserted by the
  registry. An adapter implements only the roles it supports, so capability is explicit
  and `Capabilities()` drives create-time gating.
- **Isolation + registry**: adapters live in `integrations/<name>/` and import `core`
  **only**. A registry (`AdapterFor(ConnectorType)`) is injected via setter into the job
  runner and the connector module; `main.go` is the only place that knows the concrete
  adapters. This breaks the `connector ↔ jobs` cycle using the codebase's existing DI
  grammar ([[adr-0003-modular-monolith-app-framework]]).
- **External-ID boundary**: adapters emit an external-ID-keyed `IngestBatch`
  (threads/items/annotations/tombstones/cursor) and never touch internal UUIDs or the
  archive. An `ingest` pipeline owns identity resolution and **reuses the existing
  synchronous write + event path** ([[adr-0008-hybrid-sync-write-async-events]]), so
  rules, realtime, and search work over ingested data unchanged. `Node` gains nullable
  `SourceID` + `ExternalID` provenance (idempotent dedup, container grouping, and search
  attribution in one pair of fields).
- **Domain mapping**: Connection = account/server; **Source = container** (Matrix room /
  Discourse category); Thread = conversation (topic / `m.thread`); Node = message; Edge =
  relation; Annotation = reaction (`Motivation`); User = author (`username`@`hostname`).

## Consequences

### Positive

- **POS-001**: Credentials are configured once per external system and reused across
  every Source/Sink, with one shared rate-limit budget and one place for health/auth
  failure.
- **POS-002**: Adding an integration is additive — a new package under `integrations/`
  implementing the role interfaces it supports — with no edits to `jobs/` or
  `connector/` and no growing `switch`.
- **POS-003**: The `connector ↔ jobs` import cycle is removed cleanly via an injected
  registry interface, consistent with how `Archive`/`Publisher`/`Consumer` are wired.
- **POS-004**: One `Node.SourceID`/`ExternalID` pair delivers idempotent re-collection,
  container grouping, and the search source-attribution stub at once.
- **POS-005**: Ingestion reuses the existing write/event path, so the domain vocabulary
  (Node/Edge/Thread/Annotation/User) stays frozen and downstream machinery needs no
  special cases.

### Negative

- **NEG-001**: A new first-class entity (Connection) plus an adapter/registry/pipeline
  layer is materially more moving parts than the flat Phase-4 model.
- **NEG-002**: Owner-scoped Connections referenced by repo-scoped Sources/Sinks
  introduce a cross-scope reference whose integrity is only trivially enforced while auth
  is stubbed (`defaultOwnerID`).
- **NEG-003**: The external-ID-keyed pipeline adds an identity-resolution step (lookups,
  thread/parent wiring) that every adapter implicitly depends on being correct.
- **NEG-004**: Capability flags and role interfaces can drift from reality (an adapter
  that advertises `Publish` but mis-implements it) — caught only by tests, not the type
  system.

## Alternatives Considered

### Keep flat Source/Sink config with a growing validation switch

- **ALT-001**: **Description**: Make backends real but keep credentials in
  `Source`/`Sink` `Config` and extend the inline `switch`.
- **ALT-002**: **Rejection Reason**: Duplicates credentials per source/sink, gives no
  configure-once reuse, and the `switch` plus the import cycle both remain.

### Adapter registry without a Connection entity

- **ALT-003**: **Description**: Introduce the adapter interface + registry to break the
  cycle, but defer the Connection (credentials stay in Source/Sink).
- **ALT-004**: **Rejection Reason**: Delivers extensibility but not the explicit
  configure-once/reuse the design requires, and forces a later breaking migration of the
  credential model.

### Process-isolated or Go-plugin connectors

- **ALT-005**: **Description**: Load integrations as separate processes or Go plugins for
  hard isolation.
- **ALT-006**: **Rejection Reason**: Heavy operational and build complexity for a
  single-binary, local-first system; in-tree packages importing `core` give sufficient
  isolation with none of the plugin-loading fragility.

## Implementation Notes

- **IMP-001**: Package layout — `core` (types + Adapter interface + ports),
  `integrations/{rss,discourse,matrix,mastodon}/` (import `core` only), `ingest`
  (pipeline + registry + stream supervisor), `connection` (CRUD + OAuth flow); `jobs`
  and `connector` depend on the injected registry interface, `main.go` wires.
- **IMP-002**: Migrate the existing `webhook` and `repo` connectors onto adapters as part
  of the framework so there is a single code path.
- **IMP-003**: Delete integrity — `GetConnectionReferences(connID)` returns referencing
  Sources/Sinks; `DELETE` returns `409` if any exist (mirrors filter delete,
  [[adr-0010-filter-and-rules-engine]]).
- **IMP-004**: Sequenced delivery — A: framework + Mastodon (OAuth exerciser) +
  webhook/repo migration; B: RSS + Discourse pull; C: Matrix streaming + supervisor. See
  the design doc for the full decomposition.

## References

- **REF-001**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — adapters
  as isolated infrastructure behind a `core` port.
- **REF-002**: [[adr-0003-modular-monolith-app-framework]] — the DI grammar the registry
  follows.
- **REF-003**: [[adr-0008-hybrid-sync-write-async-events]] — the write/event path the
  ingest pipeline reuses.
- **REF-004**: [[adr-0010-filter-and-rules-engine]] — Sources/Sinks reference filters;
  filters stay generic over normalised payloads.
- **REF-005**: `docs/design/2026-06-05-connector-adapterisation.md` — the full design.
