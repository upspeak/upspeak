---
title: "ADR-0008: Hybrid synchronous write core with asynchronous event propagation"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["consistency", "events"]
supersedes: ""
superseded_by: ""
---

# ADR-0008: Hybrid synchronous write core with asynchronous event propagation

## Status

**Accepted**

## Context

A write to Upspeak (creating a node, adding an edge, …) must be durable and confirmed
to the caller. It also has *consequences*: the search index should update, rules should
evaluate, real-time subscribers should be notified, and — in future — other devices
should sync. Those consequences must not slow the write down or, worse, fail it when a
downstream component is unavailable.

The architecture needed a clear stance on what is synchronous and authoritative versus
what is asynchronous and derived.

## Decision

Adopt a **hybrid model**: a synchronous, authoritative write core with asynchronous
event propagation.

- The HTTP handler writes **synchronously** to `core.Archive`. Once the archive commits,
  the write is confirmed to the client.
- The handler then publishes a domain event **fire-and-forget** via
  `app.Publisher.PublishEvent(...)`, which builds the `core.Event` envelope and
  publishes it to the `REPO_EVENTS` JetStream stream.
- Publish errors are **logged and never block or fail the response**.
- JetStream provides durable, at-least-once delivery from the stream to its durable
  consumers (rules engine, future sync), decoupling consumers from the request path.

The archive is the source of truth; events carry consequences.

## Consequences

### Positive

- **POS-001**: Writes are fast and their success depends only on the archive, not on
  the broker or any consumer.
- **POS-002**: Consumers are decoupled and can be added without touching the write path.
- **POS-003**: Durable JetStream delivery means consumers survive restarts and process
  backlog (at-least-once).
- **POS-004**: A downstream outage degrades derived features, not the ability to write.

### Negative

- **NEG-001**: The archive commit and the event publish are **not atomic**. There is no
  transactional outbox, so if the publish fails *after* the commit succeeds, the event
  is lost — derived state (index, rules, realtime) silently misses that change.
- **NEG-002**: Delivery is at-least-once, so every consumer must be idempotent.
- **NEG-003**: Derived state is eventually consistent; a client reading immediately
  after a write may observe stale index/search results.

## Alternatives Considered

### Transactional outbox

- **ALT-001**: **Description**: Write the event to an outbox table in the same
  transaction as the data, then relay it to the broker.
- **ALT-002**: **Rejection Reason**: Added complexity (outbox table, relay loop,
  dedup) is not justified for the current single-user, single-archive deployment. It is
  the natural mitigation for NEG-001 and should be revisited when remote archives and
  multi-device sync land.

### Synchronous publish inside the request

- **ALT-003**: **Description**: Publish the event before responding and fail the write
  if publishing fails.
- **ALT-004**: **Rejection Reason**: Couples write latency and success to broker
  availability, defeating the resilience goal.

### Event sourcing as the source of truth

- **ALT-005**: **Description**: Treat the event log as authoritative and derive the
  archive from it.
- **ALT-006**: **Rejection Reason**: Archive-as-truth is simpler to reason about, fits
  local-first storage, and avoids rebuild/projection machinery.

## Implementation Notes

- **IMP-001**: Write path: repo/module handlers → `core.Archive` write →
  `app.Publisher.PublishEvent(...)`. Envelope and subject owned by `core`
  (`Event.Subject()`).
- **IMP-002**: All event consumers are expected to be idempotent given at-least-once
  delivery.
- **IMP-003**: Revisit NEG-001 (transactional outbox or equivalent) as part of Phase 6b
  (multi-device sync) and any remote-archive work, where lost events become correctness
  bugs rather than cosmetic gaps.

## References

- **REF-001**: [[adr-0005-local-first-archive-storage-split]] — the synchronous write
  target that is confirmed to the client.
- **REF-002**: [[adr-0007-optimistic-concurrency-version-etag]] — entity version carried
  in events for downstream consumers.
- **REF-003**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — the
  messaging port (`app.Publisher`) used to publish events.

> The stream and consumers these events flow through
> (`adr-0009-global-repo-events-stream.md`), and the consumers themselves — the rules
> engine (`adr-0010-filter-and-rules-engine.md`) and real-time fan-out
> (`adr-0011-realtime-core-nats-fanout.md`) — build on this write path and reference it,
> not the reverse.
