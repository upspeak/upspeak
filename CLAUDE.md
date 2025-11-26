# Claude Instructions for Upspeak

## Project Overview

Upspeak is a personal knowledge management system and information client designed to collect, organise, and synthesise data from web sources and your own inputs. It follows a modular, event-driven architecture built on domain-driven design principles.

**Architecture:**
- **Modular design**: Each module implements the `app.Module` interface for HTTP and NATS handlers
- **Event sourcing**: Domain events published via embedded NATS server for inter-module communication
- **Hexagonal architecture**: Clear separation between domain layer (`core/`) and infrastructure
- **Single binary deployment**: All modules embedded at compile time using Go's `embed` package
- **Knowledge graph**: Nodes and Edges represent data points and their relationships

**Key packages:**
- `app/`: Lightweight micro-framework for composing modules, managing HTTP/NATS servers, and application lifecycle
- `core/`: Domain models (Node, Edge, User, Repository) and business logic
- `ui/`: The UI module that bundles and embeds a Sveltekit application.

## Critical Rules

1. **ALWAYS** follow patterns established in `app/` and `core/` packages
2. **ALWAYS** add GoDoc-style comments for all public functions and types
3. **ALWAYS** add comments for longer private methods (>20 lines)
4. **ALWAYS** write documentation in en-IN (Indian English with British spelling: "organise", "behaviour", "colour")
5. **NEVER** respond with summaries unless explicitly requested
6. **NEVER** skip error handling - check and handle all errors immediately
7. **NEVER** use `panic` for normal error conditions
8. **NEVER** create deep nesting - extract functions or use early returns

## Build Commands

```bash
# Full build (modules + binary)
./build.sh build

# Build ui module only
./build.sh build-ui

# Build binary only
./build.sh build-app

# Development mode (requires upspeak.yaml)
./build.sh dev

# Clean artifacts
./build.sh cleanup

# Run tests
go test ./...
```

## File Organisation

- **Logical separation**: One file per major concern or responsibility
- **Type definitions first**: Define types before functions that use them
- **Private helpers**: Use lowercase names for unexported functions
- **Co-located tests**: Place `*_test.go` files alongside implementation
- **New module location**: New modules are placed in the repo root directory, unless specifically asked to be put elsewhere.

Example structure:
```go
package mymodule

// Types first
type Config struct { ... }
type Handler struct { ... }

// Public functions
func New(config Config) *Handler { ... }

func (h *Handler) PublicMethod() error { ... }

// Private helpers
func normalisePath(path string) string { ... }
```

## Naming Conventions

**Types:**
- PascalCase for exported types: `Node`, `Edge`, `Repository`, `ErrorNotFound`
- Descriptive names: `HTTPHandler`, `MsgHandler`, `EventType`

**Functions:**
- PascalCase for exported: `New()`, `LoadConfig()`, `AddModuleOnPath()`
- camelCase for private: `normalisePath()`, `isReservedPath()`, `handleInputEvent()`
- Constructor pattern: `New<Type>()`

**Variables:**
- Short for common patterns: `err`, `nc`, `ctx`, `req`, `w`, `r`
- Descriptive for complex types: `modules`, `rootModule`, `inputEvent`
- Single-letter receivers: `a *App`, `r *Repository`, `p *Publisher`

**Constants:**
- Use typed constants with semantic grouping:
```go
type EventType string

const (
	// Input events
	EventCreateNode EventType = "CreateNode"
	EventUpdateNode EventType = "UpdateNode"

	// Output events
	EventNodeCreated EventType = "NodeCreated"
	EventNodeUpdated EventType = "NodeUpdated"
)
```

## Documentation Patterns

**Function documentation:**
```go
// AddModuleOnPath registers a module at the specified path.
//
// Path rules:
//   - Empty string "" mounts at root (/)
//   - Leading slash is optional and will be normalised
//   - Trailing slashes are removed
//   - Only one module can be mounted at root
//
// Examples:
//
//	app.AddModuleOnPath(&ui.Module{}, "")         // Root: /
//	app.AddModuleOnPath(&api.Module{}, "/api")    // Namespaced: /api/*
func (a *App) AddModuleOnPath(module Module, path string) error {
```

**Type documentation:**
```go
// Config defines the application configuration.
// Fields are populated from YAML files via mapstructure tags.
type Config struct {
	// Name of the application. Use only lowercase letters, dashes and underscores.
	Name string `mapstructure:"name"`
	// NATS server configuration
	NATS NATSConfig `mapstructure:"nats"`
}
```

## Error Handling

**Use custom error types for domain errors:**
```go
type ErrorNotFound struct {
	resource string
	msg      string
}

func (e *ErrorNotFound) Error() string {
	return fmt.Sprintf("not found error: Could not find %s. Message: %s", e.resource, e.msg)
}
```

**Wrap errors with context:**
```go
if err := r.archive.SaveNode(node); err != nil {
	return fmt.Errorf("failed to save node: %w", err)
}
```

**Check errors immediately:**
```go
if err != nil {
	return err
}
```

## Function Design

- **Prefer small functions**: Target under 30 lines for most functions
- **Single responsibility**: Each function should do one thing well
- **Extract helpers**: Break complex logic into smaller private functions
- **Early returns**: Reduce nesting depth with guard clauses

**Good example:**
```go
func (a *App) AddModule(module Module) error {
	if err := a.validateModule(module); err != nil {
		return err
	}
	
	path := "/" + module.Name()
	return a.AddModuleOnPath(module, path)
}
```

## Module Development

All modules must implement the `app.Module` interface:

