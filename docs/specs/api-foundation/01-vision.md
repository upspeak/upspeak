# Vision & Architecture

## Vision

Upspeak is personal-first, federated knowledge infrastructure. It acts as a system of record for an individual, federates with other Upspeak instances, and integrates with diverse external systems (Matrix, RSS, Discourse, Fediverse, email, APIs, AI agents). It is privacy-aware, user-focused, and designed to manage information overload.

**Deployment modes:** SaaS, self-hosted (homelab/cloud), local machine. Single user may access from multiple devices.

**Local-first:** Supports offline local edits (sync when reconnected) and runs independently processing data in the background (fetching sources, running transformers, applying rules). When a user connects, they see the updated dataset based on the rules they have set.

**API-first:** Upspeak is an API by default — no bundled UI. Clients (web, mobile, CLI, agents) connect over the HTTP API. The UI is a separate concern/project.

## Architecture: Hybrid Synchronous Core + JetStream

```
+---------------------------------------------------------+
|  HTTP API Layer (domain-rich contracts)                  |
|  - Versioned API (/api/v1/...)                          |
|  - Structured JSON responses (envelope pattern)         |
|  - Auth-ready (middleware hooks, no impl yet)            |
+---------------------+-----------------------------------+
                      | synchronous call
+---------------------v-----------------------------------+
|  Domain Services (repo module)                           |
|  - Repository as aggregate root                          |
|  - Calls HandleInputEvent() directly (in-process)        |
|  - Returns confirmed result to HTTP layer                |
|  - Publishes output event to JetStream after write       |
+---------------------+-------------------+---------------+
                      |                   |
             write    |                   | publish
+---------------------v-----+   +---------v---------------+
|  Archive (storage)         |   |  JetStream (event bus)   |
|  - SQLite + filesystem     |   |  - Output event streams  |
|  - List/query support      |   |  - Cross-module comms    |
|  - Source of truth         |   |  - Background processing |
|                            |   |  - Sync/replication      |
+----------------------------+   +-------------------------+
```

**Key principle:** The write path is synchronous and confirmed. JetStream carries the consequences of writes, not the writes themselves.

- Offline mode works — writes go straight to archive, events queue locally
- Background processors are JetStream consumers that run continuously
- Multi-device sync = replaying JetStream output streams
- The API always tells the truth — if it returns 201, the data is stored

## NATS/JetStream Role

- Event sourcing and data-driven design
- Event-oriented, modular code
- Multi-instance sync
- Multiple sources, sinks, data transformers
- JetStream for durable, persistent messaging (not fire-and-forget)
- Embedded in the Upspeak binary
