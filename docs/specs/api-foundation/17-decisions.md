# Design Decisions

1. **UUID v7** — Time-ordered UUIDs for all entities. Provides natural sort order, efficient B-tree indexing, and extractable timestamps. Replaces xid.ID from the existing codebase.

2. **Entity-type short IDs** — `NODE-42`, `EDGE-15`, `ANNO-23` etc. Per-repo sequences scoped by entity type. Immutable, never reused. Monotonically increasing sequences.

3. **Flat entity URLs** — Individual entities accessed at `/api/v1/repos/{repo_ref}/{entity_ref}` since the short ID prefix encodes the type. Collection operations still use typed paths (`/nodes`, `/edges`). Reserved path segments prevent routing ambiguity.

4. **Renameable repo slugs with redirects** — Old slugs permanently redirect (301) to the new slug. Old slugs cannot be reused.

5. **Hybrid architecture** — Synchronous writes to archive, JetStream for downstream events. The write path is confirmed; JetStream carries consequences.

6. **Filters as first-class resources** — Reusable condition sets referenced by ID from sources, sinks, and rules. Shared condition language with typed operators and field paths. Updates propagate immediately.

7. **Repo chaining** — Repos can be sources/sinks for other repos via the `repo` connector. Cycle detection prevents circular chains for both sources and sinks.

8. **PATCH for partial updates** — Nodes and repos support both PUT (full replace) and PATCH (merge metadata, update individual fields). Metadata deletion via null values.

9. **Batch operations** — Nodes and edges support atomic batch creation. Entire batch fails on any validation error (no partial creates).

10. **Jobs for async operations** — Collection, publish, sync, and repo deletion return 202 Accepted with a trackable job ID.

11. **Rate limiting as configuration** — External API rate limits are managed per source/sink, not globally.

12. **Filter chain mode** — When multiple filters are referenced, `filter_chain_mode` controls whether all must pass (AND) or any must pass (OR).

13. **API-first, no bundled UI** — Upspeak is a pure API server. Clients are separate projects. The existing `ui/` module is removed.

14. **Local-first with background processing** — Offline writes succeed immediately. The system runs autonomously (fetching, processing, applying rules) and presents results when the user reconnects.

15. **Optimistic concurrency** — All entities carry a `version` field (integer, starts at 1). ETag/If-Match headers enable conditional updates. Prevents silent overwrites in multi-device scenarios.

16. **Cascading deletes** — Node deletion cascades to edges, annotations, and thread membership (via JetStream consumers). Thread deletion removes structure but preserves contained nodes (except root). Repo deletion is async.

17. **Secret management** — Source/sink config credentials are encrypted at rest, redacted in GET responses (last 4 chars shown). Redacted values cannot be submitted as updates.

18. **W3C annotation selectors** — Annotations support TextQuoteSelector, TextPositionSelector, and FragmentSelector for targeting specific content ranges.

19. **Thread/Annotation as first-class entities** — Both have their own UUID and short ID, independent of their embedded Node. This enables direct referencing and querying.

20. **Incremental sync with tombstones** — Sync exchanges only events since last sync. Deleted entities tracked as tombstones (90-day retention). Version numbers detect conflicts; LWW is the default resolution.

21. **Administrative events** — All CRUD operations on Sources, Sinks, Filters, Rules, Schedules, and Repos emit events, enabling rules to react to administrative changes.

22. **JetStream subject migration** — New canonical format is `repo.{repo_id}.events.{EventType}` (singular, event-type segment). Dual-publish during transition from existing `repos.{id}.in/out` pattern.

23. **NATS isolation, no abstraction** — All NATS code lives in a dedicated `natsbus/` module. No generic broker abstraction — JetStream features are used directly. Other modules don't import NATS packages. If NATS ever needs replacing, the blast radius is one module. See [Event Bus: NATS Isolation Strategy](18-event-bus-adapter.md).
