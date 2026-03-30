# Schedules API

Schedules are cron-based jobs. They're a first-class resource so users can inspect, pause, and manage them.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/schedules` | Create schedule |
| `GET` | `/api/v1/schedules` | List schedules |
| `GET` | `/api/v1/schedules/{sched_ref}` | Get schedule details + next run time |
| `PUT` | `/api/v1/schedules/{sched_ref}` | Update schedule |
| `DELETE` | `/api/v1/schedules/{sched_ref}` | Delete schedule |
| `POST` | `/api/v1/schedules/{sched_ref}/trigger` | Trigger immediate run |
| `POST` | `/api/v1/schedules/{sched_ref}/pause` | Pause schedule |
| `POST` | `/api/v1/schedules/{sched_ref}/resume` | Resume schedule |
| `GET` | `/api/v1/schedules/{sched_ref}/history` | Run history |

Schedule-specific query params: `?repo_id={repo_ref}`, `?enabled=true|false`, `?action_type={type}`

## Schedule Payload

```json
{
  "name": "Fetch HN every 30 min",
  "cron": "*/30 * * * *",
  "action": {
    "type": "collect",
    "source_id": "SRC-5",
    "repo_id": "research"
  },
  "enabled": true
}
```

## Action Types

| Action type | Required fields | Purpose |
|-------------|----------------|---------|
| `collect` | `source_id`, `repo_id` | Fetch from a source on schedule |
| `publish` | `sink_id`, `repo_id`, `params.thread_id` or `params.node_id` | Push to a sink on schedule |
| `webhook` | `params.url`, `params.method` | Call an arbitrary endpoint |

## Response

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "SCHED-4",
    "name": "Fetch HN every 30 min",
    "cron": "*/30 * * * *",
    "action": { "type": "collect", "source_id": "SRC-5", "repo_id": "research" },
    "enabled": true,
    "next_run": "2026-03-30T10:30:00Z",
    "last_run": {
      "at": "2026-03-30T10:00:00Z",
      "result": "success",
      "duration_ms": 820
    },
    "created_at": "2026-03-30T09:00:00Z"
  }
}
```

## Notes

- Schedules are global (not repo-scoped) but reference repos in their actions
- Pausing a schedule stops future runs; resuming recalculates the next run time
- Triggering a schedule creates a job and runs the action immediately
- Cron expressions use standard 5-field syntax (minute, hour, day-of-month, month, day-of-week)

## Idempotent Pause/Resume

Pausing an already-paused schedule returns `200 OK` (no-op). Resuming an already-active schedule also returns `200 OK` (no-op). In both cases, the response body contains the current schedule state.

## Cross-Repo Validation

Schedule actions that reference a `source_id` or `sink_id` must refer to entities that exist within the specified `repo_id`. The server validates these references on both create and update. If the referenced source or sink does not exist in the given repo, the server returns `422 Unprocessable Entity`:

```json
{
  "error": {
    "code": "invalid_reference",
    "message": "Source SRC-5 does not exist in repo 'research'"
  }
}
```
