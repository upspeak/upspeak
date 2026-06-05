---
title: "ADR-0004: Identity via UUID v7 primary keys with scoped short IDs"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["identity"]
supersedes: ""
superseded_by: ""
---

# ADR-0004: Identity via UUID v7 primary keys with scoped short IDs

## Status

**Accepted**

## Context

Every entity in Upspeak needs a stable, globally unique identifier that works offline
(no central allocator), sorts sensibly by creation time, and is friendly to a future
federated/multi-device world where IDs minted on different devices must not collide.
At the same time, humans and clients need identifiers they can read, type, and reference
in URLs — a raw UUID is unusable in conversation or a CLI.

## Decision

Use a **two-tier identity scheme**:

- **UUID v7 as the primary key** for every entity (via `google/uuid`). UUID v7 is
  time-ordered, so keys sort by creation time and index locality is good, while
  remaining collision-free without coordination.
- **A human-friendly short ID** alongside each UUID, of the form `{PREFIX}-{SEQ}` —
  `REPO-1`, `NODE-42`, `EDGE-15`, `THREAD-7`, `ANNO-3`, `FILTER-2`, `JOB-109`.
- Short-ID **sequences are scoped**: per-repo (nodes, edges, threads, annotations),
  per-user (repos), or global (jobs, schedules, users).
- Helpers live in `core` (`NewID`, `FormatShortID`, `ParseShortID`); sequence
  generation is **private to `archive/`** (`nextRepoSequence`, `nextUserSequence`,
  `nextGlobalSequence`) and never exposed on the `core.Archive` interface.
- Short IDs are **immutable** once assigned and sequences never reuse numbers.

## Consequences

### Positive

- **POS-001**: Offline ID generation with no central allocator — essential for
  local-first and future multi-device use.
- **POS-002**: Time-ordered keys (UUID v7) improve index locality and give a natural
  creation order without a separate timestamp sort.
- **POS-003**: Short IDs are readable, typeable, and self-describing (the prefix encodes
  the entity type), which directly enables a flat URL scheme where the prefix selects the
  entity type (see the HTTP API conventions ADR, which builds on this).
- **POS-004**: Keeping sequence generation inside `archive/` stops callers from
  depending on allocation details.

### Negative

- **NEG-001**: Two identifiers per entity must be kept consistent (UUID + short ID),
  adding write-time bookkeeping.
- **NEG-002**: Sequence allocation is archive-local state; a future remote/multi-writer
  archive must define how per-scope sequences are allocated without collision.
- **NEG-003**: Immutable, non-reused short IDs mean gaps appear after deletes (a number
  is "spent" even if its entity is gone) — acceptable, but occasionally surprising.

## Alternatives Considered

### Auto-increment integer primary keys

- **ALT-001**: **Description**: Use database auto-increment integers as the primary key.
- **ALT-002**: **Rejection Reason**: Requires a central allocator, collides across
  devices/instances, and is hostile to offline and federated operation.

### UUID v4 primary keys

- **ALT-003**: **Description**: Use random UUID v4.
- **ALT-004**: **Rejection Reason**: Not time-ordered, hurting index locality and
  natural ordering; UUID v7 gives the same uniqueness with better properties.

### Short IDs only (no UUID)

- **ALT-005**: **Description**: Use `{PREFIX}-{SEQ}` as the sole identifier.
- **ALT-006**: **Rejection Reason**: Scoped sequences are not globally unique and depend
  on a per-scope allocator, which fails for federation and cross-repo references.

## Implementation Notes

- **IMP-001**: `core/identity.go` defines `NewID`, `FormatShortID`, `ParseShortID`, and
  the prefix constants.
- **IMP-002**: Sequence methods are private in `archive/sequences.go`; do not promote
  them to the `core.Archive` interface.
- **IMP-003**: A remote/multi-writer archive must specify per-scope sequence allocation
  (e.g. reservation or per-writer ranges) before relying on short IDs across instances.

## References

- **REF-001**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — sequence
  generation is kept private to the archive adapter, behind the storage port.
- **REF-002**: `core/identity.go` — the identity helpers and prefix constants.

> Later records build on this decision: the HTTP API conventions
> (`adr-0006-http-api-conventions.md`) use short-ID prefixes for flat routing, and the
> local archive (`adr-0005-local-first-archive-storage-split.md`) implements the scoped
> sequences. Those ADRs reference this one; this one does not depend on them.
