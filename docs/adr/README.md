# Architectural Decision Records

This directory captures the architectural decisions behind Upspeak as
[Architectural Decision Records](https://adr.github.io/) (ADRs). Each record states the
context that forced a decision, the decision itself, its consequences (positive *and*
negative), and the alternatives that were rejected.

## How these records came to be, and how they will grow

The initial set (ADR-0001 to ADR-0012) was written **retroactively on 2026-06-05** to
document the architecture of the already-shipped codebase. Because they describe
decisions already in active use rather than proposals awaiting ratification, they are
recorded as **Accepted**. Where a decision created a live tension (e.g. no transactional
outbox, an as-yet-unvalidated remote-archive seam, the nil-until-wired wiring order),
that tension is recorded honestly as a negative consequence rather than smoothed over.

**Going forward, ADRs are created as the need arises** — one per non-trivial
architectural or technology decision, written at the time the decision is made, not in
retrospective batches. Add the next sequential number rather than editing an existing
record; supersede an older ADR by setting its `superseded_by` front-matter field and the
new ADR's `supersedes`.

## Linear, backward-only references

ADRs are numbered in **dependency order**: a record only ever references
**lower-numbered** records, never higher-numbered ones. This keeps the sequence linear
and acyclic — reading front to back, every referenced decision has already been
introduced. When a later decision builds on an earlier one, the link lives in the
*later* ADR. (Where an earlier record needs to point forward for the reader's benefit, it
does so in prose by filename, not as a dependency link.)

## Authority

Per `docs/next-steps.md`, two things outrank prose documentation: the **shipped code**
(under `app/`, `core/`, and the module packages) and
**`assets/high-level-concepts-0.1.png`** (the defining data-flow architecture). These
ADRs explain *why* the code is shaped the way it is; the code remains the source of truth
for *what* it does.

## The records

### Pillars (foundational, cross-cutting)

| ADR | Title | Status |
|-----|-------|--------|
| [0001](adr-0001-api-first-server.md) | API-first server with no bundled UI | Accepted |
| [0002](adr-0002-hexagonal-architecture-infrastructure-isolation.md) | Hexagonal architecture with isolated infrastructure adapters | Accepted |
| [0003](adr-0003-modular-monolith-app-framework.md) | Modular monolith via the app micro-framework | Accepted |

### Decision records (focused)

| ADR | Title | Status |
|-----|-------|--------|
| [0004](adr-0004-identity-uuidv7-short-ids.md) | Identity via UUID v7 primary keys with scoped short IDs | Accepted |
| [0005](adr-0005-local-first-archive-storage-split.md) | Local-first archive with metadata/body storage split | Accepted |
| [0006](adr-0006-http-api-conventions.md) | HTTP API conventions — response envelope, flat URLs, ref resolution | Accepted |
| [0007](adr-0007-optimistic-concurrency-version-etag.md) | Optimistic concurrency via version field and ETag/If-Match | Accepted |
| [0008](adr-0008-hybrid-sync-write-async-events.md) | Hybrid synchronous write core with asynchronous event propagation | Accepted |
| [0009](adr-0009-global-repo-events-stream.md) | Single global REPO_EVENTS stream with durable consumers | Accepted |
| [0010](adr-0010-filter-and-rules-engine.md) | Reusable filter engine and hop-bounded rules engine | Accepted |
| [0011](adr-0011-realtime-core-nats-fanout.md) | Real-time delivery via core-NATS fan-out with server-side filtering | Accepted |
| [0012](adr-0012-embedded-nats-jetstream-server.md) | Embedded NATS JetStream server | Accepted |

## How to read these

Start with the pillars (0001–0003) for the shape of the system, then read the focused
records in order. The numbering tells a story: the domain foundations (identity 0004,
local archive 0005, API conventions 0006, concurrency 0007) come before the event-driven
machinery that builds on them (write path 0008, events stream 0009, rules 0010, realtime
0011, embedded broker 0012). Because references only point backward, following the links
from any record walks you down to the foundations it rests on — for example, the global
durable stream (0009) and the ephemeral real-time fan-out (0011) are deliberately
different tools for different durability needs, both built on the same write path (0008).
