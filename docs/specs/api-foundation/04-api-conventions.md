# API Conventions

## Response Envelope

Every response uses a consistent envelope:

```json
// Success (single resource)
{
  "data": { ... },
  "meta": { "request_id": "...", "timestamp": "..." }
}

// Success (collection)
{
  "data": [ ... ],
  "meta": { "request_id": "...", "total": 42, "limit": 20, "offset": 0 }
}

// Error
{
  "error": { "code": "not_found", "message": "Node not found" },
  "meta": { "request_id": "..." }
}

// Async accepted
{
  "data": { "job_id": "JOB-109", "status": "accepted" },
  "meta": { "request_id": "..." }
}
```

## URL Patterns

### Flat Entity Access

Individual entities are accessed directly under their repo using their short ID or UUID. The short ID prefix encodes the entity type, so no `/nodes/`, `/edges/` path segment is needed:

```
GET    /api/v1/repos/{repo_ref}/{entity_ref}              — get entity
PUT    /api/v1/repos/{repo_ref}/{entity_ref}              — replace entity
PATCH  /api/v1/repos/{repo_ref}/{entity_ref}              — partial update
DELETE /api/v1/repos/{repo_ref}/{entity_ref}              — delete entity
GET    /api/v1/repos/{repo_ref}/{entity_ref}/{sub}        — sub-resource
```

### Collection Access

List and create operations use typed collection paths:

```
GET    /api/v1/repos/{repo_ref}/nodes                     — list nodes
POST   /api/v1/repos/{repo_ref}/nodes                     — create node
POST   /api/v1/repos/{repo_ref}/nodes/batch               — batch create
```

### Reference Types

- `{repo_ref}` — UUID, short ID (`REPO-1`), or slug (`research`). Old slugs return 301.
- `{entity_ref}` — UUID or short ID (`NODE-42`, `EDGE-15`, `THREAD-7`, etc.)
- `{sched_ref}` — UUID or short ID (`SCHED-4`)
- `{job_ref}` — UUID or short ID (`JOB-109`)

## Reserved Path Segments

The following path segments are reserved collection or action names and cannot be used as entity refs in flat URLs. The router checks these first before attempting entity ref resolution:

**Collection names:** `nodes`, `edges`, `threads`, `annotations`, `filters`, `sources`, `sinks`, `rules`

**Action names:** `search`, `browse`, `graph`, `collect`, `batch`, `publish`, `history`, `test`, `trigger`, `pause`, `resume`

When a request arrives at `/api/v1/repos/{repo_ref}/{segment}`, the router:

1. Checks if `{segment}` matches a reserved path segment (case-sensitive).
2. If it matches, routes to the corresponding collection or action handler.
3. If it does not match, attempts entity ref resolution (short ID prefix match, then UUID lookup).

This means an entity short ID like `NODE-42` will never collide with a reserved name, but a hypothetical slug or identifier matching a reserved word would be unreachable via flat URL.

## Common Query Parameters

All list endpoints accept:

| Param | Purpose | Default |
|-------|---------|---------|
| `limit` | Page size | 20 (max 100) |
| `offset` | Pagination offset | 0 |
| `sort` | Field to sort by | `created_at` |
| `order` | `asc` or `desc` | `desc` |
| `created_after` | ISO 8601 timestamp lower bound | -- |
| `created_before` | ISO 8601 timestamp upper bound | -- |

## HTTP Status Codes

| Code | Usage |
|------|-------|
| `200` | Successful read or update |
| `201` | Resource created |
| `202` | Async operation accepted (returns job ID) |
| `204` | Successful delete (no body) |
| `301` | Slug redirect (renamed repo) |
| `400` | Invalid request (bad JSON, missing fields) |
| `401` | Unauthorized (missing or invalid authentication) |
| `403` | Forbidden (authenticated but insufficient permissions) |
| `404` | Resource not found |
| `409` | Conflict (circular repo chain, duplicate slug) |
| `412` | Precondition Failed (ETag mismatch on conditional update) |
| `422` | Unprocessable Entity (valid JSON but semantically invalid) |
| `429` | Rate limited |
| `500` | Internal server error |

## Optimistic Concurrency

All entity responses include an `ETag` header containing the entity's version number. This enables optimistic concurrency control for write operations.

- `PUT`, `PATCH`, and `DELETE` requests accept an `If-Match` header with the expected version.
- If the `If-Match` value does not match the entity's current `version` field, the server returns `412 Precondition Failed`.
- Clients should read the entity first, note the `ETag`, and include it in their update request.

**Example flow:**

```
GET /api/v1/repos/research/NODE-42
→ 200 OK
→ ETag: "3"

PATCH /api/v1/repos/research/NODE-42
If-Match: "3"
{ "subject": "Updated title" }
→ 200 OK
→ ETag: "4"

PATCH /api/v1/repos/research/NODE-42
If-Match: "3"
{ "subject": "Stale update" }
→ 412 Precondition Failed
```

## Query Parameter Combination

Entity-specific query parameters are ANDed with common query parameters. All specified filters must match for an entity to be included in the result set.

**Example:**

```
GET /api/v1/repos/research/NODE-42/edges?source=NODE-1&type=reply&sort=created_at&order=asc
```

This returns edges where `source` is `NODE-1` **and** `type` is `reply`, sorted by `created_at` in ascending order.

## Sortable Fields

Common sortable fields accepted by the `sort` query parameter:

| Field | Applicable to | Notes |
|-------|--------------|-------|
| `created_at` | All entities | Default sort field |
| `updated_at` | All entities | — |
| `type` | Nodes, Edges, Filters, Rules | Entity type or category |
| `subject` | Nodes | Node subject line |

Other sortable fields vary by entity type and are documented in the respective entity endpoint specifications. Requesting an unsupported sort field returns `400 Bad Request`.

## Request Limits

The following limits apply to all API requests:

| Limit | Value |
|-------|-------|
| Max request body size | 10 MB |
| Max batch size | 100 items |
| Max metadata entries per node | 50 |
| Max conditions per filter | 20 |
| Max actions per rule | 10 |

Exceeding these limits returns `400 Bad Request` with a descriptive error message.

## Pagination

The initial implementation uses **offset-based pagination** via the `limit` and `offset` query parameters (see Common Query Parameters above).

> **Future consideration:** Cursor-based pagination may be introduced for endpoints with frequently changing data sets or very large collections. Cursor-based pagination would use an opaque `cursor` parameter instead of `offset`, providing stable pagination over data that is being modified concurrently. When introduced, offset-based pagination will remain supported for backward compatibility.
