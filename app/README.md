# `app` Package

## Goals

The `app` package is a foundational micro-framework for building and running the Upspeak application. Its primary aims are:

1. **Modularity**: Enable declarative, event-driven modules for tasks like HTTP endpoints and message handling.
2. **Infrastructure Abstraction**: Simplify usage of messaging (NATS) and API services (HTTP).
3. **Ease of Development**: Provide a consistent developer experience with clear abstractions and configurations.
4. **Standalone**: Offer lightweight deployments via embedded NATS while supporting external setups.

## Non-Goals

1. **Standalone Framework**: It is tailored for Upspeak, not as a general-purpose framework.
2. **Dynamic Reconfiguration**: Relies on conventions like YAML files, without advanced runtime reconfiguration.

## Features

- **Modular Architecture**: Manage independent modules.
- **HTTP Server**: Built-in support for HTTP endpoints.
- **NATS Integration**:
  - Supports embedded and external NATS servers.
  - Provides publisher and subscriber abstractions.
- **Health Probes**: Designed for containerized environments.
- **Structured Configurations**: Uses `viper` for YAML, environment variables, and defaults.

## Structure

### Core Types

1. **App**: Manages modules, HTTP servers, and NATS connections.
2. **Module**: Interface for modular components with HTTP and NATS handlers.
3. **Publisher**: Handles message publication.
4. **Config**: Encapsulates app configuration, including NATS and HTTP settings.

### Key Components

- **Health/Readiness Endpoints**:
  - `/healthz`: Returns 200 if operational.
  - `/readiness`: Returns 200 if ready, otherwise 503.
- **Embedded NATS Server**: Runs an in-process instance.
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

6. **Stop the App**

```go
if err := myApp.Stop(); err != nil {
    log.Printf("Failed to stop app: %v", err)
}
```

### Health and Readiness Probes

- `/healthz`: Basic liveness check.
- `/readiness`: Readiness check; returns 503 if not ready.

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
