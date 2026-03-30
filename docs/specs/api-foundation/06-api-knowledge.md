# Knowledge Graph API

Nodes, Edges, Threads, and Annotations are the core primitives of the knowledge graph. All domain operations compose on top of these.

## Nodes

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/nodes` | Create node |
| `POST` | `/api/v1/repos/{repo_ref}/nodes/batch` | Create multiple nodes |
| `GET` | `/api/v1/repos/{repo_ref}/nodes` | List nodes |

Node-specific query params: `?type={type}`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{node_ref}` | Get node |
| `PUT` | `/api/v1/repos/{repo_ref}/{node_ref}` | Replace node |
| `PATCH` | `/api/v1/repos/{repo_ref}/{node_ref}` | Partial update (merge metadata, update fields) |
| `DELETE` | `/api/v1/repos/{repo_ref}/{node_ref}` | Delete node |
| `GET` | `/api/v1/repos/{repo_ref}/{node_ref}/edges` | List edges connected to node |
| `GET` | `/api/v1/repos/{repo_ref}/{node_ref}/annotations` | List annotations on node |

Edge sub-resource query params: `?direction=outgoing|incoming|both`, `?type={edge_type}`

### Node Response

All entity responses include a `version` field (integer, starts at 1, incremented on each update). The `ETag` header is set to the version value on GET/PUT/PATCH responses, and `If-Match` can be used for optimistic concurrency control.

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "NODE-42",
    "repo_id": "01964d2e-...",
    "type": "article",
    "subject": "EU AI Act passed",
    "content_type": "text/markdown",
    "body": "...",
    "metadata": [
      { "key": "priority", "value": "\"high\"" }
    ],
    "version": 1,
    "created_by": "01964d2e-...",
    "created_at": "2026-03-30T10:00:00Z"
  }
}
```

### Delete Semantics (Cascading)

Deleting a node triggers cascading deletes:

- All edges where this node is the source or target are also deleted.
- All annotations targeting this node are also deleted.
- The node is removed from any threads it belongs to (ThreadNodeRemoved events emitted).

These cascades are handled asynchronously by JetStream consumers reacting to `NodeDeleted` events. The DELETE endpoint itself returns `204 No Content` immediately, and the cascading events (`EdgeDeleted`, `AnnotationDeleted`, `ThreadNodeRemoved`) are emitted by the respective consumers as they process the `NodeDeleted` event.

### Batch Create

Batch operations are **atomic** — either all items succeed or the entire batch fails. No partial creates are performed. On failure, the server returns `422 Unprocessable Entity` with details of which items failed validation.

```json
// POST /api/v1/repos/{repo_ref}/nodes/batch
{
  "nodes": [
    { "type": "section", "subject": "Chapter 1", "content_type": "text/markdown", "body": "..." },
    { "type": "section", "subject": "Chapter 2", "content_type": "text/markdown", "body": "..." }
  ]
}

// Success Response
{ "data": { "created": 2, "nodes": [ ... ] } }

// Failure Response (422)
{
  "error": {
    "code": "BATCH_VALIDATION_FAILED",
    "message": "2 of 3 items failed validation",
    "details": [
      { "index": 0, "error": "subject is required" },
      { "index": 2, "error": "unknown content_type" }
    ]
  }
}
```

### PATCH Semantics

PATCH merges metadata and updates specified fields without replacing the entire node:

```json
// PATCH /api/v1/repos/research/NODE-42
{
  "metadata": [
    { "key": "category", "value": "\"regulation\"" }
  ]
}
```

Metadata merge: new keys are added, existing keys with the same name are updated. To **delete** a metadata key, set its value to `null`:

```json
// PATCH /api/v1/repos/research/NODE-42
{
  "metadata": [
    { "key": "category", "value": null }
  ]
}
```

A `null` value removes the key entirely from the node's metadata. This can be combined with adds and updates in the same request.

---

## Edges

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/edges` | Create edge |
| `POST` | `/api/v1/repos/{repo_ref}/edges/batch` | Create multiple edges |
| `GET` | `/api/v1/repos/{repo_ref}/edges` | List edges |

