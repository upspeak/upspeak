---
title: "ADR-0002: Hexagonal architecture with isolated infrastructure adapters"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["hexagonal", "ports-and-adapters"]
supersedes: ""
superseded_by: ""
---

# ADR-0002: Hexagonal architecture with isolated infrastructure adapters

## Status

**Accepted**

## Context

Upspeak must keep its domain logic independent of the concrete technology behind each
infrastructure capability, because each capability is designed to have **multiple
interchangeable adapters** rather than a single fixed implementation:

- **Storage** can be local (SQLite + filesystem) or remote (Postgres + object storage).
- **Messaging** can run against an embedded NATS JetStream broker or an external/hosted
  one — and this choice is already selectable at runtime via `app.Config`, not a
  hypothetical future swap.

The intent is not that one implementation replaces another over time, but that several
can coexist as first-class options chosen by configuration or deployment. If domain code
imported these technologies directly, supporting more than one implementation — or
letting configuration pick between them — would ripple through every package, and unit
testing the domain would require standing up real infrastructure.

A boundary discipline was needed that lets the domain be expressed and tested in terms
of intent, with technology bound only at the edges.

## Decision

Adopt **ports-and-adapters (hexagonal) architecture**:

- `core/` holds domain models and **port interfaces** with no infrastructure imports.
  The storage port is `core.Archive`, composed of focused sub-interfaces
  (`NodeStore`, `EdgeStore`, …; see `core/archive.go`).
- `archive/` and `nats/` are **adapters** that implement the ports.
- The messaging boundary is enforced by a hard rule: **only `nats/` may import
  `github.com/nats-io/*`**. Every other package depends on the `app.Publisher`,
  `app.Subscriber`, and `app.Consumer` interfaces.
- Subject and event-envelope construction is owned by `core` (`Event.Subject()`,
  `core.JobSubject`, …), so adapters and modules never hand-build wire formats.

Composition happens in exactly one place, `main.go`, which constructs adapters and
injects them into modules.

## Consequences

### Positive

- **POS-001**: The domain is testable without real infrastructure — modules accept
  narrow store interfaces and can run against fakes.
- **POS-002**: Storage and messaging implementations can be swapped behind their ports
  without touching domain or module code.
- **POS-003**: The NATS-isolation rule is mechanically checkable (inspect each package's
  import graph) and keeps broker concerns from leaking across the codebase.
- **POS-004**: A single wiring site (`main.go`) makes the dependency graph explicit.

### Negative

- **NEG-001**: Indirection and boilerplate — interfaces, setters, and envelope
  helpers — add ceremony to otherwise direct calls.
- **NEG-002**: The storage port is partly speculative: only one `core.Archive`
  implementation (`LocalArchive`, the local archive adapter) exists, so the local/remote
  seam is designed but not yet validated by a second adapter.
- **NEG-003**: Interface drift risk — without a second implementation, the port can
  quietly accrete SQLite-shaped assumptions.

## Alternatives Considered

### Layered N-tier architecture

- **ALT-001**: **Description**: Conventional controller → service → repository layers
  with technology referenced from the lower layers.
- **ALT-002**: **Rejection Reason**: Layering by itself does not prevent infrastructure
  imports from leaking upward; the swap and isolation goals would rely on convention
  alone.

### Direct use of NATS and SQLite throughout

- **ALT-003**: **Description**: Let each module talk to `nats-io` and the database
  driver directly.
- **ALT-004**: **Rejection Reason**: Couples every module to specific technologies,
  makes the domain untestable in isolation, and forecloses the remote-archive path.

## Implementation Notes

- **IMP-001**: Ports — `core/archive.go` (storage sub-interfaces), `app/app.go`
  (`Publisher`/`Subscriber`/`Consumer`). Adapters — `archive/`, `nats/`.
- **IMP-002**: Enforcement — periodically verify no package outside `nats/` imports
  `github.com/nats-io/*`; treat a violation as an architectural regression.
- **IMP-003**: Modules should accept the **narrowest** port they need (e.g.
  `core.NodeStore`) rather than the full `core.Archive` where practical.

## References

- **REF-001**: [[adr-0001-api-first-server]] — the API-first product this architecture
  serves.
- **REF-002**: `core/archive.go`, `app/app.go` — the storage and messaging ports.

> Later records depend on these ports: the modular monolith
> (`adr-0003-modular-monolith-app-framework.md`), the local archive adapter
> (`adr-0005-local-first-archive-storage-split.md`), the write path
> (`adr-0008-hybrid-sync-write-async-events.md`), and the embedded broker
> (`adr-0012-embedded-nats-jetstream-server.md`) all reference this ADR.
