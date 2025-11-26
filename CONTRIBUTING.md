# Contributing to Upspeak

Thank you for your interest in contributing to Upspeak! This document provides guidelines and standards for contributing to the project.

## Getting Started

### Prerequisites

- **Go:** Version 1.21 or later
- **Node.js:** Version 22 or later (for UI module development)
- **Git:** For version control

### Building the Project

Upspeak uses a build script that handles module builds and binary compilation:

```bash
# Clone the repository
git clone https://github.com/upspeak/upspeak.git
cd upspeak

# Full build (modules + binary)
./build.sh build

# Run tests
go test ./...

# Start in development mode
cp upspeak.sample.yaml upspeak.yaml
# Edit upspeak.yaml with your configuration
./build.sh dev
```

See the [README](README.md#develop) for more build commands.

## Development Workflow

1. **Fork the repository** and create a feature branch
2. **Make your changes** following the coding standards
3. **Write tests** for new functionality
4. **Run tests** to ensure nothing breaks
5. **Format code** with `gofmt` and `go vet`
6. **Commit** with clear, descriptive messages
7. **Push** to your fork and create a pull request

## Project Architecture

Upspeak uses a modular architecture where modules are composed into a single binary:

### Core Concepts

- **Modules:** Independent components implementing the `app.Module` interface
- **Event-driven:** Modules communicate via embedded NATS server
- **Domain-driven design:** Clear separation between domain and infrastructure layers
- **Single binary:** All modules embedded at compile time using Go's `embed` package

### Module Communication

Modules communicate through:
1. **HTTP handlers:** For external requests
2. **NATS messages:** For internal event-driven communication

Example NATS subject pattern:
- Input events: `repos.{id}.in`
- Output events: `repos.{id}.out`

## Coding Standards

### Go Code Style

All Go code must follow these standards:

#### File Organisation

- **Logical separation:** One file per major concern
- **Type definitions first:** Define types before functions
- **Private helpers:** Use lowercase names for unexported functions
- **Co-located tests:** Place `*_test.go` files alongside implementation

Example:
```go
package mymodule

// Types first
type Handler struct {
	logger *slog.Logger
	config Config
}

// Public constructors and methods
func New(config Config) *Handler { ... }

func (h *Handler) Handle() error { ... }

// Private helpers
func normalisePath(path string) string { ... }
```

#### Naming Conventions

- **Types:** PascalCase for exported (`Node`, `Repository`, `ErrorNotFound`)
- **Functions:** PascalCase for exported (`LoadConfig`), camelCase for private (`normalisePath`)
- **Variables:** Short for common patterns (`err`, `ctx`), descriptive for complex types (`modules`, `inputEvent`)
- **Receivers:** Single letter for simple types (`a *App`, `r *Repository`)
- **Constants:** Typed with semantic grouping

```go
type EventType string

const (
	// Input events
	EventCreateNode EventType = "CreateNode"
	EventUpdateNode EventType = "UpdateNode"
)
```

#### Documentation

All public APIs must have GoDoc-style comments. Use en-IN (Indian English with British spelling: "organise", "behaviour").

**Function documentation:**
```go
// AddModuleOnPath registers a module at the specified path.
//
// Path rules:
//   - Empty string "" mounts at root (/)
//   - Leading slash is optional and will be normalised
//   - Only one module can be mounted at root
//
// Examples:
//
//	app.AddModuleOnPath(&ui.Module{}, "")         // Root: /
//	app.AddModuleOnPath(&api.Module{}, "/api")    // Namespaced: /api/*
func (a *App) AddModuleOnPath(module Module, path string) error
```

**Type documentation:**
```go
// Config defines the application configuration.
type Config struct {
	// Name of the application. Use only lowercase letters, dashes and underscores.
	Name    string `mapstructure:"name"`
	// NATS server configuration
	NATS    NATSConfig `mapstructure:"nats"`
}
```

#### Error Handling

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

#### Function Design

- **Prefer small functions:** Target under 30 lines for most functions
- **Single responsibility:** Each function should do one thing well
- **Extract helpers:** Break complex logic into smaller functions
- **Early returns:** Reduce nesting depth

**Good example:**
```go
func (a *App) AddModule(module Module) error {
	if err := a.validateModule(module); err != nil {
		return err
	}

	if err := a.registerModule(module); err != nil {
		return err
	}

	return nil
}
```

#### Go Idioms

Follow standard Go idioms:

- **Pointer receivers** for methods that modify state
- **Interface-based design** for abstraction
- **Dependency injection** via constructors
- **Error as last return value**
- **Defer for cleanup** (Close, cancel, etc.)
- **Goroutines with error channels** for async operations
- **Structured logging** with `log/slog`

**Example:**
```go
// Interface for abstraction
type Archive interface {
	SaveNode(node *Node) error
	GetNode(id xid.ID) (*Node, error)
}

// Dependency injection
func NewRepository(archive Archive, natsConn *nats.Conn) *Repository {
	return &Repository{
		archive:  archive,
		natsConn: natsConn,
	}
}

// Goroutine with error channel
serverErr := make(chan error, 1)
go func() {
	if err := server.ListenAndServe(); err != nil {
		serverErr <- err
	}
	close(serverErr)
}()
```

## Module Development

### Creating a New Module

Modules must implement the `app.Module` interface:

```go
type Module interface {
	Name() string
	Init(config map[string]any) error
	HTTPHandlers(pub Publisher) []HTTPHandler
	MsgHandlers(pub Publisher) []MsgHandler
}
```

**Example module:**
```go
package mymodule

import "github.com/upspeak/upspeak/app"

type ModuleExample struct {
	config Config
}

func (m *ModuleExample) Name() string {
	return "example"
}

func (m *ModuleExample) Init(config map[string]any) error {
	// Initialise module with configuration
	return nil
}

func (m *ModuleExample) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
	return []app.HTTPHandler{
		{
			Method:  "GET",
			Path:    "/example",
			Handler: m.handleExample,
		},
	}
}

func (m *ModuleExample) MsgHandlers(pub app.Publisher) []app.MsgHandler {
	return []app.MsgHandler{
		{
			Subject: "example.events",
			Handler: m.handleEvent,
		},
	}
}
```

### Registering Your Module

Add your module to `main.go`:

```go
import "github.com/upspeak/upspeak/mymodule"

// In main()
app.AddModuleOnPath(&mymodule.ModuleExample{}, "/mymodule")
```

## UI Module (SvelteKit)

The UI module uses SvelteKit with TypeScript and is embedded in the Go binary.

### Development with Hot Reload

```bash
cd ui/web
npm install  # First time only
npm run dev
```

Access the dev server at `http://localhost:5173` with hot module replacement.

### Building for Production

```bash
cd ui/web
npm run build
```

This generates static files in `ui/web/build/` which are embedded in the Go binary.

### Adding New Routes

SvelteKit uses file-based routing:

```
ui/web/src/routes/
├── +page.svelte              # / (homepage)
├── about/
│   └── +page.svelte          # /about
└── settings/
    └── profile/
        └── +page.svelte      # /settings/profile
```

### Static Assets

Place static files in `ui/web/static/`:

```
ui/web/static/
├── favicon.png
├── robots.txt
└── logo.svg
```

These are copied to the build directory and served at root paths.

See [ui/README.md](ui/README.md) for detailed UI module documentation.

## Testing Guidelines

### Writing Tests

Use table-driven tests for comprehensive coverage:

```go
func TestNormalisePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "Removes trailing slash",
			input:    "/api/",
			expected: "/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalisePath(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
```

### Mock Implementations

Create mocks for testing interfaces:

```go
type mockArchive struct {
	nodes map[xid.ID]*Node
	err   error
}

func (m *mockArchive) SaveNode(node *Node) error {
	if m.err != nil {
		return m.err
	}
	m.nodes[node.ID] = node
	return nil
}
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./app/...

# Verbose output
go test -v ./...
```

## Pull Request Process

1. **Create a feature branch** from `main`
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make your changes** following the coding standards

3. **Write tests** for new functionality

4. **Run tests and linters**
   ```bash
   go test ./...
   go vet ./...
   gofmt -s -w .
   ```

5. **Commit with clear messages**
   ```bash
   git commit -m "feat: add new feature description"
   ```

   Use conventional commit prefixes:
   - `feat:` New features
   - `fix:` Bug fixes
   - `docs:` Documentation changes
   - `test:` Test additions or updates
   - `refactor:` Code refactoring
   - `chore:` Build process or tooling changes

6. **Push to your fork**
   ```bash
   git push origin feature/my-feature
   ```

7. **Create a pull request** with:
   - Clear title and description
   - Reference to related issues
   - Summary of changes
   - Test results

### Code Review Expectations

- Code must pass all tests
- Follow project coding standards
- Include appropriate documentation
- Address reviewer feedback promptly

## Additional Resources

### Project Documentation

- [Main README](README.md) - Project overview and development setup
- [App Module](app/README.md) - Application framework documentation
- [Core Module](core/README.md) - Domain model and event sourcing
- [UI Module](ui/README.md) - Frontend development guide

### External Resources

- [Go Documentation](https://go.dev/doc/)
- [Effective Go](https://go.dev/doc/effective_go)
- [SvelteKit Documentation](https://kit.svelte.dev/docs)
- [NATS Documentation](https://docs.nats.io/)

## Licence

By contributing to Upspeak, you agree that your contributions will be licenced under the [Apache License 2.0](LICENSE).

## Questions?

If you have questions about contributing:
- Open an issue with the `question` label
- Refer to existing module implementations for examples
- Check the [discussions](https://github.com/upspeak/upspeak/discussions) section

We appreciate your contributions to Upspeak!
