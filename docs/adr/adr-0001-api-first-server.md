---
title: "ADR-0001: API-first server with no bundled UI"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["api"]
supersedes: ""
superseded_by: ""
---

# ADR-0001: API-first server with no bundled UI

## Status

**Accepted**

## Context

Upspeak is personal-first, federated knowledge infrastructure intended to be reached
by many kinds of client over time: a web app, desktop and mobile clients, automation
and scripts, and — eventually — federated peers operated by other people. The defining
data-flow architecture (`assets/high-level-concepts-0.1.png`) treats the server as a
knowledge engine that many surfaces consume, not as a single application with one
front end.

Bundling a user interface into the server would couple UI and server release cycles,
privilege one client over all others, and sit awkwardly with the local-first and
federated goals, where the client talking to an archive may not be written by us at
all. A decision was needed on whether the server owns a UI or exposes only a contract.

## Decision

Ship a **pure HTTP API server with no bundled UI**. Every interaction happens over
HTTP. All application modules mount under a single versioned prefix (`/api/v1`) and
return a uniform response envelope (`api/envelope.go`). The only non-`/api/v1`
endpoints are the reserved system probes `/healthz` and `/readiness`. Clients are
separate programs that speak this contract.

## Consequences

### Positive

- **POS-001**: Client diversity is unconstrained — web, native, CLI, automation, and
  future federated peers all consume the same contract.
- **POS-002**: The HTTP contract is the single integration surface, which keeps the
  server's responsibilities focused and independently testable.
- **POS-003**: Clients evolve and release independently of the server.
- **POS-004**: Aligns with the federated/local-first direction, where third-party
  clients and peers are expected.

### Negative

- **NEG-001**: There is no out-of-the-box user experience; a usable product requires
  at least one separately built client.
- **NEG-002**: The API is the _only_ contract, so backward-compatibility and
  versioning discipline become load-bearing from day one.
- **NEG-003**: Discoverability falls on documentation (and, later, a machine-readable
  spec) rather than a shipped UI.

## Alternatives Considered

### Bundled single-page application

- **ALT-001**: **Description**: Serve a first-party SPA from the same binary alongside
  the API.
- **ALT-002**: **Rejection Reason**: Couples UI and server release cycles and
  privileges one client, undermining the multi-client and federation goals.

### Server-rendered HTML application

- **ALT-003**: **Description**: Render HTML server-side and treat the server as a web
  application rather than an API.
- **ALT-004**: **Rejection Reason**: Hostile to non-browser clients (automation,
  native apps, peers) and at odds with API-first and federation.

## Implementation Notes

- **IMP-001**: Response envelope and HTTP helpers live in `api/` (`envelope.go`,
  `http.go`, `middleware.go`); modules contribute handlers via `app.Module`.
- **IMP-002**: `/healthz` and `/readiness` are reserved system paths and may not be
  used as module mount points (`app/app.go` `isReservedPath`).
- **IMP-003**: Success criterion — the server runs and is fully exercisable with no UI
  assets present, via HTTP alone.

## References

- **REF-001**: `assets/high-level-concepts-0.1.png` — the defining data-flow
  architecture, which frames the server as a shared knowledge engine consumed by many
  surfaces rather than a single application with one front end.
- **REF-002**: `api/`, `app/` — the response envelope, HTTP helpers, and module
  framework that realise the single versioned HTTP contract.

> This is the root of the dependency order: every later record builds on the API-first
> stance and points back to this ADR rather than the reverse.
