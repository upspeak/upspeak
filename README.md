# Upspeak

Upspeak is a personal knowledge management system and information client designed to collect, organise, and synthesise data from web sources and your own inputs.

![Project Status: Early Stage](https://img.shields.io/badge/status-early%20stage-yellow)
![Not Production Ready](https://img.shields.io/badge/production%20ready-no-red)
[![Build and Test](https://github.com/upspeak/upspeak/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/upspeak/upspeak/actions/workflows/build-and-test.yml)

## Overview

Upspeak is an **API-first** knowledge infrastructure. It provides a structured knowledge graph for collecting, organising, and querying information from diverse sources. Clients (web, mobile, CLI, AI agents) connect over the HTTP API.

**Key features:**
- Knowledge graph with nodes, edges, threads, and annotations
- UUID v7 identifiers with human-friendly short IDs (`NODE-42`, `REPO-1`)
- Event-driven architecture with embedded NATS/JetStream
- Reusable filters, connectors, schedules, and rules
- Multi-device sync with conflict resolution
- Local-first: offline writes succeed immediately, sync when reconnected

## Architecture

Upspeak uses a **hybrid synchronous core + JetStream** architecture:
- Writes go synchronously to SQLite (confirmed to the client)
- JetStream carries the downstream consequences (events, sync, background processing)

See `docs/specs/api-foundation/` for the complete API specification.

## Develop

```bash
# Build the binary
./build.sh build

# Development mode (requires upspeak.yaml)
cp upspeak.sample.yaml upspeak.yaml
./build.sh dev

# Run tests
go test ./...

# Clean build artifacts
./build.sh cleanup
```

## Project Structure

```
app/        — Application micro-framework (HTTP routing, module lifecycle)
api/        — API response envelope and HTTP helpers
core/       — Domain models and interfaces
archive/    — SQLite storage implementation
repo/       — Repository CRUD API module
nats/       — NATS/JetStream infrastructure (isolated from other packages)
```

## License

Upspeak is licensed under the Apache License, Version 2.0 (Apache-2.0). See the [LICENSE](LICENSE) file for the full license text.
