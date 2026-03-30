# Search & Browse API

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/search` | Full-text search with filters |
| `GET` | `/api/v1/repos/{repo_ref}/browse` | Browse feed (recent, filtered, prioritised) |
| `GET` | `/api/v1/repos/{repo_ref}/graph` | Graph traversal from a node |

## Search

```json
// POST /api/v1/repos/research/search
{
  "query": "EU AI Act",
  "filters": {
    "type": ["article", "annotation"],
    "created_after": "2026-01-01T00:00:00Z",
    "created_before": "2026-04-01T00:00:00Z",
    "has_edge_type": "categorised",
    "metadata": { "priority": "high" }
  },
  "limit": 20,
  "offset": 0
}
```

Returns nodes matching the query, ranked by relevance.

### Search Response

```json
{
  "data": [
    {
      "node": { "id": "...", "short_id": "NODE-42", "type": "article", "subject": "..." },
      "score": 0.95,
      "highlights": [ { "field": "subject", "snippet": "EU <em>AI Act</em> passed" } ]
    }
  ],
  "meta": { "request_id": "...", "total": 12, "limit": 20, "offset": 0 }
}
```

Each result includes a `score` (0.0–1.0) indicating relevance and an optional `highlights` array showing which fields matched with surrounding context.

> **Note:** Search filters and saved Filters (from the Filters API) are different. Search uses an inline `filters` object for ad-hoc queries. A future enhancement may allow referencing saved Filter IDs in search requests via a `filter_ids` field.

## Browse

A feed of content, ordered by recency, with optional type and source filtering.

Query params: `?type={type}`, `?source_id={source_ref}`, plus common params (limit, offset, sort, order, created_after, created_before)

```
GET /api/v1/repos/research/browse?type=article&limit=20
```

### Browse Response

```json
{
  "data": [
    { "id": "...", "short_id": "NODE-55", "type": "article", "subject": "...", "created_at": "..." },
    { "id": "...", "short_id": "NODE-54", "type": "bookmark", "subject": "...", "created_at": "..." }
  ],
  "meta": { "request_id": "...", "total": 87, "limit": 20, "offset": 0 }
}
```

The browse response uses the standard collection envelope. Each entry is a node object. Results are ordered by `created_at` descending by default.

## Graph Traversal

Explores the knowledge graph outward from a starting node.

Query params: `?node_id={node_ref}`, `?depth={1-5}`, `?edge_type={type}`, `?direction=outgoing|incoming|both`

```
GET /api/v1/repos/research/graph?node_id=NODE-42&depth=2&direction=outgoing
```

Response:

```json
{
  "data": {
    "root": { "id": "...", "short_id": "NODE-42", "type": "article", "subject": "..." },
    "nodes": [ ... ],
    "edges": [ ... ]
  }
}
```

Nodes and edges are returned as flat arrays. The client reconstructs the graph from the edge source/target references.
