---
title: "ADR-0005: Local-first archive with metadata/body storage split"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["storage", "local-first"]
supersedes: ""
superseded_by: ""
---

# ADR-0005: Local-first archive with metadata/body storage split

## Status

**Accepted**

## Context

Upspeak is local-first: a user's archive should work fully offline on their own machine,
while leaving a clear path to a hosted/remote archive later. The data is a knowledge
graph — small, highly queryable metadata (types, subjects, edges, sequences) alongside
node **body content** that can be arbitrarily large (text, captured documents). These
two have different access patterns: metadata is queried and joined constantly; bodies
are read or streamed whole.

A storage design was needed that serves offline use, queries metadata efficiently,
stores large bodies cheaply, and maps cleanly onto an eventual remote backend without
changing the domain.

## Decision

Define a single storage port, **`core.Archive`** (composed of focused sub-interfaces),
that both local and remote implementations satisfy. The shipped **local** implementation
(`archive.LocalArchive`) splits storage by concern:

- **Metadata → SQLite** at `{path}/.upspeak/metadata.db`: everything queryable —
  entity rows, edges, threads, annotations, sequences. The `nodes` table has **no body
  column**.
- **Node body content → filesystem** at `{path}/content/{node_id}`, one file per node.

The intended **remote** implementation maps the same split onto Postgres (metadata) +
object storage (bodies) behind the identical interface. SQLite is opened with hardening
pragmas: `_foreign_keys=on`, `_busy_timeout=5000`, `_secure_delete=on`.

## Consequences

### Positive

- **POS-001**: Fully local, offline-capable archive — no external services required to
  read or write.
- **POS-002**: Metadata stays small and fast to query/join; large bodies live as files
  that are cheap to store and stream.
- **POS-003**: The split mirrors the planned remote backend (Postgres + object storage),
  so the remote adapter is an interface implementation rather than a redesign.
- **POS-004**: Defensive SQLite configuration (foreign keys, busy timeout, secure
  delete) reduces corruption and contention surprises.

### Negative

- **NEG-001**: A node's metadata row (SQLite) and its body file are **not written in a
  single transaction** — body files are written after the metadata commit, leaving a
  small partial-failure window.
- **NEG-002**: The remote implementation does not yet exist, so the local/remote seam
  is designed but **unvalidated**; the interface could harbour SQLite-shaped
  assumptions (see [[adr-0002-hexagonal-architecture-infrastructure-isolation]]
  NEG-002).
- **NEG-003**: SQLite is single-writer; heavy concurrent writes serialise (mitigated by
  the busy timeout, not eliminated).
- **NEG-004**: Full-text search depends on the `sqlite_fts5` build tag; without it the
  archive degrades to empty search results rather than failing, so production builds
  must set the tag deliberately.

## Alternatives Considered

### Store body content in SQLite as BLOBs

- **ALT-001**: **Description**: Keep node bodies in a `body` column in the database.
- **ALT-002**: **Rejection Reason**: Bloats the database, harms streaming, and inflates
  vacuum/backup cost for large content.

### Document database

- **ALT-003**: **Description**: Use a document store for both metadata and bodies.
- **ALT-004**: **Rejection Reason**: Weaker relational/graph querying for edges and
  traversal, and a heavier dependency than a local-first tool warrants.

### Files only, no SQLite

- **ALT-005**: **Description**: Store everything as files with no relational metadata
  store.
- **ALT-006**: **Rejection Reason**: No efficient way to query, filter, or join
  metadata or maintain sequences and the graph.

## Implementation Notes

- **IMP-001**: Local adapter in `archive/` — `local.go` (facade + schema init), the
  per-entity `*_store.go` files, and `schema.go` (DDL). Body path handling lives
  alongside the node store.
- **IMP-002**: When the remote adapter is built, exercise the `core.Archive` contract
  against both implementations (shared conformance tests) to flush out leaked
  assumptions before relying on the seam.
- **IMP-003**: Consider closing NEG-001 (atomic metadata+body write, or a
  reconciliation/repair step) if partial writes prove to occur in practice.

## References

- **REF-001**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] —
  `core.Archive` as a port.
- **REF-002**: [[adr-0004-identity-uuidv7-short-ids]] — sequences and short IDs are
  managed inside the archive.

> The optimistic-concurrency record (`adr-0007-optimistic-concurrency-version-etag.md`)
> relies on the archive write methods raising `VersionConflictError`; it references this
> ADR rather than the reverse.
