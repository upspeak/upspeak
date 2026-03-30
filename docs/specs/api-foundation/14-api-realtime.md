# Real-time API (WebSocket)

| Protocol | Path | Purpose |
|----------|------|---------|
| `WS` | `/api/v1/ws` | WebSocket connection |

## Client to Server Messages

### Subscribe

```json
{
  "action": "subscribe",
  "channel": "repos.research.events",
  "filter": {
    "event_type": ["NodeCreated", "AnnotationCreated"],
    "node_type": ["article"]
  }
}
```

Filter is optional. Without it, all events on the channel are delivered.

### Unsubscribe

```json
{ "action": "unsubscribe", "channel": "repos.research.events" }
```

## Server to Client Messages

```json
{
  "channel": "repos.research.events",
  "event": {
    "id": "01964d2e-...",
    "type": "NodeCreated",
    "data": { ... },
    "timestamp": "2026-03-30T10:00:00Z"
  }
}
```

## Available Channels

| Channel pattern | Events |
|----------------|--------|
| `repos.{repo_ref}.events` | All events in a repo |
| `repos.{repo_ref}.nodes.{node_ref}` | Changes to a specific node |
| `repos.{repo_ref}.threads.{thread_ref}` | Changes to a thread (node added/removed, updates) |
| `repos.{repo_ref}.rules.{rule_ref}.actions` | When a rule fires |
| `jobs.{job_ref}` | Job status changes |
| `sync` | Sync status changes and conflict notifications |

## Authentication

WebSocket connections are authenticated during the HTTP upgrade handshake using the same authentication mechanism as REST endpoints (bearer token in query param or header). Unauthenticated upgrade requests receive a `401 Unauthorized` HTTP response before the WebSocket connection is established.

## Connection Management

- Server sends `ping` frames every 30 seconds; client must respond with `pong`
- Connections that fail 3 consecutive pings are terminated
- Max 10 subscriptions per connection
- Max 5 concurrent connections per user
- If client cannot consume events fast enough, server buffers up to 1000 messages; beyond that, oldest messages are dropped and a `messages_dropped` notification is sent

## Error Messages

Errors are delivered to the client as JSON messages on the WebSocket connection:

```json
{
  "type": "error",
  "code": "invalid_channel|subscription_limit|authentication_failed",
  "message": "..."
}
```

## Notes

- Each WebSocket subscription maps to a JetStream consumer under the hood
- Clients can hold multiple subscriptions on a single connection
- Subscriptions with filters reduce traffic — filtering happens server-side before delivery
- Connection drops are handled gracefully — clients can reconnect and resubscribe
