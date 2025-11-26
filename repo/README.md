# Repository Module

The `repo` module provides HTTP and NATS interfaces for managing repositories, nodes, and edges in Upspeak. It implements the hexagonal architecture pattern, separating domain logic (in `core/`) from infrastructure concerns.

## Overview

The repo module:
- Exposes HTTP REST API for CRUD operations on repositories, nodes, and edges
- Publishes domain events to NATS for event-driven integrations
- Routes commands to the core domain layer (`core.Repository`)
- Manages multiple repositories with unique IDs

## Configuration

The repo module is configured in `upspeak.yaml`:

```yaml
modules:
  repo:
    enabled: true
    config:
      repo_id: "c..."      # Repository ID (xid format)
      repo_name: "default" # Human-readable name
```

If `repo_id` is not provided, a new ID is generated automatically.

## HTTP API

All endpoints use JSON for request and response bodies.

### Base URL

```
http://localhost:8080
```

### Endpoints

#### List Repositories

```http
GET /repos
```

Returns a map of repository IDs to names.

**Response:**
```json
{
  "c7g8h9i0j1k2l3m4": "default",
  "d8h9i0j1k2l3m4n5": "work"
}
```

**Status Codes:**
- `200 OK` - Success

---

#### Create Node

```http
POST /repos/{repo_id}/nodes
```

Creates a new node in the specified repository.

**Request Body:**
```json
{
  "type": "note",
  "subject": "Meeting notes",
  "content_type": "text/markdown",
  "body": "# Notes\n\nDiscussed project timeline...",
  "metadata": [
    {"key": "tags", "value": ["meetings", "project-x"]},
    {"key": "date", "value": "2024-01-15"}
  ]
}
```

**Fields:**
- `type` (string, required) - Type of node (e.g., "note", "article", "task")
- `subject` (string, required) - Brief description or title
- `content_type` (string, required) - MIME type of body content
- `body` (JSON, required) - Content of the node (can be any JSON value)
- `metadata` (array, optional) - Key-value pairs for additional data
- `created_by` (string, optional) - User ID who created the node
- `created_at` (string, optional) - ISO 8601 timestamp

**Response:**
```json
{
  "id": "c7g8h9i0j1k2l3m4",
  "type": "note",
  "subject": "Meeting notes",
  "content_type": "text/markdown",
  "body": "# Notes\n\nDiscussed project timeline...",
  "metadata": [
    {"key": "tags", "value": ["meetings", "project-x"]},
    {"key": "date", "value": "2024-01-15"}
  ],
  "created_by": "...",
  "created_at": "2024-01-15T10:30:00Z"
}
```

**Status Codes:**
- `201 Created` - Node created successfully
- `400 Bad Request` - Invalid request body or missing repository ID
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `CreateNode`
- Payload: `{"node": {...}}`

---

#### Get Node

```http
GET /repos/{repo_id}/nodes/{id}
```

Retrieves a node by its ID.

