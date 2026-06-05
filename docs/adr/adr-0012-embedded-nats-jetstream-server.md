---
title: "ADR-0012: Embedded NATS JetStream server"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["nats", "deployment"]
supersedes: ""
superseded_by: ""
---

# ADR-0012: Embedded NATS JetStream server

## Status

**Accepted**

## Context

Upspeak's event-driven core ([[adr-0008-hybrid-sync-write-async-events]],
[[adr-0009-global-repo-events-stream]]) depends on NATS JetStream for durable streams
and consumers. For a personal-first product, requiring the user to install, configure,
and operate a separate message broker before they can run the application would be a
significant adoption barrier and an operational burden disproportionate to a
single-user deployment.

A decision was needed on whether NATS runs as external infrastructure or inside the
Upspeak process.

## Decision

**Embed the NATS JetStream server in-process** by default, while keeping the option to
connect to an external broker.

- `nats.Start` reads `Config{URL, Embedded, Private, Logging}`. When `Embedded` is true
  it starts an in-process `server.Server` with JetStream and connects to it
  (`nats/nats.go` `startEmbeddedServer`, `connectToEmbedded`); otherwise it connects to
  the configured external `URL`.
- The `nats.Bus` exposes `Publisher`/`Subscriber`/`Consumer` to the rest of the app, so
  no other package knows or cares whether the server is embedded
  ([[adr-0002-hexagonal-architecture-infrastructure-isolation]]).
- Connection management is hardened for a long-running process: it drains in-flight work
  on shutdown rather than closing the connection outright, reconnects automatically and
  indefinitely, and logs disconnect/reconnect/error events via handler callbacks.

## Consequences

### Positive

- **POS-001**: Single-binary deployment — the user runs one process and gets durable
  messaging with no external broker to install or operate.
- **POS-002**: Local-first friendly: the broker lives with the app and its data, working
  fully offline.
- **POS-003**: The embedded-vs-external choice is config-only and invisible to every
  other package, so scaling out to an external/clustered NATS later needs no code
  changes outside `nats/`.
- **POS-004**: Connection hardening (draining on shutdown, indefinite automatic
  reconnection) suits a resident background service.

### Negative

- **NEG-001**: The broker shares the application's process and resources — JetStream
  memory/disk and a crash are coupled to the app lifecycle.
- **NEG-002**: No out-of-the-box horizontal scaling or high availability in embedded
  mode; that requires switching to an external/clustered deployment.
- **NEG-003**: JetStream storage lifecycle (limits, retention, disk location) becomes the
  application's operational responsibility rather than a dedicated broker's.

## Alternatives Considered

### Require an external NATS server

- **ALT-001**: **Description**: Treat NATS as external infrastructure the operator must
  provide.
- **ALT-002**: **Rejection Reason**: Heavy onboarding and operational burden for a
  single-user, personal-first product; defeats the single-binary goal. Still supported as
  an option for users who want it.

### A non-JetStream/in-memory event bus

- **ALT-003**: **Description**: Use an in-process channel/bus without NATS.
- **ALT-004**: **Rejection Reason**: Loses durable, at-least-once delivery and the
  stream/consumer model the rules engine and future sync rely on, and forecloses an easy
  path to an external broker later.

## Implementation Notes

- **IMP-001**: Implementation in `nats/nats.go` (`Start`, `startEmbeddedServer`,
  `connectToEmbedded`, `Stop`/drain); config surfaced via `app` config and
  `upspeak.sample.yaml`.
- **IMP-002**: To scale out, set `Embedded: false` and point `URL` at an external/
  clustered NATS — no changes required outside the `nats/` package.
- **IMP-003**: Operationally, monitor JetStream storage growth and configure stream
  retention/limits appropriately for the deployment.

## References

- **REF-001**: [[adr-0002-hexagonal-architecture-infrastructure-isolation]] — NATS is an
  isolated adapter, making embedded/external swappable.
- **REF-002**: [[adr-0009-global-repo-events-stream]] — the streams and consumers this
  server hosts.
- **REF-003**: [[adr-0008-hybrid-sync-write-async-events]] — why durable messaging is
  required at all.
