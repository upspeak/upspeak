# Filters API

Filters are reusable, named condition sets. Sources, sinks, and rules reference them by ID rather than embedding conditions inline.

## Endpoints

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/filters` | Create filter |
| `GET` | `/api/v1/repos/{repo_ref}/filters` | List filters |

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{filter_ref}` | Get filter |
| `PUT` | `/api/v1/repos/{repo_ref}/{filter_ref}` | Update filter |
| `DELETE` | `/api/v1/repos/{repo_ref}/{filter_ref}` | Delete filter (fails if referenced) |
| `POST` | `/api/v1/repos/{repo_ref}/{filter_ref}/test` | Test filter against a sample payload |

## Filter Payload

```json
{
  "name": "AI governance articles",
  "description": "Matches articles related to AI governance and regulation",
  "mode": "all|any",
  "conditions": [
    { "field": "node.type", "op": "eq", "value": "article" },
    { "field": "node.subject", "op": "contains", "value": "AI" }
  ]
}
```

**Filter mode:**
- `"all"` — every condition must match (AND)
- `"any"` — at least one condition must match (OR)

## Condition Operators

| Operator | Meaning | Value type |
|----------|---------|-----------|
| `eq` | Equals | string, number, boolean |
| `neq` | Not equals | string, number, boolean |
| `contains` | String contains substring | string |
| `not_contains` | String does not contain | string |
| `starts_with` | String starts with | string |
| `ends_with` | String ends with | string |
| `in` | Value is one of | array |
| `not_in` | Value is not one of | array |
| `gt` | Greater than | number, timestamp |
| `lt` | Less than | number, timestamp |
| `gte` | Greater or equal | number, timestamp |
| `lte` | Less or equal | number, timestamp |
| `exists` | Field exists (value ignored) | -- |
| `not_exists` | Field does not exist (value ignored) | -- |
| `matches` | Regex match | string (regex pattern) |

## Condition Field Paths

| Path | Resolves to |
|------|------------|
| `node.id` | Node UUID |
| `node.short_id` | Node short ID |
| `node.type` | Node type |
| `node.subject` | Node subject |
| `node.content_type` | Node content type |
| `node.created_at` | Node creation timestamp |
| `node.metadata.{key}` | Value of metadata with given key |
| `edge.type` | Edge type |
| `edge.source` | Edge source node ref |
| `edge.target` | Edge target node ref |
| `edge.weight` | Edge weight |
| `annotation.motivation` | Annotation motivation |
| `source.connector` | Source connector type |

## Filter Test

```json
// POST /api/v1/repos/research/FILTER-3/test
{
  "node": {
    "type": "article",
    "subject": "New AI governance framework announced",
    "metadata": [{ "key": "priority", "value": "\"high\"" }]
  }
}

// Response
{
  "data": {
    "matches": true,
    "condition_results": [
      { "field": "node.type", "op": "eq", "result": true },
      { "field": "node.subject", "op": "contains", "result": true }
    ]
  }
}
```

## Deleting a Referenced Filter

Attempting to delete a filter that is still referenced by a source, sink, or rule returns `409 Conflict` with a list of the referencing resources:

```json
{
  "error": {
    "code": "filter_in_use",
    "message": "Cannot delete filter because it is referenced by other resources",
    "references": [
      { "type": "source", "id": "SRC-5", "name": "HN Front Page" },
      { "type": "rule", "id": "RULE-8", "name": "Auto-tag AI governance articles" }
    ]
  }
}
```

Remove the filter from all referencing resources before deleting it.

## Update Propagation

When a filter is updated, all sources, sinks, and rules referencing it automatically use the updated conditions on their next evaluation. There is no filter versioning -- updates take effect immediately.

## Referencing Filters

Sources and sinks reference filters:
```json
{
  "filter_ids": ["FILTER-1", "FILTER-2"],
  "filter_chain_mode": "all|any"
}
```

- `filter_ids` — ordered list of filter references
- `filter_chain_mode` — `"all"` (every filter must pass) or `"any"` (at least one must pass)

Rules reference filters in their trigger:
```json
{
  "trigger": {
    "event": "NodeCreated",
    "filter_ids": ["FILTER-1"]
  }
}
```