**Response:**
```json
{
  "id": "c7g8h9i0j1k2l3m4",
  "type": "note",
  "subject": "Meeting notes",
  "content_type": "text/markdown",
  "body": "# Notes\n\nDiscussed project timeline...",
  "metadata": [...],
  "created_by": "...",
  "created_at": "2024-01-15T10:30:00Z"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid node ID or missing repository ID
- `404 Not Found` - Repository or node not found

---

#### Update Node

```http
PUT /repos/{repo_id}/nodes/{id}
```

Updates an existing node.

**Request Body:**
Same as Create Node. The entire node is replaced with the new data.

**Response:**
```json
{
  "id": "c7g8h9i0j1k2l3m4",
  "type": "note",
  "subject": "Meeting notes - Updated",
  ...
}
```

**Status Codes:**
- `200 OK` - Node updated successfully
- `400 Bad Request` - Invalid request body or node ID
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `UpdateNode`
- Payload: `{"node_id": "...", "updated_node": {...}}`

---

#### Delete Node

```http
DELETE /repos/{repo_id}/nodes/{id}
```

Deletes a node by its ID.

**Response:**
No content.

**Status Codes:**
- `204 No Content` - Node deleted successfully
- `400 Bad Request` - Invalid node ID
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `DeleteNode`
- Payload: `{"node_id": "..."}`

**Note:** Deleting a node does not automatically delete its edges. You may need to clean up edges separately.

---

#### Create Edge

```http
POST /repos/{repo_id}/edges
```

Creates a new edge (relationship) between two nodes.

**Request Body:**
```json
{
  "type": "reply",
  "source": "c7g8h9i0j1k2l3m4",
  "target": "d8h9i0j1k2l3m4n5",
  "label": "reply to",
  "weight": 1.0
}
```

**Fields:**
- `type` (string, required) - Type of relationship (e.g., "reply", "annotation", "related")
- `source` (string, required) - ID of the source node (parent)
- `target` (string, required) - ID of the target node (child)
- `label` (string, optional) - Human-readable relationship description
- `weight` (number, optional) - Importance or strength of relationship (default: 1.0)

**Response:**
```json
{
  "id": "e9f0g1h2i3j4k5l6",
  "type": "reply",
  "source": "c7g8h9i0j1k2l3m4",
  "target": "d8h9i0j1k2l3m4n5",
  "label": "reply to",
  "weight": 1.0,
  "created_at": "2024-01-15T10:35:00Z"
}
```

**Status Codes:**
- `201 Created` - Edge created successfully
- `400 Bad Request` - Invalid request body
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `CreateEdge`
- Payload: `{"edge": {...}}`

---

#### Get Edge

```http
GET /repos/{repo_id}/edges/{id}
```

Retrieves an edge by its ID.

**Response:**
```json
{
  "id": "e9f0g1h2i3j4k5l6",
  "type": "reply",
  "source": "c7g8h9i0j1k2l3m4",
  "target": "d8h9i0j1k2l3m4n5",
  "label": "reply to",
  "weight": 1.0,
  "created_at": "2024-01-15T10:35:00Z"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid edge ID
- `404 Not Found` - Repository or edge not found

---

#### Update Edge

```http
PUT /repos/{repo_id}/edges/{id}
```

Updates an existing edge.

**Request Body:**
Same as Create Edge. The edge ID in the URL is used, not the ID in the request body.

**Response:**
```json
{
  "id": "e9f0g1h2i3j4k5l6",
  "type": "reply",
  "source": "c7g8h9i0j1k2l3m4",
  "target": "d8h9i0j1k2l3m4n5",
  "label": "updated reply to",
  "weight": 2.0,
  "created_at": "2024-01-15T10:35:00Z"
}
```

**Status Codes:**
- `200 OK` - Edge updated successfully
- `400 Bad Request` - Invalid request body or edge ID
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `UpdateEdge`
- Payload: `{"edge": {...}}`

---

#### Delete Edge

```http
DELETE /repos/{repo_id}/edges/{id}
```

Deletes an edge by its ID.

**Response:**
No content.

**Status Codes:**
- `204 No Content` - Edge deleted successfully
- `400 Bad Request` - Invalid edge ID
- `404 Not Found` - Repository not found
- `500 Internal Server Error` - Failed to create event or publish to NATS

**Event Published:**
- Subject: `repos.{repo_id}.in`
- Event Type: `DeleteEdge`
- Payload: `{"edge_id": "..."}`

---

## NATS Event Integration

The repo module uses NATS for event-driven communication between modules and external integrations.

### Event Flow

1. HTTP request arrives at repo module
2. Repo module creates an input event
3. Input event is published to `repos.{repo_id}.in`
4. Repository processes the event and returns an output event
5. Output event is published to `repos.{repo_id}.out`
6. External subscribers can react to output events

### NATS Subjects

Each repository has two NATS subjects:

- **Input:** `repos.{repo_id}.in` - Receives commands for the repository
- **Output:** `repos.{repo_id}.out` - Publishes domain events from the repository

### Event Types

#### Input Events (Commands)

Events published to `repos.{repo_id}.in`:

- `CreateNode` - Create a new node
- `UpdateNode` - Update an existing node
- `DeleteNode` - Delete a node
- `CreateEdge` - Create a new edge
- `UpdateEdge` - Update an existing edge
- `DeleteEdge` - Delete an edge

#### Output Events (Domain Events)

Events published to `repos.{repo_id}.out`:

- `NodeCreated` - A node was created
- `NodeUpdated` - A node was updated
- `NodeDeleted` - A node was deleted
- `EdgeCreated` - An edge was created
- `EdgeUpdated` - An edge was updated
- `EdgeDeleted` - An edge was deleted

### Event Structure

All events follow this structure:

```json
{
  "id": "f0g1h2i3j4k5l6m7",
  "type": "NodeCreated",
  "payload": {
    "node": {
      "id": "c7g8h9i0j1k2l3m4",
      "type": "note",
      "subject": "Meeting notes",
      ...
    }
  }
}
```

**Fields:**
- `id` (string) - Unique event ID (xid format)
- `type` (string) - Event type (see Event Types above)
- `payload` (object) - Event-specific data

### Event Payloads

#### NodeCreated, NodeUpdated

```json
{
  "node": {
    "id": "...",
    "type": "note",
    "subject": "...",
    "content_type": "text/plain",
    "body": "...",
    "metadata": [...],
    "created_by": "...",
    "created_at": "..."
  }
}
```

#### NodeDeleted

```json
{
  "node_id": "c7g8h9i0j1k2l3m4"
}
```

#### EdgeCreated, EdgeUpdated

```json
{
  "edge": {
    "id": "...",
    "type": "reply",
    "source": "...",
    "target": "...",
    "label": "reply to",
    "weight": 1.0,
    "created_at": "..."
  }
}
```

#### EdgeDeleted

```json
{
  "edge_id": "e9f0g1h2i3j4k5l6"
}
```

### Subscribing to Events

To listen for events from a repository, subscribe to its output subject:

**Go Example:**

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
	"github.com/upspeak/upspeak/core"
)

