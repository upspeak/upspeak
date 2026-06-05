---
title: "ADR-0011: Real-time delivery via core-NATS fan-out with server-side filtering"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["realtime", "websocket", "nats"]
supersedes: ""
superseded_by: ""
---

# ADR-0011: Real-time delivery via core-NATS fan-out with server-side filtering

## Status

**Accepted**

## Context

Clients want to observe changes live — new nodes, edges, rule activity — over a
WebSocket, subscribing to channels scoped to a repository or entity. Each connection is
ephemeral: when a client disconnects, anything it missed is irrelevant (it will re-read
state on reconnect). The real-time path therefore has very different requirements from
the durable processors (rules, sync), which must never miss an event
([[adr-0009-global-repo-events-stream]]).

The question was how the real-time module should receive domain events and how channel
scoping should be enforced.

## Decision

Deliver real-time updates over **core-NATS fan-out**, not per-subscription JetStream
consumers, and **filter on the server side per connection**.

- The realtime module subscribes once to the wildcard subject **`repo.*.events.>`** over
  core NATS (`app.Subscriber`), via its `MsgHandler` (`realtime/realtime.go`).
- A **Hub** (`realtime/hub.go`), run as an `app.Runner`, receives every fanned-out event
  and dispatches it to matching connections. Events buffered before the hub starts are
  drained once it runs.
- Each connection declares channel subscriptions; **server-side matching**
  (`realtime/subscription.go`, `realtime/match.go`) decides which events that connection
  receives, so clients never see events outside their subscriptions.
- Authentication is currently an **allow-all seam** (`realtime/auth.go`,
  `Authenticator` interface; identity `"local"`), preserved for a real implementation
  later.

## Consequences

### Positive

- **POS-001**: No per-subscription JetStream consumer churn — one core subscription
  feeds all connections, so connecting/disconnecting clients are cheap.
- **POS-002**: Correct durability semantics by tool: ephemeral fan-out for live clients,
  durable JetStream for processors that must not miss events.
- **POS-003**: Server-side filtering keeps authorisation and scoping decisions on the
  server; clients cannot subscribe their way to events they shouldn't see.
- **POS-004**: The `Authenticator` seam means real auth is additive without reworking
  the data path.

### Negative

- **NEG-001**: Core-NATS delivery is at-most-once — a brief disconnect or hub backlog
  can drop events for that client; clients must re-sync state on reconnect rather than
  assume a gap-free stream.
- **NEG-002**: Every event fans out to the hub regardless of whether any connection wants
  it; filtering cost is paid centrally and scales with connection count.
- **NEG-003**: Allow-all auth is load-bearing today; until replaced, any client can open
  the WebSocket as identity `"local"`. Several spec channels
  (`…rules.{rule}.actions`, `jobs.{job}`, `sync`) are accepted but not yet backed by
  emitting events.

## Alternatives Considered

### Per-subscription JetStream consumers

- **ALT-001**: **Description**: Create a JetStream consumer per client subscription for
  durable delivery to WebSockets.
- **ALT-002**: **Rejection Reason**: Consumer lifecycle churn on every connect/disconnect
  and durability the use case does not need — a reconnecting client re-reads state
  anyway.

### Client-side filtering (push everything, let clients drop)

- **ALT-003**: **Description**: Send all events to every connection and filter in the
  client.
- **ALT-004**: **Rejection Reason**: Leaks events across scopes/authorisation boundaries
  and wastes bandwidth.

## Implementation Notes

- **IMP-001**: Module and hub in `realtime/realtime.go`, `realtime/hub.go`; connection
  handling in `realtime/connection.go`; channel matching in `realtime/subscription.go` +
  `realtime/match.go`; auth seam in `realtime/auth.go`. Endpoint: `GET /api/v1/ws`.
- **IMP-002**: Replace `allowAllAuthenticator` with real token/identity verification when
  auth lands (Phase 7+); wire the three unbacked channels to their events
  (`EventRuleTriggered` for rule actions; a `JobUpdated` event for jobs; Phase 6b events
  for sync).
- **IMP-003**: Clients must treat the stream as best-effort and reconcile via REST reads
  on reconnect.

## References

- **REF-001**: [[adr-0009-global-repo-events-stream]] — the durable counterpart; explains
  why realtime uses core fan-out instead.
- **REF-002**: [[adr-0008-hybrid-sync-write-async-events]] — the events being fanned out.
- **REF-003**: [[adr-0001-api-first-server]] — the WebSocket endpoint is part of the API
  surface.
