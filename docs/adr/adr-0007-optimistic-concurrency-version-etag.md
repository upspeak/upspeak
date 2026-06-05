---
title: "ADR-0007: Optimistic concurrency via version field and ETag/If-Match"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["concurrency", "api"]
supersedes: ""
superseded_by: ""
---

# ADR-0007: Optimistic concurrency via version field and ETag/If-Match

## Status

**Accepted**

## Context

Multiple clients (and, in future, multiple devices) can edit the same entity. Two
concurrent updates must not silently overwrite one another ("lost update"). Because
Upspeak is API-first and aims to be local-first/multi-device, the concurrency control
needs to be expressible over HTTP and not depend on long-held server locks.

## Decision

Use **optimistic concurrency control** with an explicit version, surfaced over standard
HTTP conditional-request semantics:

- Every entity carries a `Version` integer field, starting at 1 and incremented on each
  successful write.
- Archive write methods check the incoming `Version` against the stored value and return
  `VersionConflictError` on mismatch.
- The HTTP layer maps the entity version to an **`ETag`** and requires **`If-Match`** on
  updates:
  - mismatch → **412 Precondition Failed** (`version_mismatch` / `version_conflict`);
  - missing `If-Match` where it is mandatory → **428 Precondition Required**
    (`if_match_required`);
  - malformed `If-Match` → **412** with an `invalid_if_match` code.

## Consequences

### Positive

- **POS-001**: Lost updates are prevented without server-side locking — writers detect
  conflicts and can retry.
- **POS-002**: Uses standard HTTP conditional requests (`ETag`/`If-Match`), so any
  HTTP-aware client gets concurrency control for free.
- **POS-003**: The same `Version` field supports conflict detection for future
  multi-device sync (event consumers and Phase 6b build on it).
- **POS-004**: Conflict handling is uniform across modules (repo, connector, scheduler,
  rules all use the same status codes and error codes).

### Negative

- **NEG-001**: Clients must implement read-modify-write with retry on 412, which is more
  client work than last-write-wins.
- **NEG-002**: The version/ETag check and status-code mapping are repeated across module
  handlers, a small amount of duplicated logic to keep consistent.
- **NEG-003**: Optimistic control detects conflicts but does not resolve them; a
  resolution policy is still required for automated multi-device sync (deferred to
  Phase 6b).

## Alternatives Considered

### Last-write-wins (no version check)

- **ALT-001**: **Description**: Accept every write and let the latest overwrite.
- **ALT-002**: **Rejection Reason**: Silently loses concurrent edits — unacceptable for
  a knowledge store the user curates.

### Pessimistic locking

- **ALT-003**: **Description**: Lock an entity for the duration of an edit.
- **ALT-004**: **Rejection Reason**: Poor fit for stateless HTTP and offline/multi-device
  editing; introduces lock lifetime, ownership, and deadlock concerns.

## Implementation Notes

- **IMP-001**: `Version` lives on every entity; archive write methods raise
  `VersionConflictError` (see `core` error types and the `archive/` stores).
- **IMP-002**: HTTP mapping uses `api.WriteError` with codes `if_match_required` (428),
  `version_mismatch`/`version_conflict`/`invalid_if_match` (412); examples in
  `scheduler/handlers.go`, `connector/handlers_source.go`, `rules/handlers.go`.
- **IMP-003**: For Phase 6b, layer a conflict-resolution strategy (e.g. default
  last-write-wins with per-conflict override) on top of this detection mechanism.

## References

- **REF-001**: [[adr-0006-http-api-conventions]] — conditional requests within the API
  conventions.
- **REF-002**: [[adr-0005-local-first-archive-storage-split]] — archive write methods
  raise `VersionConflictError`, the mechanism this ADR surfaces over HTTP.

> The write-path record (`adr-0008-hybrid-sync-write-async-events.md`) carries the
> entity version in events for downstream consumers; it references this ADR.