func main() {
	// Connect to NATS (Upspeak uses embedded NATS on localhost:4222)
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Subscribe to all output events for a repository
	repoID := "c7g8h9i0j1k2l3m4"
	subject := fmt.Sprintf("repos.%s.out", repoID)

	_, err = nc.Subscribe(subject, func(msg *nats.Msg) {
		var event core.Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("Failed to unmarshal event: %v", err)
			return
		}

		fmt.Printf("Received event: %s (ID: %s)\n", event.Type, event.ID)

		// Handle specific event types
		switch event.Type {
		case core.EventNodeCreated:
			var payload core.EventNodeCreatePayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				log.Printf("Failed to unmarshal payload: %v", err)
				return
			}
			fmt.Printf("New node created: %s\n", payload.Node.Subject)

		case core.EventNodeUpdated:
			var payload core.EventNodeUpdatePayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				log.Printf("Failed to unmarshal payload: %v", err)
				return
			}
			fmt.Printf("Node updated: %s\n", payload.UpdatedNode.Subject)

		// Handle other event types...
		}
	})

	if err != nil {
		log.Fatal(err)
	}

	// Keep the subscription alive
	select {}
}
```

**Note:** Upspeak runs an embedded NATS server by default. If `nats.private` is set to `true` in the configuration, external connections are not allowed.

### Publishing Events Directly

You can publish events directly to NATS to trigger repository operations:

**Go Example:**

```go
package main

import (
	"encoding/json"
	"log"

	"github.com/nats-io/nats.go"
	"github.com/rs/xid"
	"github.com/upspeak/upspeak/core"
)

func main() {
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Create a node through NATS
	node := &core.Node{
		ID:          xid.New(),
		Type:        "note",
		Subject:     "Created via NATS",
		ContentType: "text/plain",
		Body:        json.RawMessage(`"This node was created by publishing to NATS"`),
		Metadata:    []core.Metadata{},
	}

	event, err := core.NewEvent(core.EventCreateNode, core.EventNodeCreatePayload{
		Node: node,
	})
	if err != nil {
		log.Fatal(err)
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		log.Fatal(err)
	}

	repoID := "c7g8h9i0j1k2l3m4"
	inSubject := fmt.Sprintf("repos.%s.in", repoID)

	if err := nc.Publish(inSubject, eventData); err != nil {
		log.Fatal(err)
	}

	log.Println("Event published successfully")
}
```

## Error Handling

### HTTP Error Responses

All errors return a plain text error message:

```
Repository not found: repository not found: c7g8h9i0j1k2l3m4
```

### Common Errors

- **400 Bad Request** - Invalid JSON, missing required fields, or invalid IDs
- **404 Not Found** - Repository, node, or edge not found
- **500 Internal Server Error** - Internal processing error (event creation, NATS publish failure)

## Architecture

The repo module follows hexagonal architecture:

- **HTTP Handlers** (`handlers_node.go`, `handlers_edge.go`) - Infrastructure layer for HTTP
- **NATS Handlers** (`repo.go`) - Infrastructure layer for messaging
- **Repository** (`core/repo.go`) - Domain layer with business logic
- **Archive** (`core/archive.go`) - Port for storage abstraction

The module does not directly interact with storage. All persistence is handled through the `core.Archive` interface, which is implemented by the `archive` module.

## Module Interface

The repo module implements `app.Module`:

```go
type ModuleRepo struct {
	repos  map[string]*core.Repository
	logger *slog.Logger
}

func (m *ModuleRepo) Name() string
func (m *ModuleRepo) Init(config map[string]any) error
func (m *ModuleRepo) HTTPHandlers(pub app.Publisher) []app.HTTPHandler
func (m *ModuleRepo) MsgHandlers(pub app.Publisher) []app.MsgHandler
```

Additional methods:

```go
func (m *ModuleRepo) SetArchive(archive core.Archive)
func (m *ModuleRepo) GetRepository(repoID string) (*core.Repository, error)
func (m *ModuleRepo) ListRepositories() map[string]string
```

## See Also

- [docs/USAGE.md](../docs/USAGE.md) - User guide with workflow examples
- [archive/README.md](./archive/README.md) - Archive module documentation
- [core/README.md](../core/README.md) - Domain model documentation
