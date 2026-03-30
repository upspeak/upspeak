# Jobs API

Async operations (collection, publish, sync) return `202 Accepted` with a job ID. Jobs are trackable and cancellable.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/jobs` | List recent jobs |
| `GET` | `/api/v1/jobs/{job_ref}` | Get job status and result |
| `POST` | `/api/v1/jobs/{job_ref}/cancel` | Cancel a running job |

Job-specific query params: `?status=pending|running|completed|failed|cancelled`, `?type=collect|publish|sync`, `?repo_id={repo_ref}`

## Job Response

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "JOB-109",
    "repo_id": "01964d2e-...",
    "type": "collect",
    "status": "completed",
    "started_at": "2026-03-30T10:00:00Z",
    "completed_at": "2026-03-30T10:00:12Z",
    "result": { "nodes_created": 12 },
    "error": null
  }
}
```

## Job Statuses

| Status | Meaning |
|--------|---------|
| `pending` | Job created, waiting to be picked up |
| `running` | Job is being executed |
| `completed` | Job finished successfully |
| `failed` | Job finished with an error |
| `cancelled` | Job was cancelled before completion |

## Notes

- Jobs are processed by the job-runner JetStream consumer
- Job results vary by type (collect returns `nodes_created`, publish returns `items_published`, etc.)
- Cancelling a running job is best-effort — the job may complete before the cancellation takes effect
- Job records are retained for a configurable period (default: 30 days)
