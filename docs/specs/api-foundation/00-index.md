# Upspeak API Foundation Design

**Date:** 2026-03-30
**Status:** Draft — approved in conversation, pending implementation plan

This document defines the complete API surface, internal architecture, and design decisions for Upspeak's production-ready API foundation.

## Contents

1. **[Vision & Architecture](01-vision.md)** — Product vision, deployment modes, hybrid architecture (synchronous core + JetStream), NATS role
2. **[Identity Scheme](02-identity.md)** — UUID v7, entity-type short IDs, repo slugs with redirects, URL resolution
3. **[User Journeys](03-journeys.md)** — Seven user journeys that drive the API design
4. **[API Conventions](04-api-conventions.md)** — Response envelope, URL patterns, common query parameters, flat entity URLs
5. **[Repositories API](05-api-repos.md)** — Repo CRUD, slug management, redirects
6. **[Knowledge Graph API](06-api-knowledge.md)** — Nodes, Edges, Threads, Annotations — core primitives with flat URL access
7. **[Filters API](07-api-filters.md)** — Reusable condition sets, operators, field paths, testing
8. **[Connectors API](08-api-connectors.md)** — Sources, Sinks, one-shot Collect, connector types, rate limiting
9. **[Schedules API](09-api-schedules.md)** — Cron-based job scheduling, action types, pause/resume
10. **[Rules API](10-api-rules.md)** — Event-condition-action automation, trigger events, rule actions
11. **[Search & Browse API](11-api-search.md)** — Full-text search, browse feed, graph traversal
12. **[Sync API](12-api-sync.md)** — Multi-device sync, conflict resolution, peer management
13. **[Jobs API](13-api-jobs.md)** — Async operation tracking, cancellation
14. **[Real-time API](14-api-realtime.md)** — WebSocket channels, subscriptions, event filtering
15. **[Event Types](15-events.md)** — Complete event catalogue
16. **[Internal Architecture](16-architecture.md)** — Domain models, JetStream streams, Archive interface, module composition
17. **[Design Decisions](17-decisions.md)** — Summary of all architectural decisions with rationale
