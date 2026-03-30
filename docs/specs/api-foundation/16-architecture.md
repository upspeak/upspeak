# Internal Architecture

## Domain Models

All entities use UUID v7 as primary key and carry a short ID. Existing `core/` models are updated to use `uuid.UUID` instead of `xid.ID` and gain `ShortID` and `RepoID` fields.

All models that represent persisted entities include `updated_at` alongside `created_at`. The `updated_at` field is set to the same value as `created_at` on initial creation and is bumped on every subsequent write. This is critical for sync conflict detection and Last-Write-Wins (LWW) merge strategies.

All persisted entities that support concurrent editing carry an integer `Version` field for optimistic concurrency control. The version starts at 1 on creation and is incremented on each successful write. Archive write methods accept the expected version; if the stored version does not match, the write is rejected with a version conflict error.

### User Model

```go
// User represents an identity in the system. Users are global — they are not
// scoped to a repository. The ShortID is drawn from a global sequence.
type User struct {
    ID          uuid.UUID `json:"id"`
    ShortID     string    `json:"short_id"`    // "USER-1" — global, not repo-scoped
    Username    string    `json:"username"`
    Hostname    string    `json:"hostname"`
    DisplayName string    `json:"display_name"`
    Source      string    `json:"source"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### Updated Existing Models

