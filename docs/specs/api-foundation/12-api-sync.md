# Sync API

Handles multi-device synchronisation, conflict resolution, and peer management.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/sync/status` | Overall sync state |
| `POST` | `/api/v1/sync/trigger` | Trigger immediate sync |
| `GET` | `/api/v1/sync/conflicts` | List unresolved conflicts |
| `GET` | `/api/v1/sync/conflicts/{id}` | Get conflict details (local vs remote versions) |
| `POST` | `/api/v1/sync/conflicts/{id}/resolve` | Resolve a conflict |
| `GET` | `/api/v1/sync/peers` | List known sync peers |
| `POST` | `/api/v1/sync/peers` | Register a sync peer |
| `DELETE` | `/api/v1/sync/peers/{id}` | Remove a sync peer |

## Sync Status

```json
{
  "data": {
    "local_pending": 3,
    "remote_pending": 47,
    "last_sync": "2026-03-29T14:00:00Z",
    "conflicts": 0,
    "state": "synced|syncing|offline|error"
  }
}
```

## Conflict Resolution

Conflicts can occur on any entity (nodes, edges, threads, annotations). The conflict detail uses generic entity references:

```json
// GET /api/v1/sync/conflicts/{id}
{
  "data": {
    "id": "conflict_001",
    "entity_type": "node|edge|thread|annotation",
    "entity_id": "NODE-42",
    "local_version": { ... },
    "remote_version": { ... },
    "type": "concurrent_edit|delete_conflict",
    "detected_at": "2026-03-30T10:00:00Z"
  }
}

// POST /api/v1/sync/conflicts/{id}/resolve
{
  "resolution": "keep_local|keep_remote|merge",
  "merged_entity": {}
}
```

`merged_entity` only required when `resolution` is `merge`. The shape of `merged_entity` must match the `entity_type` of the conflict.

The conflicts list endpoint supports filtering by repository: `GET /api/v1/sync/conflicts?repo_id={repo_ref}`

## Peers

```json
// POST /api/v1/sync/peers
{
  "name": "Home Server",
  "url": "https://home.example.com",
  "auth": { "type": "token", "token": "..." }
}
```

## Sync Mechanism

- Sync is **incremental** — only events since last sync are exchanged
- Deleted entities are tracked via **tombstones** (a record of deletion with timestamp) retained for 90 days
- Conflict detection uses **version numbers** — if two peers have different versions of the same entity, a conflict is raised
- Default resolution is **last-write-wins (LWW)** based on `updated_at`; users can override per-conflict
- Schema versioning: peers exchange their API version during handshake; incompatible versions refuse to sync

## Peer Health Monitoring

Unreachable peers are marked `status: "unreachable"` after 3 failed attempts. Sync retries with exponential backoff.

## Notes

- Sync is powered by JetStream stream replication
- Offline edits are queued locally and synced on reconnection
- Conflicts arise from concurrent edits to the same entity on different devices/instances
- The sync module emits `SyncCompleted` and `ConflictDetected` events
