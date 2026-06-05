---
title: "ADR-0003: Modular monolith via the app micro-framework"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["modularity", "lifecycle"]
supersedes: ""
superseded_by: ""
---

# ADR-0003: Modular monolith via the app micro-framework

## Status

**Accepted**

## Context

Upspeak was conceived as a single binary hosting many cohesive features — repositories
and the knowledge graph, filters, jobs, connectors, schedules, rules, search, and
real-time delivery. Each needs its own HTTP surface, some need background loops, and
several must react to domain events. The project is personal-first, so the operational
cost of running many services is not justified, yet the codebase must keep clear seams
so a feature could be extracted later if needed.

Rather than letting these features accrete and then extracting a structure in retrospect,
the **`app` framework was designed first** as the foundation, and every feature was then
built as a module against it. This required a structure that gives strong internal
boundaries and a uniform lifecycle up front — without adopting microservices or a
heavyweight dependency-injection framework.

## Decision

Build a **modular monolith** around a tiny in-repo framework in `app/`:

- Each feature is an `app.Module` (`Name`, `Init`, `HTTPHandlers`, `MsgHandlers`).
  `HTTPHandlers()`/`MsgHandlers()` take no parameters; dependencies arrive via setter
  methods (`SetArchive`, `SetPublisher`, …).
- Long-lived background loops implement `app.Runner` (`Run(ctx)`) and are started
  uniformly from `main.go` via a `[]app.Runner`.
- Modules co-mount on a shared path (`/api/v1`); `http.ServeMux` resolves overlapping
  registrations by method+path specificity, so no third-party router is needed.
- The lifecycle is strict and explicit: `InitModules()` (Init + register handlers)
  → wire cross-module dependencies via setters → `Start()` (begin serving HTTP).

## Consequences

### Positive

- **POS-001**: Clear feature seams in a single binary; a module is a candidate unit for
  future extraction.
- **POS-002**: Background work has one uniform start/stop contract (`app.Runner` +
  context cancellation), so `main.go` treats every loop identically.
- **POS-003**: No router dependency — the standard library's `ServeMux` carries
  multi-module mounting.
- **POS-004**: Single-process deployment keeps operations trivial, fitting the
  personal-first goal.

### Negative

- **NEG-001**: Setter-based injection produces a **nil-until-wired** window: handlers
  are registered at `Init` but dependencies are set afterwards, so `main.go` must follow
  the exact `InitModules()` → wire → `Start()` order. This ordering contract cannot be
  enforced by the compiler and is a footgun for the next module author (the wiring
  order in `main.go`).
- **NEG-002**: No process-level isolation between modules; a panic or resource leak in
  one affects the whole binary.
- **NEG-003**: Readiness is decided by a 200 ms "no error yet" heuristic in `Start()`
  (`app/app.go`), so a failure surfacing after that window reports ready-then-dies.

## Alternatives Considered

### Constructor injection with strict construction ordering

- **ALT-001**: **Description**: Pass dependencies into module constructors so a module
  is never observable without them.
- **ALT-002**: **Rejection Reason**: The archive is itself an `app.Module` whose handle
  is obtained only after its own `Init`; constructor injection would force a brittle
  global construction order and complicate the archive-as-module pattern. Setters were
  chosen to decouple construction from dependency availability (the trade-off is
  NEG-001).

### Microservices

- **ALT-003**: **Description**: Split features into independently deployed services.
- **ALT-004**: **Rejection Reason**: Operational overhead (deployment, networking,
  observability) is unjustifiable for a personal-first, single-binary product.

### A dependency-injection framework

- **ALT-005**: **Description**: Adopt a DI/wiring library to manage the object graph.
- **ALT-006**: **Rejection Reason**: Heavier than warranted; a hand-written `main.go`
  keeps the wiring explicit and readable.

## Implementation Notes

- **IMP-001**: Framework lives in `app/app.go` (`Module`, `Runner`, `App`,
  `AddModuleOnPath`, `InitModules`, `Start`/`Stop`); wiring example in `main.go`.
- **IMP-002**: New modules must be added to `main.go` in the established order and wired
  *after* `InitModules()`; document any new cross-module dependency at the wiring site.
- **IMP-003**: Future hardening worth considering — replace the 200 ms readiness
  heuristic with an explicit listener-bound signal.

## References

- **REF-001**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — the
  ports modules depend on.
- **REF-002**: [[adr-0001-api-first-server]] — modules collectively form the HTTP API.
- **REF-003**: `app/README.md` — module interface documentation.
