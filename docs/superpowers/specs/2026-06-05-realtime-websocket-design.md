# Realtime WebSocket Module — Design

**Date:** 2026-06-05
**Phase:** 6a (Real-time). Sync is deferred to a separate follow-up phase.
**Status:** Approved, pending implementation plan.

## Context

Phase 6 in the API foundation plan bundles two independent subsystems — real-time
event streaming (WebSocket) and multi-device sync. They have been split:

- **Realtime** fans out existing JetStream domain events to connected clients. It is
  self-contained and buildable now.
- **Sync** needs a federation/peer protocol and access control that the plan defers to
  Phase 7+ (see "Known Gap: Social Features and Federation" in
  `docs/superpowers/plans/2026-03-30-api-foundation.md`), and is therefore deferred to
  its own phase.

This document covers **realtime only**.

The spec source is `docs/specs/api-foundation/14-api-realtime.md`.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Phase scope | Realtime now; sync as a separate follow-up | Sync depends on deferred federation/auth; realtime delivers value immediately |
| Auth | Open now, design the seam | No auth exists anywhere yet; ship an `Authenticator` interface defaulting to allow-all so real auth drops in later without touching handlers |
| Channels | Implement all six spec channels; three are inert stubs | Full client-facing surface now; stub channels emit once their backing events exist |
| WebSocket library | `coder/websocket` | Modern, context-aware, minimal API, zero transitive deps, fits the std-`net/http` + context style of the codebase |

## Goal & Scope

A `realtime/` module exposing `GET /api/v1/ws`. A client opens one WebSocket
connection, sends `subscribe`/`unsubscribe` messages naming channels (with optional
filters), and receives a live push of matching domain events.

**In scope:** WebSocket lifecycle, channel subscription, server-side filtering,
connection management/limits, an auth seam.

**Out of scope this phase:** the sync module, peer exchange, tombstones, conflict
resolution, and real authentication.

## Architecture

### Event ingestion — one subscription, in-process fan-out

The module ingests events through a single `app.MsgHandler` on subject
`repo.*.events.>` (core-NATS fan-out, registered by the framework's existing
`SetSubscriber` path at `app/app.go:250`). No JetStream consumer is created and no
nats-io import is added — NATS isolation holds. The handler pushes each `core.Event`
into the Hub, which fans out to matching connections in-process.

```
NATS repo.*.events.>  ──(app.Subscriber)──▶  Hub.dispatch(event)
                                                  │  match each connection's
                                                  │  subscriptions + filters
                                                  ▼
                              conn.send(msg) → per-conn buffered chan → write loop → WS
```

The spec line "each WebSocket subscription maps to a JetStream consumer" is
deliberately **not** followed: a live socket wants the current tail, not durable
replay. One core-NATS fan-out subscription plus in-process filtering is the right tool
and leaks no consumers.

### The Hub

The Hub owns the connection registry and runs dispatch on its own goroutine.
Connections never touch each other's state. Register / unregister / dispatch flow
through the Hub via channels rather than shared-map locking in the hot path. The Hub is
started with `go realtimeModule.Run(ctx)` after module init.

## Module Structure

Follows the established `app.Module` + setter pattern.

```
realtime/
  realtime.go      — Module struct; Name/Init/HTTPHandlers/MsgHandlers; SetArchive, SetAuthenticator
  handlers.go      — GET /api/v1/ws upgrade handler; auth seam check
  hub.go           — Hub: connection registry + central event dispatch (one goroutine)
  connection.go    — per-client read loop, write loop, ping/pong, send buffer, limits
  subscription.go  — channel parsing/validation, subscription set, filter matching
  auth.go          — Authenticator interface + allowAllAuthenticator default
  *_test.go
```

## Channels & Subscription Model

The client `repo_ref` (slug / short ID / UUID) is resolved to a repo **UUID** at
subscribe time via `RefResolver`, so the module needs `SetArchive`. All six spec
channels are accepted; three are live and three are inert until later phases emit their
backing events.

