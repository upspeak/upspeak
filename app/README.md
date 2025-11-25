# `app` Package

## Goals

The `app` package is a lightweight, modular, foundational micro-framework based on [NATS](https://nats.io) for building and running the Upspeak application. Its primary aims are:

1. **Composable; Modular**: Enable declarative, event-driven modules. Compose an application out of these modules, without the modules having to worry about macro-architectural decisions.
2. **Flexible deployment**: Produce a single binary. Deploy as standalone, or run specific modules to create distributed deployments. Offer lightweight deployments via embedded NATS while supporting external NATS setups.
3. **Simplify till it hurts, without compromising functionality**: Thanks, NATS, for making _this_ possible.

## Important note

1. This package is tailored for Upspeak. _It is not intended to be a general-purpose framework_. Avoid dependending on this package for other projects. Copy/Fork if you like it that much.
2. Relies on configurations from YAML/JSON files, without advanced runtime reconfiguration, at least for now.
3. Not intended to support multiple message queues to keep the intra-system and inter-module communication simple; embracing NATS. Offload as much of the requirements to NATS as possible.

## Key concepts

- **App**: Composes modules, and manages HTTP servers with namespaced module routes, and NATS connections. Responsible for the entire application lifecycle.
- **Module**: Interface for modular components with HTTP and NATS handlers.
- **Publisher**: Handles message publication.
- **Config**: Encapsulates app configuration, including NATS and HTTP settings.
- **Embedded NATS Server**: Runs an in-process instance.
- **Health/Readiness Endpoints**:
  - `/healthz`: Returns 200 if operational.
  - `/readiness`: Returns 200 if ready, otherwise 503.
- **Lifecycle Management**:
  - `Start`: Initializes modules, NATS, and HTTP server.
  - `Stop`: Gracefully shuts down components.

## Usage

### Setting Up the App

1. **Create a Config File**

Example `config.yaml`:

```yaml
name: "upspeak"
nats:
  embedded: true
  private: false
  logging: true
http:
  port: 8080
modules:
  example:
    enabled: true
    config:
      key: "value"
```

2. **Load Configuration**

```go
config, err := app.LoadConfig("config.yaml")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}
```

3. **Initialize the App**

```go
myApp := app.New(*config)
```

4. **Add Modules**

Modules implement the `Module` interface. Example:

```go
type ExampleModule struct{}

func (m *ExampleModule) Name() string {
    return "example"
}

func (m *ExampleModule) Init(config map[string]any) error {
    return nil
}

func (m *ExampleModule) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
    return []app.HTTPHandler{
        {
            Method: "GET",
            Path:   "/hello",
            Handler: func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprintln(w, "Hello, World!")
            },
        },
    }
}

func (m *ExampleModule) MsgHandlers(pub app.Publisher) []app.MsgHandler {
    return []app.MsgHandler{
        {
            Subject: "example.subject",
            Handler: func(msg *nats.Msg) {
                fmt.Printf("Received message: %s", string(msg.Data))
            },
        },
    }
}

// Add module at default path (/<module-name>/)
if err := myApp.AddModule(&ExampleModule{}); err != nil {
    log.Fatalf("Failed to add module: %v", err)
}
```

Based on the code above, the `example` module's endpoints will now be mounted at `GET http://localhost:8080/example/hello`.

**Alternative: Custom Mount Path**

You can mount modules at custom paths using `AddModuleOnPath`:

```go
// Mount UI at root
if err := myApp.AddModuleOnPath(&UIModule{}, ""); err != nil {
    log.Fatalf("Failed to add UI module: %v", err)
}

// Mount API at /api
if err := myApp.AddModuleOnPath(&APIModule{}, "/api"); err != nil {
    log.Fatalf("Failed to add API module: %v", err)
}

// Mount v1 API (can omit leading slash)
if err := myApp.AddModuleOnPath(&V1Module{}, "v1"); err != nil {
    log.Fatalf("Failed to add v1 module: %v", err)
}
```

**Path Mounting Rules:**

- Empty string `""` or `"/"` mounts at root
- Leading slash is optional and normalized automatically
- Trailing slashes are removed
- Only one module can be mounted at root
- Paths cannot conflict with reserved endpoints (`/healthz`, `/readiness`)
- Root module handlers are registered last for proper catch-all routing

5. **Start the App**

```go
if err := myApp.Start(); err != nil {
    log.Fatalf("Failed to start app: %v", err)
}
```

6. **Stop the App**

```go
if err := myApp.Stop(); err != nil {
    log.Printf("Failed to stop app: %v", err)
}
```

### Health and Readiness Probes

- `GET /healthz`: Basic liveness check.
- `GET /readiness`: Readiness check; returns 503 if not ready.

### Configuration via Environment Variables

- Prefix: `UPSPEAK_`
- Example: `UPSPEAK_HTTP_PORT=9090` sets the HTTP port to 9090.

## Advanced: Custom Module Paths

The framework allows flexible module mounting using `AddModuleOnPath`. This is particularly useful for serving UI applications at root or creating versioned APIs.

### Example: UI at Root with API Modules

```go
func main() {
    config, _ := app.LoadConfig("config.yaml")
    myApp := app.New(*config)

    // Mount UI module at root (/) for clean URLs
    if err := myApp.AddModuleOnPath(&ui.Module{}, ""); err != nil {
        log.Fatal(err)
    }

    // Mount API modules at specific paths
    if err := myApp.AddModuleOnPath(&api.Module{}, "/api"); err != nil {
        log.Fatal(err)
    }
    
    if err := myApp.AddModule(&writer.Module{}); err != nil {
        log.Fatal(err)
    }

    myApp.Start()
}
```

**Resulting URL structure:**
- `GET /` → UI Module (SPA)
- `GET /about` → UI Module
- `GET /api/users` → API Module
- `GET /writer/posts` → Writer Module
- `GET /healthz` → Health check
- `GET /readiness` → Readiness check

### Module Registration Order

The framework registers modules in two passes:

1. **First pass**: All non-root modules are registered
2. **Second pass**: Root module (if any) is registered last

This ensures that specific routes (like `/api/users`) take precedence over catch-all routes (like `/*` for SPA routing).

## Future Enhancements

- Add support for SQL DBs, object stores, and KV stores.
- Enable dynamic reconfiguration.
- Improve observability with metrics endpoints.
- Enhance lifecycle management for module reliability.

## License

This package is part of the Upspeak project and follows its licensing terms.
