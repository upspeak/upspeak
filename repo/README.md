# Repository Module

The `repo` module provides the HTTP API for managing repositories in Upspeak. It mounts at `/api/v1` and exposes CRUD endpoints for the `Repository` domain model.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos` | Create repository |
| `GET` | `/api/v1/repos` | List repositories (paginated) |
| `GET` | `/api/v1/repos/{repo_ref}` | Get repository (by UUID, short ID, or slug) |
| `PUT` | `/api/v1/repos/{repo_ref}` | Full update |
| `PATCH` | `/api/v1/repos/{repo_ref}` | Partial update |
| `DELETE` | `/api/v1/repos/{repo_ref}` | Delete repository |

## Reference Resolution

`{repo_ref}` accepts:
1. UUID (e.g. `01964d2e-7c00-7000-8000-000000000001`)
2. Short ID (e.g. `REPO-1`)
3. Slug (e.g. `research`)
4. Old slug (returns 301 redirect to current slug)

## Response Envelope

All responses use the standard envelope from the `api` package:

```json
{
  "data": { ... },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

## Optimistic Concurrency

- Responses include `ETag` header with the entity version
- `PUT`/`PATCH`/`DELETE` accept `If-Match` header for version checks
- Mismatched versions return `412 Precondition Failed`

## Slug Rename

When a slug is changed via `PUT` or `PATCH`, the old slug becomes a permanent redirect. Old slugs cannot be reused.

## Dependencies

The module receives its dependencies via setter methods:
- `SetArchive(archive core.Archive)` — Storage backend
- `SetPublisher(pub app.Publisher)` — Event publishing