```go
type Module interface {
	Name() string
	Init(config map[string]any) error
	HTTPHandlers(pub Publisher) []HTTPHandler
	MsgHandlers(pub Publisher) []MsgHandler
}
```

**HTTP Handler structure:**
```go
HTTPHandler struct {
	Method  string           // "GET", "POST", etc.
	Path    string           // Handler path relative to module mount
	Handler http.HandlerFunc // Standard http.HandlerFunc
}
```

**NATS Message Handler structure:**
```go
MsgHandler struct {
	Subject string                // NATS subject pattern
	Handler func(msg *nats.Msg)   // Message handling function
}
```

**Module mounting rules:**
- Empty string `""` or `"/"` mounts at root
- Leading slash is optional and normalised automatically
- Trailing slashes are removed
- Only one module can be mounted at root
- Paths cannot conflict with reserved endpoints (`/healthz`, `/readiness`)
- Root module handlers are registered last for proper catch-all routing

## NATS Communication Patterns

**Repository subjects:**
- Input events: `repos.{id}.in` - for commands/operations
- Output events: `repos.{id}.out` - for event notifications

**Publishing events:**
```go
event, err := NewEvent(EventNodeCreated, EventNodeCreatePayload{Node: node})
if err != nil {
	return &ErrorEventCreation{msg: "EventNodeCreated"}
}
if err := r.publishEvent(event); err != nil {
	return &ErrorPublish{msg: "EventNodeCreated"}
}
```

## HTTP Routing (Go 1.22+ Pattern Matching)

**CRITICAL:** Always use method-specific patterns to avoid conflicts:

```go
// Good - method-specific
a.httpRouter.HandleFunc("GET /healthz", handler)
a.httpRouter.HandleFunc("POST /api/nodes", handler)

// Bad - method-agnostic (can conflict with root handlers)
a.httpRouter.HandleFunc("/healthz", handler)
```

**Pattern conflict rules:**
- Method-agnostic patterns match all HTTP methods
- More specific paths with broader methods conflict with general paths
- Always specify methods for system endpoints (`GET /healthz`, `GET /readiness`)

## Testing Standards

- Write table-driven tests for multiple cases
- Use meaningful test names: `TestAddModuleOnPath_RootMount`
- Test error cases and edge conditions
- Mock external dependencies (NATS, HTTP servers)
- Co-locate test files with implementation

```go
func TestNormalisePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"root slash", "/", ""},
		{"without leading slash", "api", "/api"},
		{"with trailing slash", "/api/", "/api"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalisePath(tt.input)
			if got != tt.expected {
				t.Errorf("normalisePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
```

## Domain Models (core/)

**Node**: Basic unit of information in knowledge graph
- `ID`: Unique identifier (xid.ID)
- `Type`: Node type/category
- `Subject`: Brief description
- `ContentType`: MIME type of body content
- `Body`: JSON-encoded content
- `Metadata`: Key-value pairs for additional data
- `CreatedBy`, `CreatedAt`: Audit fields

**Edge**: Relationship between nodes
- `Source`, `Target`: Node IDs being related
- `Type`: Relationship type ("reply", "annotation", etc.)
- `Label`: Human-readable relationship description
- `Weight`: Importance/strength of relationship

**Repository**: Domain aggregate for managing nodes/edges
- Encapsulates archive storage
- Handles NATS pub/sub for events
- Processes input commands and emits output events

## Two-Phase Module Registration

Register modules in two phases for correct routing priority:

```go
// First pass: Register all non-root modules
for name, mount := range a.modules {
	if mount.path == "" {
		continue
	}
	if err := a.registerModule(name, mount, pub); err != nil {
		return err
	}
}

// Second pass: Register root module last (gives it catch-all priority)
if a.rootModule != "" {
	mount := a.modules[a.rootModule]
	if err := a.registerModule(a.rootModule, mount, pub); err != nil {
		return err
	}
}
```

## Workflow Best Practices

**When making changes:**
1. **Read relevant files first** - understand context before coding
2. **Make a plan** - outline approach before implementation
3. **Ask for confirmation** - verify plan before proceeding
4. **Implement incrementally** - make small, verifiable changes
5. **Test as you go** - run tests after each significant change
6. **Commit logically** - group related changes together

**When exploring codebase:**
- Use semantic search for concepts and patterns
- Read git history to understand design decisions
- Check tests to understand expected behaviour
- Look for similar implementations as templates

**When debugging:**
- Check error messages carefully
- Read relevant test files
- Examine git history for related changes
- Consider edge cases and error paths

## Code Review Checklist

Before submitting code, ensure:
- [ ] All public functions have GoDoc comments
- [ ] Error handling follows project patterns
- [ ] Tests added for new functionality
- [ ] Code formatted with `gofmt`
- [ ] No lint warnings from `go vet`
- [ ] Function complexity is reasonable (under 30 lines for most)
- [ ] Proper use of interfaces for abstraction
- [ ] Thread-safe access to shared state
- [ ] Documentation uses en-IN language
- [ ] HTTP handlers specify methods explicitly

## Common Patterns

**Path normalisation:**
```go
func normalisePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}
```

**Logging for root paths:**
```go
logPath := mount.path
if logPath == "" {
	logPath = "/"
}
a.logger.Info("Initialising module", "module", name, "path", logPath)
```

## Getting Help

- Check `app/README.md` for framework details
- See `core/README.md` for domain model documentation
- Read `CONTRIBUTING.md` for contribution guidelines
- Examine existing tests for usage examples
- Review git history for design rationale