```go
type Repository struct {
    ID          uuid.UUID `json:"id"`
    ShortID     string    `json:"short_id"`    // "REPO-1" — scoped per owner (user_sequences)
    Slug        string    `json:"slug"`        // "research"
    Name        string    `json:"name"`        // "AI Governance Research"
    Description string    `json:"description"` // Human-readable description of the repository's purpose
    OwnerID     uuid.UUID `json:"owner_id"`
    Version     int       `json:"version"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Node struct {
    ID          uuid.UUID       `json:"id"`
    ShortID     string          `json:"short_id"`  // "NODE-42"
    RepoID      uuid.UUID       `json:"repo_id"`
    Type        string          `json:"type"`
    Subject     string          `json:"subject"`
    ContentType string          `json:"content_type"`
    Body        json.RawMessage `json:"body"`
    Metadata    []Metadata      `json:"metadata"`
    CreatedBy   uuid.UUID       `json:"created_by"`
    Version     int             `json:"version"`
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`
}

type Edge struct {
    ID        uuid.UUID `json:"id"`
    ShortID   string    `json:"short_id"`  // "EDGE-15"
    RepoID    uuid.UUID `json:"repo_id"`
    Type      string    `json:"type"`
    Source    uuid.UUID `json:"source"`
    Target    uuid.UUID `json:"target"`
    Label     string    `json:"label"`
    Weight    float64   `json:"weight"`
    CreatedBy uuid.UUID `json:"created_by"`
    Version   int       `json:"version"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Thread and Annotation

Thread and Annotation are composite entities. Each carries its own UUID and ShortID while also containing embedded domain objects (a root Node for threads; a Node and Edge for annotations). This means a Thread has a first-class identity independent of its root node — you can reference, query, and delete a thread by its own `threadID`.

```go
// Thread is a composite entity representing an ordered collection of nodes
// linked by edges. The Thread itself has a first-class identity (ID, ShortID)
// distinct from its root Node. The root Node is the initial content of the
// thread; additional nodes are attached via AddNodeToThread/RemoveNodeFromThread.
type Thread struct {
    ID        uuid.UUID  `json:"id"`
    ShortID   string     `json:"short_id"`  // "THREAD-7" — repo-scoped sequence
    RepoID    uuid.UUID  `json:"repo_id"`
    Node      Node       `json:"node"`      // Root node — the aggregate root of the thread
    Edges     []Edge     `json:"edges"`     // Edges linking nodes within the thread
    Metadata  []Metadata `json:"metadata"`
    CreatedBy uuid.UUID  `json:"created_by"`
    Version   int        `json:"version"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

// Annotation is a composite entity representing a note or highlight attached
// to a target node. The Annotation itself has a first-class identity (ID,
// ShortID) separate from the Node and Edge it contains. The embedded Node
// holds the annotation content; the embedded Edge links that content to the
// target node with type="annotation".
type Annotation struct {
    ID         uuid.UUID `json:"id"`
    ShortID    string    `json:"short_id"`  // "ANNO-3" — repo-scoped sequence
    RepoID     uuid.UUID `json:"repo_id"`
    Node       Node      `json:"node"`       // The annotation content node
    Edge       Edge      `json:"edge"`       // Edge linking annotation node to target, type="annotation"
    Motivation string    `json:"motivation"` // e.g. "commenting", "highlighting", "bookmarking"
    CreatedBy  uuid.UUID `json:"created_by"`
    Version    int       `json:"version"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

### New Domain Models

```go
type Source struct {
    ID              uuid.UUID       `json:"id"`
    ShortID         string          `json:"short_id"`     // "SRC-5"
    RepoID          uuid.UUID       `json:"repo_id"`
    Name            string          `json:"name"`
    Connector       ConnectorType   `json:"connector"`
    Config          json.RawMessage `json:"config"`
    FilterIDs       []uuid.UUID     `json:"filter_ids"`
    FilterChainMode FilterMode      `json:"filter_chain_mode"`
    RateLimit       *RateLimit      `json:"rate_limit"`
    Status          ResourceStatus  `json:"status"`
    Version         int             `json:"version"`
    CreatedAt       time.Time       `json:"created_at"`
    UpdatedAt       time.Time       `json:"updated_at"`
}

type Sink struct {
    ID              uuid.UUID       `json:"id"`
    ShortID         string          `json:"short_id"`     // "SINK-2"
    RepoID          uuid.UUID       `json:"repo_id"`
    Name            string          `json:"name"`
    Connector       ConnectorType   `json:"connector"`
    Config          json.RawMessage `json:"config"`
    FilterIDs       []uuid.UUID     `json:"filter_ids"`
    FilterChainMode FilterMode      `json:"filter_chain_mode"`
    RateLimit       *RateLimit      `json:"rate_limit"`
    Status          ResourceStatus  `json:"status"`
    Version         int             `json:"version"`
    CreatedAt       time.Time       `json:"created_at"`
    UpdatedAt       time.Time       `json:"updated_at"`
}

type Filter struct {
    ID          uuid.UUID   `json:"id"`
    ShortID     string      `json:"short_id"`  // "FILTER-3"
    RepoID      uuid.UUID   `json:"repo_id"`
    Name        string      `json:"name"`
    Description string      `json:"description"`
    Mode        FilterMode  `json:"mode"`
    Conditions  []Condition `json:"conditions"`
    Version     int         `json:"version"`
    CreatedAt   time.Time   `json:"created_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
}

type Condition struct {
    Field string      `json:"field"`
    Op    ConditionOp `json:"op"`
    Value any         `json:"value"`
}

type Schedule struct {
    ID        uuid.UUID      `json:"id"`
    ShortID   string         `json:"short_id"`  // "SCHED-4"
    Name      string         `json:"name"`
    Cron      string         `json:"cron"`
    Action    ScheduleAction `json:"action"`
    Enabled   bool           `json:"enabled"`
    NextRun   time.Time      `json:"next_run"`
    LastRun   *time.Time     `json:"last_run,omitempty"` // nil until the schedule has run at least once
    Version   int            `json:"version"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}

type ScheduleAction struct {
    Type     JobType         `json:"type"`
    SourceID *uuid.UUID      `json:"source_id,omitempty"`
    SinkID   *uuid.UUID      `json:"sink_id,omitempty"`
    RepoID   uuid.UUID       `json:"repo_id"`
    Params   json.RawMessage `json:"params,omitempty"`
}

type Rule struct {
    ID        uuid.UUID    `json:"id"`
    ShortID   string       `json:"short_id"`  // "RULE-8"
    RepoID    uuid.UUID    `json:"repo_id"`
    Name      string       `json:"name"`
    Trigger   RuleTrigger  `json:"trigger"`
    Actions   []RuleAction `json:"actions"`
    Enabled   bool         `json:"enabled"`
    Version   int          `json:"version"`
    CreatedAt time.Time    `json:"created_at"`
    UpdatedAt time.Time    `json:"updated_at"`
}

type RuleTrigger struct {
    Event     EventType   `json:"event"`
    FilterIDs []uuid.UUID `json:"filter_ids"`
}

type RuleAction struct {
    Type   JobType         `json:"type"`
    Params json.RawMessage `json:"params"`
}

type Job struct {
    ID          uuid.UUID       `json:"id"`
    ShortID     string          `json:"short_id"`  // "JOB-109"
    RepoID      uuid.UUID       `json:"repo_id"`   // Top-level for efficient querying by repository
    Type        JobType         `json:"type"`
    Status      JobStatus       `json:"status"`
    StartedAt   *time.Time      `json:"started_at"`
    CompletedAt *time.Time      `json:"completed_at"`
    Result      json.RawMessage `json:"result,omitempty"`
    Error       *string         `json:"error,omitempty"`
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`
}
```

### Shared Types

```go
type ConnectorType string   // "rss", "discourse", "matrix", "fediverse", "webhook", "email", "repo", "upspeak", "webpage"
type JobType string         // "collect", "publish", "sync", "webhook"
type ActionType string      // "collect", "publish", "webhook", "enrich", "relate", "annotate"
type FilterMode string      // "all", "any"
type ConditionOp string     // "eq", "neq", "contains", "not_contains", "starts_with", "ends_with", "in", "not_in", "gt", "lt", "gte", "lte", "exists", "not_exists", "matches"
type ResourceStatus string  // "active", "paused", "error", "rate_limited"
type JobStatus string       // "pending", "running", "completed", "failed", "cancelled"

type RateLimit struct {
    MaxRequests       int `json:"max_requests"`
    WindowSeconds     int `json:"window_seconds"`
    RetryAfterSeconds int `json:"retry_after_seconds"`
}
```

## Input Event Types

Input events are commands sent to `Repository.HandleInputEvent()`. The repository processes these synchronously, writes to the archive, and then publishes the corresponding output event to JetStream.

```go
type InputEventType string

const (
    // Node operations
    InputCreateNode InputEventType = "CreateNode"
    InputUpdateNode InputEventType = "UpdateNode"
    InputPatchNode  InputEventType = "PatchNode"
    InputDeleteNode InputEventType = "DeleteNode"

    // Edge operations
    InputCreateEdge InputEventType = "CreateEdge"
    InputUpdateEdge InputEventType = "UpdateEdge"
    InputDeleteEdge InputEventType = "DeleteEdge"

    // Thread operations
    InputCreateThread     InputEventType = "CreateThread"
    InputUpdateThread     InputEventType = "UpdateThread"
    InputDeleteThread     InputEventType = "DeleteThread"
    InputAddThreadNode    InputEventType = "AddThreadNode"
    InputRemoveThreadNode InputEventType = "RemoveThreadNode"

    // Annotation operations
    InputCreateAnnotation InputEventType = "CreateAnnotation"
    InputUpdateAnnotation InputEventType = "UpdateAnnotation"
    InputDeleteAnnotation InputEventType = "DeleteAnnotation"
)
```

## Output Event Types (JetStream)

Output events are published to JetStream after the corresponding write is confirmed in the archive. See [15-events.md](./15-events.md) for full details.

### Knowledge Graph Events

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

### Administrative Events

These events are emitted when connector, filter, rule, and schedule resources are created, updated, or deleted.

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

### Operational Events

| Event | Emitted when |
|-------|-------------|
| `CollectionCompleted` | Source collection finished (success or failure) |
| `PublishCompleted` | Sink publish finished (success or failure) |
| `RuleTriggered` | Rule fired and executed actions |
| `SyncCompleted` | Sync cycle finished |
| `ConflictDetected` | Sync found a conflict requiring resolution |

## Sequence Storage

Short IDs are monotonically increasing within their scope. The `next_seq` column is atomically incremented (using SQL `UPDATE ... SET next_seq = next_seq + 1 RETURNING next_seq - 1`) to guarantee uniqueness even under concurrent access. The returned value is used to construct the short ID (e.g. `NODE-42`).

```sql
-- Per-repo sequences for entity short IDs.
-- Each (repo_id, entity) pair maintains an independent monotonically increasing counter.
CREATE TABLE repo_sequences (
    repo_id    TEXT NOT NULL,
    entity     TEXT NOT NULL,  -- 'node', 'edge', 'thread', 'anno', 'filter', 'source', 'sink', 'rule'
    next_seq   INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (repo_id, entity)
);

-- Global sequences (schedule, job).
-- These are not scoped to any repo or user.
CREATE TABLE global_sequences (
    entity    TEXT PRIMARY KEY,  -- 'schedule', 'job', 'user'
    next_seq  INTEGER NOT NULL DEFAULT 1
);

-- Per-user sequences for user-scoped resources.
-- Repository short IDs (REPO-N) are scoped per owner so that each user's
-- repositories are numbered independently starting from 1.
CREATE TABLE user_sequences (
    owner_id  TEXT NOT NULL,
    entity    TEXT NOT NULL,  -- 'repo'
    next_seq  INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (owner_id, entity)
);

-- Repo slug redirects.
-- When a repository slug changes, the old slug is recorded here so that
-- existing links and bookmarks continue to resolve.
CREATE TABLE repo_slug_redirects (
    old_slug   TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    repo_id    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (old_slug, owner_id)
);
```

## JetStream Stream Design

```
Stream: REPO_{repo_id}_EVENTS
  Subjects: repo.{repo_id}.events.>
  Retention: Limits (configurable per deployment)
  Storage: File
  Consumers:
    - rules-engine      (filters and evaluates rules for this repo)
    - connector-repo    (repo connector subscriptions from other repos)
    - realtime-ws       (fans out to WebSocket clients)
    - sync-outbound     (queues for remote replication)

Stream: JOBS
  Subjects: jobs.>
  Retention: WorkQueue
  Storage: File
  Consumers:
    - job-runner        (picks up and executes async jobs)

Stream: SCHEDULES
  Subjects: schedules.trigger.>
  Retention: WorkQueue
  Storage: File
  Consumers:
    - scheduler         (executes triggered schedules)
```

### Per-Repo Stream Lifecycle

Each repository gets its own JetStream stream (`REPO_{repo_id}_EVENTS`) created when the repository is first created. **When a repository is deleted, its JetStream stream `REPO_{repo_id}_EVENTS` is also deleted**, along with all associated consumers. This ensures that no orphaned streams or consumers accumulate over time. The deletion is performed as part of the repository deletion transaction — if stream deletion fails, the repository deletion is rolled back.

### Event Flow After a Write

```
HTTP Handler
  -> Repository.HandleInputEvent()  (synchronous, in-process)
  -> Archive.SaveNode()             (synchronous, to SQLite + filesystem)
  -> return confirmed response to client (201/200)
  -> publish to JetStream: repo.{repo_id}.events.NodeCreated
       |
       +-> rules-engine consumer
       |     loads repo rules, evaluates filter conditions, executes matching actions
       |
       +-> connector-repo consumer
       |     for repos with this repo as source: apply filters, write to target
       |
       +-> realtime-ws consumer
       |     push event to subscribed WebSocket clients
       |
       +-> sync-outbound consumer
             queue for replication to registered peers
```

## Archive Interface (revised)

### Query and Result Types

```go
// ListOptions provides cursor-based pagination for list operations.
type ListOptions struct {
    Limit  int    `json:"limit"`
    Offset int    `json:"offset"`
    SortBy string `json:"sort_by"` // e.g. "created_at", "updated_at", "short_id"
    Order  string `json:"order"`   // "asc" or "desc"
}

// EdgeQueryOptions filters edges by direction and type.
type EdgeQueryOptions struct {
    Direction string // "outgoing", "incoming", "both"
    Type      string // edge type filter; empty means all types
    ListOptions
}

// AnnotationQueryOptions filters annotations, optionally by motivation.
type AnnotationQueryOptions struct {
    Motivation string // e.g. "commenting", "highlighting"; empty means all
    ListOptions
}

// SearchOptions provides structured search filters for nodes.
type SearchOptions struct {
    Type          []string          `json:"type"`            // filter by node type(s)
    CreatedAfter  *time.Time        `json:"created_after"`
    CreatedBefore *time.Time        `json:"created_before"`
    HasEdgeType   string            `json:"has_edge_type"`   // only nodes connected by this edge type
    Metadata      map[string]string `json:"metadata"`        // key-value metadata filters (all must match)
    Limit         int               `json:"limit"`
    Offset        int               `json:"offset"`
}

// GraphOptions configures graph traversal behaviour.
type GraphOptions struct {
    EdgeType  string // filter traversal to this edge type; empty means all
    Direction string // "outgoing", "incoming", "both"
}

// GraphResult holds the result of a graph traversal.
type GraphResult struct {
    Root  *Node  `json:"root"`
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}
```

### Archive Interface

```go
type Archive interface {
    // Repository operations
    SaveRepository(repo *Repository) error
    GetRepository(repoID uuid.UUID) (*Repository, error)
    ListRepositories(ownerID uuid.UUID, opts ListOptions) ([]Repository, int, error)
    DeleteRepository(repoID uuid.UUID) error

    // Node operations
    SaveNode(node *Node) error
    SaveBatchNodes(nodes []Node) ([]Node, error)
    GetNode(nodeID uuid.UUID) (*Node, error)
    DeleteNode(nodeID uuid.UUID) error
    ListNodes(repoID uuid.UUID, opts ListOptions) ([]Node, int, error)
    SearchNodes(repoID uuid.UUID, query string, opts SearchOptions) ([]Node, int, error)
    GetNodeEdges(nodeID uuid.UUID, opts EdgeQueryOptions) ([]Edge, error)
    GetNodeAnnotations(nodeID uuid.UUID, opts AnnotationQueryOptions) ([]Annotation, error)

    // Edge operations
    SaveEdge(edge *Edge) error
    SaveBatchEdges(edges []Edge) ([]Edge, error)
    GetEdge(edgeID uuid.UUID) (*Edge, error)
    DeleteEdge(edgeID uuid.UUID) error
    ListEdges(repoID uuid.UUID, opts ListOptions) ([]Edge, int, error)

    // Thread operations — addressed by threadID, not nodeID
    SaveThread(thread *Thread) error
    GetThread(threadID uuid.UUID) (*Thread, error)
    DeleteThread(threadID uuid.UUID) error
    ListThreads(repoID uuid.UUID, opts ListOptions) ([]Thread, int, error)
    AddNodeToThread(threadID uuid.UUID, nodeID uuid.UUID, edgeType string) error
    RemoveNodeFromThread(threadID uuid.UUID, nodeID uuid.UUID) error

    // Annotation operations — addressed by annotationID, not nodeID
    SaveAnnotation(annotation *Annotation) error
    GetAnnotation(annotationID uuid.UUID) (*Annotation, error)
    DeleteAnnotation(annotationID uuid.UUID) error
    ListAnnotations(repoID uuid.UUID, opts ListOptions) ([]Annotation, int, error)

    // Graph operations
    TraverseGraph(nodeID uuid.UUID, depth int, opts GraphOptions) (*GraphResult, error)

    // Ref resolution.
    // ResolveRef resolves a short ID (e.g. "NODE-42"), slug, or bare UUID string
    // to the canonical UUID and entity type. Returns (uuid, entityType, error)
    // where entityType is one of "node", "edge", "thread", "annotation", etc.
    ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error)
}
```

Each new module (connector, filter, scheduler, rules, sync, jobs) has its own storage interface. They share the same SQLite database but through their own tables and dedicated interfaces.

## Optimistic Concurrency

Archive write methods (`SaveNode`, `SaveEdge`, `SaveThread`, `SaveAnnotation`, `SaveRepository`) check the `Version` field on the entity being saved:

1. On **create** (Version == 0 or entity does not exist): the entity is inserted with Version = 1.
2. On **update** (Version > 0): the archive executes `UPDATE ... WHERE id = ? AND version = ?` using the entity's current version. If no row is updated, a `VersionConflictError` is returned. On success, the version is incremented.

This prevents lost updates when two clients read the same entity, modify it, and attempt to write concurrently.

```go
type VersionConflictError struct {
    EntityType string
    EntityID   uuid.UUID
    Expected   int
    Actual     int
}

func (e *VersionConflictError) Error() string {
    return fmt.Sprintf("version conflict on %s %s: expected %d, got %d",
        e.EntityType, e.EntityID, e.Expected, e.Actual)
}
```

## Module Composition

All modules are mounted under the `/api/v1` path prefix. The `app` framework's `AddModuleOnPath` is used for every module so that all API handlers are grouped under a single versioned prefix. Each module's internal handler paths are relative to this mount point.

```go
func main() {
    config := app.LoadConfig("upspeak.yaml")
    up := app.New(config)

    // All API modules mounted under /api/v1.
    // Each module registers its own sub-paths relative to the mount point.
    // For example, repo.Module registers "/repos", "/repos/{repo_id}/nodes", etc.
    // so the full path becomes /api/v1/repos, /api/v1/repos/{repo_id}/nodes, etc.
    up.AddModuleOnPath(&repo.Module{}, "/api/v1")
    up.AddModuleOnPath(&connector.Module{}, "/api/v1")   // sources, sinks, collect
    up.AddModuleOnPath(&filter.Module{}, "/api/v1")      // filter CRUD + evaluation
    up.AddModuleOnPath(&scheduler.Module{}, "/api/v1")   // cron scheduling
    up.AddModuleOnPath(&rules.Module{}, "/api/v1")       // rule evaluation engine
    up.AddModuleOnPath(&search.Module{}, "/api/v1")      // search + browse + graph
    up.AddModuleOnPath(&realtime.Module{}, "/api/v1")    // WebSocket
    up.AddModuleOnPath(&sync.Module{}, "/api/v1")        // sync + peers + conflicts
    up.AddModuleOnPath(&jobs.Module{}, "/api/v1")        // job tracking

    // UI module at root for SPA serving
    up.AddModuleOnPath(&ui.Module{}, "")

    up.Start()
}
```

> **Note:** The current `app` framework allows multiple modules to be mounted at the same path because handler registration uses the full `method + path` pattern (e.g. `GET /api/v1/repos`). Each module's handlers are registered individually on the shared `http.ServeMux`, so there is no conflict as long as no two modules register the same method+path combination. If the framework is updated to enforce unique mount points, a sub-module path inheritance mechanism will be needed — e.g. a `ModuleGroup` that composes multiple modules under a single path prefix.

## Module Mapping

| Module | API Groups | New? |
|--------|-----------|------|
| `repo` | Repositories, Nodes, Edges, Threads, Annotations | Existing (expanded) |
| `connector` | Sources, Sinks, Collect | New |
| `filter` | Filters | New |
| `scheduler` | Schedules | New |
| `rules` | Rules | New |
| `search` | Search, Browse, Graph | New |
| `realtime` | WebSocket | New |
| `sync` | Sync, Peers, Conflicts | New |
| `jobs` | Jobs | New |