| Channel | Backing | Status |
|---|---|---|
| `repos.{repo_ref}.events` | `repo.{uuid}.events.>` | live |
| `repos.{repo_ref}.nodes.{node_ref}` | repo events, payload node-ID match | live |
| `repos.{repo_ref}.threads.{thread_ref}` | repo events, payload thread-ID match | live |
| `repos.{repo_ref}.rules.{rule_ref}.actions` | no `RuleFired` event yet | stub |
| `jobs.{job_ref}` | no `JobUpdated` event yet | stub |
| `sync` | no sync module yet | stub |

Stub channels validate and accept the subscription but never emit. Each is marked with
a `TODO(phase-N)` so the later phase only needs to start publishing the event.

## Server-Side Filtering

Three filter levels, applied in the Hub before enqueueing to a connection:

1. **Channel scoping** — repo UUID match; for node/thread channels, extract the entity
   ID from the heterogeneous payload structs in `core/events.go` (create payloads carry
   a full entity; update/patch/delete carry only IDs; thread-node payloads carry both
   thread and node IDs) and compare.
2. **`event_type`** filter — always reliable; it is on the `core.Event` envelope.
3. **`node_type`** filter — best-effort: only create payloads carry the full `Node`;
   update/patch/delete carry only IDs. Documented behaviour: `node_type` matches only
   where the payload includes the node, rather than silently dropping delete events.

## Connection Management

Per spec §59-65:

- Server sends `ping` every 30s; a connection failing 3 consecutive pings is
  terminated. `coder/websocket`'s `Ping(ctx)` plus a read deadline handles this.
- **10 subscriptions per connection.**
- **1000-message send buffer per connection.** On overflow, drop the oldest message and
  send one `messages_dropped` notification.
- **5 connections per user** — enforced as a per-`Authenticator`-identity counter in the
  Hub. Under allow-all auth there is one synthetic identity, so this is effectively a
  global cap of 5 until real identities exist. This collapse is intentional and
  documented.
- Graceful close on context cancellation.

### Message shapes

Client → server: `subscribe` / `unsubscribe` (per spec §7-28).
Server → client event envelope per spec §30-42.
Errors use the spec `{ "type": "error", "code": ..., "message": ... }` shape with codes
`invalid_channel | subscription_limit | authentication_failed`.

## Auth Seam

```go
// Authenticator authorises a WebSocket upgrade. The default permits every
// request; real token/identity checks replace it in a later phase.
type Authenticator interface {
    Authenticate(r *http.Request) (identity string, err error)
}
```

`allowAllAuthenticator` returns a fixed identity and a nil error. The upgrade handler
calls `Authenticate` before `Accept`; a non-nil error becomes a `401` *before* the
WebSocket handshake completes. The authenticator is injected via `SetAuthenticator` and
defaults to allow-all in `Init`. When real auth lands it drops in here, and the
per-user connection cap immediately becomes meaningful.

## Wiring (`main.go`)

```go
realtimeModule := &realtime.Module{}
up.AddModuleOnPath(realtimeModule, "/api/v1")   // contributes the repo.*.events.> MsgHandler
// after InitModules:
realtimeModule.SetArchive(a)                      // for ref resolution
go realtimeModule.Run(runnerCtx)                  // Hub dispatch loop
```

The MsgHandler is registered by the framework's existing `SetSubscriber` path. No new
stream and no new consumer are added to `nats/`.

## Testing

- `httptest.Server` plus a real `coder/websocket` client: subscribe, publish a
  `core.Event` through a fake `Subscriber`, assert delivery.
- Table-driven coverage: channel parsing/validation; filter matching (event_type,
  node_type best-effort, node/thread ID extraction); subscription-limit and
  buffer-overflow / `messages_dropped`; auth-seam reject path.
- No live NATS required — the Hub is driven with an in-memory event feed.

## Explicitly Deferred

- Sync module, peers, tombstones, conflict resolution → next phase.
- Real authentication, per-user identity → its owning phase (the seam is ready).
- Backing events for the three stub channels (`RuleFired`, `JobUpdated`, sync events) →
  their owning phases.
