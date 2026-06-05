---
title: "ADR-0009: Single global REPO_EVENTS stream with durable consumers"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["nats", "jetstream", "events"]
supersedes: ""
superseded_by: ""
---

# ADR-0009: Single global REPO_EVENTS stream with durable consumers

## Status

**Accepted**

## Context

Domain events are published per repository on subjects of the form
`repo.{repo_id}.events.{EventType}` ([[adr-0008-hybrid-sync-write-async-events]]).
Downstream processors — the rules engine today, multi-device sync later — need durable,
at-least-once delivery that survives restarts and processes one repository's events the
same way as any other's.

An early design used a **per-repository** JetStream stream
(`REPO_{repo_id}_EVENTS`, via `CreateRepoStream`). That forces a new stream and a new
consumer for every repository created at runtime, and a durable consumer cannot span
repositories. It does not match how `JOBS` and `SCHEDULES` are already structured (one
global stream each).

## Decision

Use a **single global `REPO_EVENTS` JetStream stream** that captures `repo.*.events.>`
for every repository, with **Limits** retention. One durable pull consumer can then
serve all repositories.

- Streams are created at startup in `main.go` (`CreateRepoEventsStream`), consistent with
  `CreateJobsStream` (`JOBS`, WorkQueue) and `CreateSchedulesStream` (`SCHEDULES`,
  WorkQueue).
- Consumers are **durable pull consumers with explicit ack**: `MaxDeliver=5`,
  `AckWait=30s`. The `rules-engine` consumer runs on `REPO_EVENTS`; `job-runner` on
  `JOBS`; `schedule-runner` on `SCHEDULES`.
- The per-repo `CreateRepoStream` is retained for tests only and is **superseded**;
  wiring it in production would overlap `REPO_EVENTS` on subject space and must not be
  done.

## Consequences

### Positive

- **POS-001**: One stream and one durable consumer serve every repository — no
  per-repo stream/consumer churn as repositories are created.
- **POS-002**: Consistent with the `JOBS`/`SCHEDULES` design, so all background
  processing follows one mental model (global stream + durable pull consumer).
- **POS-003**: Durable delivery with bounded redelivery (`MaxDeliver=5`, `AckWait=30s`)
  gives crash-safe, at-least-once processing.
- **POS-004**: Adding a new processor is just another durable consumer on the same
  stream.

### Negative

- **NEG-001**: All repositories share one stream, so there is no per-repo isolation of
  retention, throughput, or replay — a noisy repository shares the stream with quiet
  ones.
- **NEG-002**: Consumers must filter/route by `repo_id` themselves (the subject and the
  event envelope carry it), rather than getting a pre-scoped stream.
- **NEG-003**: This decision post-dates parts of `CLAUDE.md`, whose NATS section still
  describes per-repo event streams in places; documentation must be read with this ADR
  as the current truth.

## Alternatives Considered

### Per-repository event stream (`REPO_{repo_id}_EVENTS`)

- **ALT-001**: **Description**: Create a dedicated stream and consumer per repository
  (`CreateRepoStream`).
- **ALT-002**: **Rejection Reason**: Runtime stream/consumer proliferation, no single
  durable consumer can span repos, and it diverges from the `JOBS`/`SCHEDULES` pattern.
  Kept for tests only; superseded for production.

### Core-NATS fan-out (no JetStream) for these consumers

- **ALT-003**: **Description**: Deliver events to processors over plain core NATS.
- **ALT-004**: **Rejection Reason**: No durability or redelivery — a processor restart
  would drop in-flight events, unacceptable for rules and sync. (Core fan-out *is* the
  right tool for ephemeral real-time delivery, a separate decision recorded later in the
  sequence.)

## Implementation Notes

- **IMP-001**: Stream and consumer definitions in `nats/streams.go` and
  `nats/consumers.go`; created from `main.go`. Subjects built only via `core`
  (`Event.Subject()`, `core.JobSubject`, `core.ScheduleTriggerSubject`).
- **IMP-002**: Never wire `CreateRepoStream` alongside `REPO_EVENTS` — the subject
  overlap will conflict.
- **IMP-003**: If per-repo isolation is ever required (e.g. quotas, tenant separation),
  revisit with subject-filtered consumers or partitioned streams rather than reverting to
  per-repo streams.

## References

- **REF-001**: [[adr-0008-hybrid-sync-write-async-events]] — what produces these events.
- **REF-002**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — NATS is an
  isolated adapter behind the messaging port.

> The consumers of this stream are recorded later: the rules engine
> (`adr-0010-filter-and-rules-engine.md`) and the real-time fan-out
> (`adr-0011-realtime-core-nats-fanout.md`, which deliberately uses core NATS instead of
> a durable consumer). They reference this ADR.
