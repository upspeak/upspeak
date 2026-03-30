# Identity Scheme

## UUID v7

All entities use UUID v7 as their primary key.

- **Time-ordered:** natural chronological sorting
- **Database-friendly:** sequential inserts for B-tree indices
- **Timestamp-extractable:** millisecond precision creation time
- **Globally unique:** no coordination needed across instances

## Short IDs

Every entity has a human-friendly short ID: `{ENTITY_PREFIX}-{SEQUENCE}`.

| Entity | Prefix | Example | Sequence scope |
|--------|--------|---------|---------------|
| Repository | `REPO` | `REPO-1` | Per user |
| Node | `NODE` | `NODE-42` | Per repo |
| Edge | `EDGE` | `EDGE-15` | Per repo |
| Thread | `THREAD` | `THREAD-7` | Per repo |
| Annotation | `ANNO` | `ANNO-23` | Per repo |
| Filter | `FILTER` | `FILTER-3` | Per repo |
| Source | `SRC` | `SRC-5` | Per repo |
| Sink | `SINK` | `SINK-2` | Per repo |
| Rule | `RULE` | `RULE-8` | Per repo |
| Schedule | `SCHED` | `SCHED-4` | Global |
| Job | `JOB` | `JOB-109` | Global |

**Rules:**
- Format: `{PREFIX}-{SEQ}` where SEQ is a positive integer, no zero-padding
- Generated server-side on creation, never client-provided
- Unique within their scope (repo or global), within their entity type
- Immutable — never reused even after deletion
- Sequences are monotonically increasing and never decremented. Deleted entity short IDs are never reused.

## Repository Slugs

Repos have a human-friendly `slug` scoped to the owning user.

- Lowercase alphanumeric + hyphens: `[a-z0-9][a-z0-9-]*`
- Max 32 characters
- Unique per user (not globally)
- **Renameable** — renaming creates a permanent redirect from old slug to new slug
- Redirects respond with `301 Moved Permanently` + `Location` header
- Old slugs cannot be reused (prevented by redirect table)

## URL Resolution

Any URL segment that accepts an entity reference resolves in order:

1. Try as UUID
2. Try as short ID (e.g., `NODE-42`) — the prefix determines entity type
3. Try as slug (repos only)
4. Check redirect table (repos only, return 301)
5. Return 404

### Flat Entity URLs

Since the short ID prefix encodes the entity type, individual entity operations use a flat URL under the repo:

```
GET    /api/v1/repos/research/NODE-42          — get node
PUT    /api/v1/repos/research/EDGE-15          — update edge
DELETE /api/v1/repos/research/ANNO-23          — delete annotation
GET    /api/v1/repos/research/NODE-42/edges    — sub-resource
GET    /api/v1/repos/research/{uuid}           — server resolves type by lookup
```

Collection operations still use typed paths:

```
GET    /api/v1/repos/research/nodes            — list nodes
POST   /api/v1/repos/research/nodes            — create node
POST   /api/v1/repos/research/nodes/batch      — batch create
```

### Full URL Examples

All of these resolve to the same node:

```
GET /api/v1/repos/research/NODE-42
GET /api/v1/repos/research/01964d2e-7c00-7000-8000-000000000042
GET /api/v1/repos/REPO-1/NODE-42
GET /api/v1/repos/01964d2e-7c00-7000-8000-000000000001/NODE-42
```

## Flat URL Routing

The flat URL scheme requires the router to disambiguate between reserved collection/action names and entity references. The resolution process works as follows:

1. **Reserved name check:** The router first checks if the path segment matches a reserved collection name (e.g., `nodes`, `edges`, `threads`, `annotations`, `filters`, `sources`, `sinks`, `rules`) or a reserved action name (e.g., `search`, `batch`, `publish`). If it matches, the request is routed to the corresponding collection or action handler. See the full list of reserved path segments in [API Conventions](./04-api-conventions.md#reserved-path-segments).

2. **Short ID prefix match:** If the segment is not a reserved name, the router attempts to parse it as a short ID (e.g., `NODE-42`, `EDGE-15`). The prefix determines the entity type and the corresponding table to query.

3. **UUID lookup:** If the segment is a valid UUID, the router searches across all entity tables for the current repo to find the matching entity. Tables are searched in the following order: **nodes, edges, threads, annotations, filters, sources, sinks, rules**. The first match is returned.

4. **404 Not Found:** If no match is found in any of the above steps, the router returns `404`.

This ordering ensures that reserved names always take precedence, preventing collisions between collection endpoints and any entity whose identifier might resemble a reserved word.
