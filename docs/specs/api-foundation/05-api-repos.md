# Repositories API

Repos are the top-level organising unit — a self-contained knowledge graph. A user might have "research", "work", "personal" repos.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos` | Create repository |
| `GET` | `/api/v1/repos` | List repositories |
| `GET` | `/api/v1/repos/{repo_ref}` | Get repository details |
| `PUT` | `/api/v1/repos/{repo_ref}` | Full update (all mutable fields required) |
| `PATCH` | `/api/v1/repos/{repo_ref}` | Partial update (only provided fields are changed) |
| `DELETE` | `/api/v1/repos/{repo_ref}` | Delete repository (async) |

`{repo_ref}` accepts UUID, short ID (`REPO-1`), or slug (`research`). Old slugs return `301 Moved Permanently`.

## Create Payload

```json
{
  "slug": "research",
  "name": "AI Governance Research",
  "description": "Research on AI governance frameworks and regulation"
}
```

## Response

```json
{
  "data": {
    "id": "01964d2e-7c00-7000-8000-000000000001",
    "short_id": "REPO-1",
    "slug": "research",
    "name": "AI Governance Research",
    "description": "Research on AI governance frameworks and regulation",
    "owner_id": "01964d2e-...",
    "version": 1,
    "created_at": "2026-03-30T10:00:00Z",
    "updated_at": "2026-03-30T10:00:00Z"
  }
}
```

## Immutable Fields

The fields `id`, `short_id`, `owner_id`, and `created_at` cannot be changed via `PUT` or `PATCH`. If these fields are provided in the request body, the server silently ignores them.

## Slug Rename

When updating a repo slug via `PUT` or `PATCH`, the old slug becomes a permanent redirect:

```json
// PUT /api/v1/repos/research
{ "slug": "ai-governance", "name": "AI Governance Research", "description": "..." }

// PATCH /api/v1/repos/research (only rename slug, other fields unchanged)
{ "slug": "ai-governance" }
```

After rename:
- `GET /api/v1/repos/ai-governance` — returns repo (200)
- `GET /api/v1/repos/research` — returns redirect (301) to `/api/v1/repos/ai-governance`
- The old slug `research` can never be reused by any repo for this user

## Delete Semantics

Deleting a repository is an asynchronous operation. The server returns `202 Accepted` with a job ID:

```json
// DELETE /api/v1/repos/research

// Response (202 Accepted)
{
  "data": {
    "job_id": "01964d2e-...",
    "status": "pending",
    "message": "Repository deletion has been queued"
  }
}
```

The deletion job removes all entities within the repo (nodes, edges, threads, annotations), tears down associated JetStream streams, and cancels any schedules referencing the repo.

**Warning:** Repository deletion is irreversible. Once the job completes, all data is permanently removed.

## ETag / Version

Responses include a `version` field (integer, incremented on every write). Clients should use this for optimistic concurrency control. To perform a conditional update, include the `If-Match` header with the current version:

```
PUT /api/v1/repos/research
If-Match: 3
```

If the version has changed since the client last read it, the server returns `412 Precondition Failed`. The `If-Match` header is optional — omitting it bypasses the version check.
