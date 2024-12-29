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

myApp.AddModule(&ExampleModule{})
```

5. **Start the App**

```go
if err := myApp.Start(); err != nil {
    log.Fatalf("Failed to start app: %v", err)
}
```

Based on the code above, the `example` module's endpoints will now be mounted at `GET http://localhost:8080/example/hello`.

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

## Future Enhancements

- Add support for SQL DBs, object stores, and KV stores.
- Enable dynamic reconfiguration.
- Improve observability with metrics endpoints.
- Enhance lifecycle management for module reliability.

## License

This package is part of the Upspeak project and follows its licensing terms.
