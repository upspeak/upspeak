# Core Package

The `core` package defines the foundational domain types and abstractions for Upspeak. It follows Domain-Driven Design principles, keeping domain logic independent of infrastructure concerns.

## Domain Models

All entities use **UUID v7** as their primary key and carry a human-friendly **short ID** (e.g. `NODE-42`).

- **Node** — A unit of information in the knowledge graph (article, note, bookmark, etc.)
- **Edge** — A relationship between two nodes (reply, annotation, contains, etc.)
- **Thread** — A composite entity: an ordered collection of nodes linked by edges, with its own first-class identity
- **Annotation** — A composite entity: a note or highlight attached to a target node, with its own identity
- **Repository** — A top-level organising unit (a self-contained knowledge graph)
- **User** — An identity in the system (global, not repo-scoped)

## Key Design Decisions

- **UUID v7** replaces xid: time-ordered, database-friendly, globally unique
- **Short IDs** (e.g. `NODE-42`, `REPO-1`): per-repo monotonic sequences, immutable, never reused
- **Versioning**: all persisted entities carry a `Version` field for optimistic concurrency control
- **Timestamps**: all entities have `CreatedAt` and `UpdatedAt`
- **Repository** is a pure data model, not an aggregate — event handling is in the `repo` module

## Shared Types

`shared_types.go` defines typed string constants used across the system:

- `EventType` / `InputEventType` — output and input event types
- `ConnectorType`, `JobType`, `ActionType` — connector and job classification
- `FilterMode`, `ConditionOp` — filter evaluation
- `ResourceStatus`, `JobStatus` — lifecycle tracking
- `RateLimit` — per-source/sink rate limiting configuration

## Archive Interface

`archive.go` defines the `Archive` interface — the contract for persistent storage. Implementations handle repositories, nodes, edges, threads, annotations, and sequence generation for short IDs.

## Event System

`events.go` defines the `Event` struct and payload types. Events use the canonical subject format `repo.{repo_id}.events.{EventType}`.
