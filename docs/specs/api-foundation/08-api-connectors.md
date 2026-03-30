# Connectors API (Sources, Sinks, Collect)

## Sources

Sources are where content comes from. Each has a connector type, optional filters, and rate-limit configuration.

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/sources` | Register a source |
| `GET` | `/api/v1/repos/{repo_ref}/sources` | List sources |

Source-specific query params: `?connector={type}`, `?status=active|paused|error|rate_limited`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{source_ref}` | Get source details, status, rate-limit state |
| `PUT` | `/api/v1/repos/{repo_ref}/{source_ref}` | Update source config |
| `DELETE` | `/api/v1/repos/{repo_ref}/{source_ref}` | Remove source |
| `POST` | `/api/v1/repos/{repo_ref}/{source_ref}/collect` | Trigger immediate collection |
| `GET` | `/api/v1/repos/{repo_ref}/{source_ref}/history` | Collection history |

### Source Payload

```json
{
  "name": "HN Front Page",
  "connector": "rss",
  "config": { "url": "https://hnrss.org/frontpage", "format": "atom" },
  "filter_ids": ["FILTER-1"],
  "filter_chain_mode": "all",
  "rate_limit": {
    "max_requests": 60,
    "window_seconds": 3600,
    "retry_after_seconds": 120
  }
}
```

### Source Status Response

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "SRC-5",
    "name": "HN Front Page",
    "status": "active",
    "last_collection": {
      "at": "2026-03-30T10:00:00Z",
      "result": "success",
      "nodes_created": 12,
      "error_message": null
    },
    "rate_limit_state": {
      "remaining": 45,
      "resets_at": "2026-03-30T11:00:00Z"
    }
  }
}
```

## Sinks

Sinks are where content goes to. They share the same structure as sources (connector type, filters, rate limits).

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/sinks` | Register a sink |
| `GET` | `/api/v1/repos/{repo_ref}/sinks` | List sinks |

Sink-specific query params: `?connector={type}`, `?status=active|paused|error|rate_limited`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{sink_ref}` | Get sink details, status, rate-limit state |
| `PUT` | `/api/v1/repos/{repo_ref}/{sink_ref}` | Update sink config |
| `DELETE` | `/api/v1/repos/{repo_ref}/{sink_ref}` | Remove sink |
| `POST` | `/api/v1/repos/{repo_ref}/{sink_ref}/publish` | Trigger immediate publish |
| `GET` | `/api/v1/repos/{repo_ref}/{sink_ref}/history` | Publish history |

### Publish Payload

```json
{
  "node_id": "NODE-42",
  "thread_id": "THREAD-7",
  "dry_run": false
}
```

One of `node_id` or `thread_id` required.

### Publish Response

For small or synchronous operations, the server returns `200 OK`:

```json
{
  "data": {
    "result": "success",
    "published_at": "2026-03-30T10:05:00Z",
    "external_url": "https://mastodon.social/@user/123456"
  }
}
```

For large or long-running operations, the server returns `202 Accepted` with a job ID:

```json
{
  "data": {
    "job_id": "01964d2e-...",
    "status": "pending",
    "message": "Publish operation has been queued"
  }
}
```

### Dry-Run Response

When `"dry_run": true` is set in the publish payload, no data is actually published. The server returns a preview:

```json
{
  "data": {
    "dry_run": true,
    "would_publish": { "nodes": 5, "edges": 3 },
    "preview": "...",
    "destination": "mastodon.social"
  }
}
```

## History Response

The history endpoints for both sources (`GET .../history`) and sinks (`GET .../history`) return the same shape:

```json
{
  "data": [
    {
      "id": "01964d2e-...",
      "at": "2026-03-30T10:00:00Z",
      "result": "success|partial|error",
      "details": { "nodes_created": 12 },
      "error_message": null,
      "duration_ms": 1250
    }
  ],
  "meta": { "total": 45, "limit": 20, "offset": 0 }
}
```

## Connector Types

| Connector | Source behaviour | Sink behaviour |
|-----------|----------------|----------------|
| `rss` | Fetch and parse RSS/Atom feeds | Generate RSS/Atom feed |
| `discourse` | Fetch from Discourse API | Post to Discourse |
| `matrix` | Read from Matrix rooms | Send to Matrix rooms |
| `fediverse` | Follow ActivityPub actors | Publish as ActivityPub |
| `webhook` | Receive via webhook (incoming) | Call webhook (outgoing) |
| `email` | Read from mailbox (IMAP/JMAP) | Send email (SMTP/API) |
| `webpage` | Fetch and parse a single web page (one-shot collect) | -- |
| `repo` | Subscribe to another local repo's events | Push to another local repo |
| `upspeak` | Subscribe to a remote Upspeak instance | Push to a remote Upspeak instance |

### Repo Connector & Cycle Detection

The `repo` connector allows repo chaining — one repo as a source or sink for another:

```json
{
  "name": "AI articles from Raw",
  "connector": "repo",
  "config": { "repo_id": "raw-collection" },
  "filter_ids": ["FILTER-1"],
  "filter_chain_mode": "all"
}
```

Cycle detection applies to both `repo` sources AND `repo` sinks. Creating a source or sink that would form a circular repo chain returns `409 Conflict`:

```json
{
  "error": {
    "code": "circular_chain",
    "message": "This would create a circular chain: newsletter -> curated -> raw -> newsletter"
  }
}
```

## Collect (One-Shot Ingestion)

A domain operation for ad-hoc collection not tied to a pre-registered source.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/collect` | Collect from a URL or remote resource |

```json
{
  "source_url": "https://example.com/article.html",
  "mode": "import|follow",
  "connector": "rss|webpage|upspeak",
  "config": {}
}
```

- `mode: "import"` — one-time copy into the repo
- `mode: "follow"` — creates a source and tracks updates

Returns `202 Accepted` with a job ID.

## Rate Limiting

Rate limits are configured per source/sink, not globally:

```json
{
  "rate_limit": {
    "max_requests": 60,
    "window_seconds": 3600,
    "retry_after_seconds": 120
  }
}
```

When a source/sink hits its rate limit, its status changes to `rate_limited` and the `rate_limit_state` shows when it resets. Scheduled collections are deferred until the window resets.

## Secret Management

Config fields containing credentials (API keys, tokens, passwords) are stored encrypted at rest. `GET` responses redact secrets, showing only the last 4 characters:

```json
{
  "config": {
    "url": "https://discourse.example.com",
    "api_key": "****5678"
  }
}
```

To update a secret, provide the new value in `PUT`. To keep the existing value, omit the field from the request body. Providing the redacted value (e.g., `"****5678"`) is treated as an error and returns `422 Unprocessable Entity`.