Edge-specific query params: `?source={node_ref}`, `?target={node_ref}`, `?type={edge_type}`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{edge_ref}` | Get edge |
| `PUT` | `/api/v1/repos/{repo_ref}/{edge_ref}` | Update edge |
| `DELETE` | `/api/v1/repos/{repo_ref}/{edge_ref}` | Delete edge |

### Create Payload

```json
// POST /api/v1/repos/{repo_ref}/edges
{
  "type": "has_section",
  "source": "NODE-100",
  "target": "NODE-101",
  "label": "Installation section",
  "weight": 1.0
}
```

### Edge Response

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "EDGE-55",
    "repo_id": "01964d2e-...",
    "type": "has_section",
    "source": "NODE-100",
    "target": "NODE-101",
    "label": "Installation section",
    "weight": 1.0,
    "version": 1,
    "created_by": "01964d2e-...",
    "created_at": "2026-03-30T10:00:00Z"
  }
}
```

### Delete Semantics (Cascading)

Deleting an edge that is part of a thread removes it from the thread, emitting a `ThreadNodeRemoved` event. No other cascading deletes occur. The DELETE endpoint returns `204 No Content`.

---

## Threads

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/threads` | Create thread |
| `GET` | `/api/v1/repos/{repo_ref}/threads` | List threads |

Thread-specific query params: `?type={thread_type}`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{thread_ref}` | Get thread (includes edges and referenced nodes) |
| `PUT` | `/api/v1/repos/{repo_ref}/{thread_ref}` | Update thread metadata |
| `DELETE` | `/api/v1/repos/{repo_ref}/{thread_ref}` | Delete thread |
| `POST` | `/api/v1/repos/{repo_ref}/{thread_ref}/nodes` | Add node to thread |
| `DELETE` | `/api/v1/repos/{repo_ref}/{thread_ref}/{node_ref}` | Remove node from thread |
| `POST` | `/api/v1/repos/{repo_ref}/{thread_ref}/publish` | Publish thread to network/sink |

### Create Payload

Creating a thread creates a root node and the thread structure. The `node` field defines the root node of the thread.

```json
// POST /api/v1/repos/{repo_ref}/threads
{
  "node": {
    "type": "collection",
    "subject": "EU AI Act Coverage",
    "content_type": "text/plain",
    "body": ""
  },
  "metadata": []
}
```

### Thread GET Response

The GET response for a thread includes the full thread structure: the root node, all edges connecting nodes to the thread, and the referenced nodes.

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "THREAD-7",
    "repo_id": "01964d2e-...",
    "root_node": {
      "id": "01964d2e-...",
      "short_id": "NODE-50",
      "type": "collection",
      "subject": "EU AI Act Coverage",
      "content_type": "text/plain",
      "body": "",
      "metadata": [],
      "version": 1,
      "created_by": "01964d2e-...",
      "created_at": "2026-03-30T10:00:00Z"
    },
    "edges": [
      {
        "id": "01964d2e-...",
        "short_id": "EDGE-60",
        "type": "contains",
        "source": "NODE-50",
        "target": "NODE-42",
        "label": "",
        "weight": 1.0,
        "version": 1,
        "created_by": "01964d2e-...",
        "created_at": "2026-03-30T10:05:00Z"
      }
    ],
    "nodes": [
      {
        "id": "01964d2e-...",
        "short_id": "NODE-42",
        "type": "article",
        "subject": "EU AI Act passed",
        "content_type": "text/markdown",
        "body": "...",
        "metadata": [],
        "version": 2,
        "created_by": "01964d2e-...",
        "created_at": "2026-03-30T09:00:00Z"
      }
    ],
    "version": 1,
    "created_by": "01964d2e-...",
    "created_at": "2026-03-30T10:00:00Z"
  }
}
```

### Delete Semantics (Cascading)

Deleting a thread removes the thread structure (all edges linking nodes to the thread) but does **not** delete the contained nodes. The root node **is** deleted. The DELETE endpoint returns `204 No Content`, and `EdgeDeleted` and `NodeDeleted` (for the root node) events are emitted.

### Add Node to Thread

```json
// POST /api/v1/repos/research/THREAD-7/nodes
{ "node_id": "NODE-42", "edge_type": "contains" }
```

### Publish Thread

```json
// POST /api/v1/repos/research/THREAD-7/publish
{
  "visibility": "public|network|private",
  "sink_id": "SINK-1",
  "allow_follow": true,
  "dry_run": false
}
```

---

## Annotations

### Collection Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/repos/{repo_ref}/annotations` | Create annotation on a node |
| `GET` | `/api/v1/repos/{repo_ref}/annotations` | List annotations |

