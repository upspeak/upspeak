# Rules API

Rules are event-condition-action triggers: "When X happens, if Y is true, do Z."

## Endpoints

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/rules` | Create rule |
| `GET` | `/api/v1/repos/{repo_ref}/rules` | List rules |

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{rule_ref}` | Get rule details |
| `PUT` | `/api/v1/repos/{repo_ref}/{rule_ref}` | Update rule |
| `DELETE` | `/api/v1/repos/{repo_ref}/{rule_ref}` | Delete rule |
| `POST` | `/api/v1/repos/{repo_ref}/{rule_ref}/test` | Dry-run against recent events |
| `POST` | `/api/v1/repos/{repo_ref}/{rule_ref}/pause` | Pause rule |
| `POST` | `/api/v1/repos/{repo_ref}/{rule_ref}/resume` | Resume rule |
| `GET` | `/api/v1/repos/{repo_ref}/{rule_ref}/history` | Action history |

## Rule Payload

```json
{
  "name": "Auto-tag AI governance articles",
  "trigger": {
    "event": "NodeCreated",
    "filter_ids": ["FILTER-1"]
  },
  "actions": [
    { "type": "enrich", "params": { "metadata_key": "priority", "metadata_value": "high" } }
  ],
  "enabled": true
}
```

## Trigger Events

`NodeCreated`, `NodeUpdated`, `NodePatched`, `NodeDeleted`, `EdgeCreated`, `EdgeUpdated`, `EdgeDeleted`, `ThreadCreated`, `ThreadUpdated`, `ThreadDeleted`, `ThreadNodeAdded`, `ThreadNodeRemoved`, `AnnotationCreated`, `AnnotationUpdated`, `AnnotationDeleted`, `CollectionCompleted`, `PublishCompleted`

## Action Types

| Action type | Params | Purpose |
|-------------|--------|---------|
| `enrich` | `metadata_key`, `metadata_value` | Add/update metadata on the triggering node |
| `relate` | `target_thread_id`, `edge_type`, `target_node_id` | Create edge or add to thread |
| `annotate` | `motivation`, `body`, `content_type` | Create annotation on triggering node |
| `collect` | `source_id` | Trigger collection from a source |
| `publish` | `sink_id`, `thread_id` or `node_id` | Trigger publish to a sink |
| `webhook` | `url`, `method`, `headers`, `body_template` | Call external endpoint |

## Rule Test

Dry-run evaluates the rule against recent events without executing actions:

```json
// POST /api/v1/repos/research/RULE-8/test

// Response
{
  "data": {
    "events_evaluated": 25,
    "events_matched": 3,
    "matches": [
      { "event_id": "...", "event_type": "NodeCreated", "would_execute": ["enrich"] }
    ]
  }
}
```

## History Response

The history endpoint (`GET .../history`) returns execution records for the rule:

```json
{
  "data": [
    {
      "id": "01964d2e-...",
      "triggered_by_event": { "id": "...", "type": "NodeCreated" },
      "actions_executed": [ { "type": "enrich", "result": "success" } ],
      "at": "2026-03-30T10:00:00Z",
      "duration_ms": 45
    }
  ],
  "meta": { "total": 120, "limit": 20, "offset": 0 }
}
```

## Notes

- Rules are evaluated by the rules-engine JetStream consumer
- A rule can have multiple actions -- all execute when the trigger matches
- Actions that fail are logged in the rule's history but don't block other actions
- Paused rules are not evaluated
