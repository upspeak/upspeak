# Event Types

All events are published to JetStream after the corresponding write is confirmed in the archive.

## Knowledge Graph Events

| Event | Emitted when |
|-------|-------------|
| `NodeCreated` | Node created |
| `NodeUpdated` | Node fully replaced (PUT) |
| `NodePatched` | Node partially updated (PATCH) |
| `NodeDeleted` | Node deleted |
| `EdgeCreated` | Edge created |
| `EdgeUpdated` | Edge updated |
| `EdgeDeleted` | Edge deleted |
| `ThreadCreated` | Thread created |
| `ThreadUpdated` | Thread metadata updated |
| `ThreadDeleted` | Thread deleted |
| `ThreadNodeAdded` | Node added to thread |
| `ThreadNodeRemoved` | Node removed from thread |
| `AnnotationCreated` | Annotation created |
| `AnnotationUpdated` | Annotation updated |
| `AnnotationDeleted` | Annotation deleted |

## Operational Events

| Event | Emitted when |
|-------|-------------|
| `CollectionCompleted` | Source collection finished (success or failure) |
| `PublishCompleted` | Sink publish finished (success or failure) |
| `RuleTriggered` | Rule fired and executed actions |
| `SyncCompleted` | Sync cycle finished |
| `ConflictDetected` | Sync found a conflict requiring resolution |

## Administrative Events

| Event | Emitted when |
|-------|-------------|
| `SourceCreated` | Source created |
| `SourceUpdated` | Source updated |
| `SourceDeleted` | Source deleted |
| `SinkCreated` | Sink created |
| `SinkUpdated` | Sink updated |
| `SinkDeleted` | Sink deleted |
| `FilterCreated` | Filter created |
| `FilterUpdated` | Filter updated |
| `FilterDeleted` | Filter deleted |
| `RuleCreated` | Rule created |
| `RuleUpdated` | Rule updated |
| `RuleDeleted` | Rule deleted |
| `ScheduleCreated` | Schedule created |
| `ScheduleUpdated` | Schedule updated |
| `ScheduleDeleted` | Schedule deleted |
| `RepoCreated` | Repository created |
| `RepoUpdated` | Repository updated |
| `RepoDeleted` | Repository deleted |

## Input Events

The hybrid architecture uses input events internally within `HandleInputEvent()`. These events represent commands that are processed by the repository before corresponding output events (listed above) are emitted.

| Input Event | Purpose |
|-------------|---------|
| `CreateNode` | Create a new node |
| `UpdateNode` | Fully replace a node (PUT) |
| `PatchNode` | Partially update a node (PATCH) |
| `DeleteNode` | Delete a node |
| `CreateEdge` | Create a new edge |
| `UpdateEdge` | Update an edge |
| `DeleteEdge` | Delete an edge |
| `CreateThread` | Create a new thread |
| `UpdateThread` | Update thread metadata |
| `DeleteThread` | Delete a thread |
| `AddThreadNode` | Add a node to a thread |
| `RemoveThreadNode` | Remove a node from a thread |
| `CreateAnnotation` | Create a new annotation |
| `UpdateAnnotation` | Update an annotation |
| `DeleteAnnotation` | Delete an annotation |

Input events are received on the repository's input subject and are not published externally. Each input event is validated and processed, resulting in the corresponding output event being published on the repository's output subject.

## Event Structure

```json
{
  "id": "01964d2e-...",
  "type": "NodeCreated",
  "repo_id": "01964d2e-...",
  "payload": { ... },
  "timestamp": "2026-03-30T10:00:00Z"
}
```

The `payload` varies by event type and contains the relevant entity data (the created node, the deleted edge ID, etc.).

## JetStream Subjects

Events are published to: `repo.{repo_id}.events.{EventType}`

Examples:
- `repo.01964d2e-xxx.events.NodeCreated`
- `repo.01964d2e-xxx.events.AnnotationDeleted`
- `repo.01964d2e-xxx.events.CollectionCompleted`

> **Migration note:** The existing codebase uses `repos.{id}.in` and `repos.{id}.out` subject patterns (plural `repos`, with separate input/output subjects). The canonical subject format going forward is `repo.{repo_id}.events.{EventType}` (singular `repo`, with event type as the final segment). Existing consumers should be migrated to the new subject pattern. During the transition, the repository module may publish to both old and new subjects to avoid breaking existing subscribers.
