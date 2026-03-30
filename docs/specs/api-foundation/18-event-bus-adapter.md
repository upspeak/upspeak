# Event Bus: NATS Isolation Strategy

## Decision

Keep NATS/JetStream as the event bus. No abstraction layer. Instead, isolate all NATS code in a dedicated module so it can be removed or replaced if needed.

## Rationale

The spec relies heavily on JetStream-specific features (durable consumers, stream retention, work queues, replay). Abstracting these behind a generic interface would either lose features or leak the abstraction — added complexity with no immediate benefit.

Instead, the NATS code is contained in a single module (`natsbus/` or similar) that:
- Owns all NATS connection, server, and JetStream management
- Exposes NATS-typed APIs to other modules (no pretence of being generic)
- Is the only package that imports `github.com/nats-io/*`

If NATS ever needs to be replaced, the blast radius is one module.

## Module Boundary

```
natsbus/
  natsbus.go         — embedded server startup, connection management
  streams.go         — JetStream stream lifecycle (create/delete per repo)
  consumers.go       — consumer creation for rules, connectors, realtime, sync, jobs
  publisher.go       — event publishing (wraps nats.Conn.Publish)
```

Other modules receive the NATS connection or publisher at init time via dependency injection (as the codebase already does with `app.Publisher`). They don't import NATS packages directly.

## What Changes from Current Code

The existing `app/nats.go` contains embedded NATS server startup and connection logic mixed into the `app` package. This moves to the dedicated `natsbus/` module. The `app` package becomes NATS-unaware — it receives a publisher and passes it to modules.

## Alternatives Considered

| Approach | Verdict |
|----------|---------|
| Generic broker interface (Watermill-style) | Rejected — loses JetStream features, adds complexity |
| Domain-level abstraction (Publisher + StreamManager + ConsumerFactory interfaces) | Rejected — added complexity without immediate benefit; premature abstraction |
| NATS code isolated in dedicated module (chosen) | Simple, no abstraction overhead, clear blast radius for future replacement |
| NATS code spread across modules (current state) | Only `app/nats.go` today, but would spread as modules grow — isolate now |
