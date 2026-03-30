# App Package

The `app` package is a lightweight micro-framework for composing and running the Upspeak application from modular components.

## Key Concepts

- **App** — Composes modules, manages HTTP routing with namespaced module routes, and handles the application lifecycle
- **Module** — Interface for modular components with HTTP and message handlers
- **Publisher** — Interface for publishing messages to the event bus (implemented by the `nats` package)
- **Subscriber** — Interface for subscribing to messages (implemented by the `nats` package)

## Module Interface

```go
type Module interface {
    Name() string
    Init(config map[string]any) error
    HTTPHandlers() []HTTPHandler
    MsgHandlers() []MsgHandler
}
```

Modules receive their dependencies (archive, publisher) via setter methods, not through handler parameters.

## NATS Isolation

The `app` package has **no NATS imports**. All NATS code lives in the dedicated `nats/` package. The app receives a `Subscriber` for registering message handlers and modules receive a `Publisher` for event publishing — both are interfaces defined in `app`.

## Module Mounting

Multiple modules can share the same mount path (e.g. `/api/v1`). Handler registration uses method+path patterns on `http.ServeMux`, so there is no conflict as long as handlers use different method+path combinations.

```go
up.AddModuleOnPath(&repo.Module{}, "/api/v1")
up.AddModuleOnPath(&filter.Module{}, "/api/v1")  // OK: shared path
```

## Health and Readiness

- `GET /healthz` — Returns 200 OK if operational
- `GET /readiness` — Returns 200 READY if ready, 503 NOT READY otherwise