Annotation-specific query params: `?target={node_ref}`, `?motivation={motivation}`

### Entity Endpoints (flat URL)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/repos/{repo_ref}/{anno_ref}` | Get annotation |
| `PUT` | `/api/v1/repos/{repo_ref}/{anno_ref}` | Update annotation |
| `DELETE` | `/api/v1/repos/{repo_ref}/{anno_ref}` | Delete annotation |

### Create Payload

```json
{
  "target_node_id": "NODE-42",
  "motivation": "commenting|highlighting|summarising|classifying|bookmarking",
  "node": {
    "type": "comment",
    "subject": "Key implications",
    "content_type": "text/markdown",
    "body": "This changes the compliance requirements for..."
  }
}
```

### Text Selector Support

Annotations can optionally include a `selector` field for targeting specific parts of the target node's content. The selector follows the [W3C Web Annotation Data Model](https://www.w3.org/TR/annotation-model/#selectors) selector types: `TextQuoteSelector`, `TextPositionSelector`, and `FragmentSelector`.

```json
{
  "target_node_id": "NODE-42",
  "motivation": "highlighting",
  "selector": {
    "type": "TextQuoteSelector",
    "exact": "the compliance requirements",
    "prefix": "This changes ",
    "suffix": " for..."
  },
  "node": {
    "type": "highlight",
    "subject": "Compliance requirements excerpt",
    "content_type": "text/plain",
    "body": "the compliance requirements"
  }
}
```

Selector types:

- **TextQuoteSelector**: Identifies content by exact text match with optional `prefix` and `suffix` context.
- **TextPositionSelector**: Identifies content by `start` and `end` character offsets (zero-based).
- **FragmentSelector**: Identifies content by a fragment identifier conforming to the target's media type (e.g., RFC 3236 for HTML).

The `selector` field is stored as part of the annotation's edge metadata. If omitted, the annotation targets the entire node.

### Response

```json
{
  "data": {
    "id": "01964d2e-...",
    "short_id": "ANNO-23",
    "node": {
      "id": "01964d2e-...",
      "short_id": "NODE-43",
      "type": "comment",
      "subject": "Key implications",
      "content_type": "text/markdown",
      "body": "This changes the compliance requirements for...",
      "metadata": [],
      "version": 1,
      "created_by": "01964d2e-...",
      "created_at": "2026-03-30T10:00:00Z"
    },
    "edge": {
      "id": "01964d2e-...",
      "short_id": "EDGE-70",
      "type": "annotates",
      "source": "NODE-43",
      "target": "NODE-42",
      "label": "commenting",
      "weight": 1.0,
      "version": 1,
      "created_by": "01964d2e-...",
      "created_at": "2026-03-30T10:00:00Z"
    },
    "motivation": "commenting",
    "version": 1
  }
}
```

### Update (PUT) Semantics

The `motivation` and the annotation's node `body`/`subject` can be updated via PUT. The `target_node_id` and edge cannot be changed — they are immutable after creation. Attempting to change the target returns `422 Unprocessable Entity`.

The annotation's `version` is incremented on each successful update. Use the `If-Match` header with the current version for optimistic concurrency control.

```json
// PUT /api/v1/repos/research/ANNO-23
{
  "motivation": "summarising",
  "node": {
    "subject": "Updated implications summary",
    "body": "The key change is that compliance requirements now include..."
  }
}
```

### Delete Semantics (Cascading)

Deleting an annotation deletes both the annotation's node and its edge. No other cascading deletes occur. The DELETE endpoint returns `204 No Content`, and `NodeDeleted` and `EdgeDeleted` events are emitted.
