# Copilot Instructions for Upspeak

## Architecture Overview

Upspeak is a personal knowledge management system using **modular, event-driven architecture**:

- **Modules** implement `app.Module` interface with HTTP and NATS handlers
- **Event sourcing** via embedded NATS server for inter-module communication
- **Single binary deployment** with frontend embedded via `go:embed`
- **Domain-driven design** with clear separation between `core/` (domain) and infrastructure

**Key packages:**
- `app/`: Micro-framework for module composition, HTTP/NATS servers, lifecycle management
- `core/`: Domain models (Node, Edge, Repository, Events) with CQRS-style event handling
- `ui/`: SvelteKit SPA bundled as Go embedded filesystem

## Critical Build & Dev Workflows

Use `build.sh` exclusively - **never run `go build` or `npm build` directly**:

```bash
./build.sh build       # Full build (ui + binary)
./build.sh build-ui    # Rebuild frontend only
./build.sh dev         # Run with hot reload (requires upspeak.yaml)
go test ./...          # Run all tests
```

**First-time setup:** `cp upspeak.sample.yaml upspeak.yaml` before running `./build.sh dev`

**Frontend dev with HMR:** `cd ui/web && npm run dev` (runs on :5173, proxy to :8080 for API)

## Module Development Patterns

All modules must implement the `app.Module` interface. See `ui/ui.go` for reference:

```go
type Module interface {
    Name() string
    Init(config map[string]any) error
    HTTPHandlers(pub Publisher) []HTTPHandler
    MsgHandlers(pub Publisher) []MsgHandler
}
```

**Module registration:** Use `app.AddModuleOnPath(module, path)` where path can be:
- `""` for root (only one module allowed at root)
- `"/api"` for namespaced routes
- Paths auto-normalize (leading slash optional, trailing slash removed)

**Reserved paths:** Never mount at `/healthz` or `/readiness` (system endpoints)

## Event-Driven Communication

**Module NATS subjects:** Modules define their own subject namespaces:
```go
// Example from a module's MsgHandlers
MsgHandler{
    Subject: "writer.events",      // Module-scoped namespace
    Handler: m.handleEvent,
}
```

**Repository pattern (core/ domain):** Each `Repository` uses dedicated subjects:
- `repos.{id}.in` - receives operation events (EventCreateNode, etc.)
- `repos.{id}.out` - publishes result events (EventNodeCreated, etc.)

```go
// Input events trigger operations
EventCreateNode -> Repository.handleInputEvent() -> Archive.SaveNode()

// Output events broadcast results
EventNodeCreated published to repos.{id}.out
```

See `core/repo.go` and `core/events.go` for event types and payloads.

## Code Conventions (Strict)

**Language:** British English (en-IN) in all docs and comments: "organise", "behaviour"

**Error handling:**
- Use custom error types in `core/errors.go` (e.g., `ErrorNotFound`)
- Wrap errors with context: `fmt.Errorf("failed to save: %w", err)`
- Check errors immediately, never use `panic` for normal errors

**Naming:**
- Constructors: `New()` or `New<Type>()`
- Private helpers: camelCase (`normalisePath`, `handleInputEvent`)
- Short vars: `err`, `nc`, `ctx`, `req`, `w`, `r`
- Single-letter receivers: `a *App`, `r *Repository`

**Documentation:**
- GoDoc-style comments for all public functions/types
- Include examples for complex APIs (see `app.AddModuleOnPath`)
- Comment private methods >20 lines

**Function design:**
- Target <30 lines per function
- Use early returns to reduce nesting
- Extract complex logic to private helpers

## Testing Patterns

**Co-located tests:** Place `*_test.go` alongside implementation

**Table-driven tests:** Use `tests []struct{...}` with `t.Run(tt.name, ...)` (see `app/module_test.go`)

**Mock modules:** See `app/module_test.go` for `mockModule` pattern

**Test embedded assets:** Verify `//go:embed` directives work (see `ui/ui_test.go`)

## Frontend Integration (ui module)

**Embedding strategy:**
- `//go:embed web/build/*` - SvelteKit production build
- `//go:embed web/static/*` - Static assets (favicon, robots.txt)
- Handler order: `/_app/` assets → static files → SPA fallback (`/`)

**SPA routing:** All unmatched routes serve `index.html` for client-side routing

**Build requirement:** Run `cd ui/web && npm run build` before `go build` to populate embed directives

## Domain Model Notes

**Core types:**
- `Node`: Knowledge graph unit with arbitrary JSON in `Body` (uses `json.RawMessage` for flexibility)
- `Edge`: Relationship between Nodes (source/target with type, label, weight)
- `Thread`: Hierarchical discussion structure (composition of Nodes + sub-Threads)
- `Annotation`: Web Annotations API-compliant metadata

**Design tradeoffs documented in `core/README.md`:**
- `json.RawMessage` trades type safety for flexibility (performance impact noted)
- Repository-scoped Archives ensure isolation but complicate cross-repo queries

## Configuration

**YAML-based:** See `upspeak.sample.yaml` for structure

```yaml
name: "upspeak"
nats:
  embedded: true   # Run in-process NATS server
  private: false   # Allow external connections
http:
  port: 8080
modules:
  writer:
    enabled: true
    config: {...}
```

**Module config:** Passed to `Module.Init(config map[string]any)` from YAML `modules.<name>.config`

## File Organisation

- New modules in repo root (e.g., `ui/`, future `writer/`)
- Type definitions before functions
- One file per major concern
- Private helpers at bottom of file

## Common Pitfalls

1. **Don't build manually** - use `build.sh` to ensure frontend embeds correctly
2. **Module paths** - remember root path is `""` not `"/"`
3. **Event payloads** - unmarshal to specific payload types (e.g., `EventNodeCreatePayload`)
4. **NATS subjects** - modules use their own namespaces (e.g., `"writer.events"`). Only Repository uses `repos.{id}.in/out`
5. **Embedded FS paths** - use `"web/build"` prefix when accessing `buildFS.ReadFile()`
