# Realtime WebSocket Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a `realtime/` module exposing `GET /api/v1/ws` that fans out domain events from the `repo.*.events.>` subject to subscribed WebSocket clients with server-side filtering.

**Architecture:** A single `app.MsgHandler` on `repo.*.events.>` (core-NATS fan-out, registered by the framework) feeds a `hub`. The hub holds a registry of `connection`s and dispatches each event to connections whose subscriptions match. No JetStream consumer and no new `nats/` code are added. An `Authenticator` seam guards the upgrade (allow-all by default).

**Tech Stack:** Go 1.25, `github.com/coder/websocket`, existing `app.Module` framework, `core.Archive` for ref resolution.

**Design doc:** `docs/superpowers/specs/2026-06-05-realtime-websocket-design.md`
**Spec:** `docs/specs/api-foundation/14-api-realtime.md`

**Conventions reminder:** GoDoc comments on all exported symbols and on private methods >20 lines. en-IN spelling in docs ("organise", "behaviour"). Small commits per task. Check all errors immediately. No nats-io imports outside `nats/`.

---

## Task 1: Add the coder/websocket dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/coder/websocket@v1.8.13
```
Expected: `go.mod` gains `github.com/coder/websocket v1.8.13`.

- [ ] **Step 2: Tidy and verify it resolves**

Run:
```bash
go mod tidy && go build ./...
```
Expected: builds with no errors; `coder/websocket` appears in `go.mod` require block.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add coder/websocket dependency for realtime module"
```

---

## Task 2: Message types and the auth seam

**Files:**
- Create: `realtime/types.go`
- Create: `realtime/auth.go`
- Test: `realtime/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `realtime/auth_test.go`:
```go
package realtime

import (
	"net/http/httptest"
	"testing"
)

func TestAllowAllAuthenticator_PermitsEveryRequest(t *testing.T) {
	var auth Authenticator = allowAllAuthenticator{}
	req := httptest.NewRequest("GET", "/api/v1/ws", nil)

	identity, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if identity != "local" {
		t.Fatalf("expected identity %q, got %q", "local", identity)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestAllowAllAuthenticator -v`
Expected: FAIL — `undefined: Authenticator` / `undefined: allowAllAuthenticator`.

- [ ] **Step 3: Write the types**

Create `realtime/types.go`:
```go
// Package realtime provides the WebSocket module. Clients open a single
// connection at /api/v1/ws, subscribe to channels, and receive a live push of
// matching domain events fanned out from the repo.*.events.> subject.
package realtime

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// Connection and subscription limits (spec 14-api-realtime.md).
const (
	outboundBufferSize  = 1000             // buffered frames per connection
	maxSubscriptions    = 10               // subscriptions per connection
	maxConnsPerIdentity = 5                // connections per authenticated identity
	ingestBufferSize    = 1024             // hub ingest backlog
	pingInterval        = 30 * time.Second // server keepalive interval
	maxPingFailures     = 3                // consecutive missed pongs before close
)

// clientMessage is a control message sent by a client over the socket.
type clientMessage struct {
	Action  string     `json:"action"` // "subscribe" | "unsubscribe"
	Channel string     `json:"channel"`
	Filter  *subFilter `json:"filter,omitempty"`
}

// subFilter narrows which events a subscription delivers. Both fields are
// optional; an absent or empty filter delivers every event on the channel.
type subFilter struct {
	EventType []string `json:"event_type,omitempty"`
	NodeType  []string `json:"node_type,omitempty"`
}

// outboundEvent is the server-to-client envelope for a delivered domain event.
type outboundEvent struct {
	Channel string            `json:"channel"`
	Event   outboundEventBody `json:"event"`
}

// outboundEventBody mirrors the spec's event shape (id/type/data/timestamp).
type outboundEventBody struct {
	ID        uuid.UUID       `json:"id"`
	Type      core.EventType  `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

// errorMessage is the server-to-client error shape.
type errorMessage struct {
	Type    string `json:"type"` // always "error"
	Code    string `json:"code"` // invalid_channel | subscription_limit | authentication_failed
	Message string `json:"message"`
}

// noticeMessage signals a non-fatal condition such as dropped messages.
type noticeMessage struct {
	Type    string `json:"type"` // e.g. "messages_dropped"
	Message string `json:"message"`
}

// Error codes used in errorMessage.
const (
	codeInvalidChannel     = "invalid_channel"
	codeSubscriptionLimit  = "subscription_limit"
	codeAuthenticationFail = "authentication_failed"
)
```

Create `realtime/auth.go`:
```go
package realtime

import "net/http"

// Authenticator authorises a WebSocket upgrade request. The returned identity
// scopes per-user connection limits. The default implementation permits every
// request; real token or identity checks replace it in a later phase.
type Authenticator interface {
	Authenticate(r *http.Request) (identity string, err error)
}

// allowAllAuthenticator permits every upgrade and assigns a single shared
// identity. It is the default until real authentication exists.
type allowAllAuthenticator struct{}

// Authenticate always succeeds with a fixed identity.
func (allowAllAuthenticator) Authenticate(_ *http.Request) (string, error) {
	return "local", nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestAllowAllAuthenticator -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add realtime/types.go realtime/auth.go realtime/auth_test.go
git commit -m "feat(realtime): message types and allow-all auth seam"
```

---

## Task 3: Channel parsing

**Files:**
- Create: `realtime/subscription.go`
- Test: `realtime/subscription_test.go`

Channel grammar (spec §44-53):
- `repos.{repo_ref}.events`
- `repos.{repo_ref}.nodes.{node_ref}`
- `repos.{repo_ref}.threads.{thread_ref}`
- `repos.{repo_ref}.rules.{rule_ref}.actions` (stub)
- `jobs.{job_ref}` (stub)
- `sync` (stub)

- [ ] **Step 1: Write the failing test**

Create `realtime/subscription_test.go`:
```go
package realtime

import "testing"

func TestParseChannel(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		wantKind  channelKind
		wantRepo  string
		wantEnt   string
		wantError bool
	}{
		{"repo events", "repos.research.events", channelRepoEvents, "research", "", false},
		{"node", "repos.research.nodes.NODE-42", channelNode, "research", "NODE-42", false},
		{"thread", "repos.research.threads.THREAD-7", channelThread, "research", "THREAD-7", false},
		{"rule actions stub", "repos.research.rules.RULE-3.actions", channelRuleActions, "research", "RULE-3", false},
		{"job stub", "jobs.JOB-9", channelJob, "", "JOB-9", false},
		{"sync stub", "sync", channelSync, "", "", false},
		{"empty", "", 0, "", "", true},
		{"unknown root", "foo.bar", 0, "", "", true},
		{"repo missing tail", "repos.research", 0, "", "", true},
		{"node missing ref", "repos.research.nodes", 0, "", "", true},
		{"rule missing actions suffix", "repos.research.rules.RULE-3", 0, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChannel(tt.channel)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.channel)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.kind != tt.wantKind {
				t.Errorf("kind: got %v, want %v", got.kind, tt.wantKind)
			}
			if got.repoRef != tt.wantRepo {
				t.Errorf("repoRef: got %q, want %q", got.repoRef, tt.wantRepo)
			}
			if got.entityRef != tt.wantEnt {
				t.Errorf("entityRef: got %q, want %q", got.entityRef, tt.wantEnt)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestParseChannel -v`
Expected: FAIL — `undefined: channelKind` / `undefined: parseChannel`.

- [ ] **Step 3: Write the implementation**

Create `realtime/subscription.go`:
```go
package realtime

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// channelKind enumerates the channel families a client can subscribe to.
type channelKind int

const (
	channelRepoEvents  channelKind = iota // repos.{repo}.events
	channelNode                           // repos.{repo}.nodes.{node}
	channelThread                         // repos.{repo}.threads.{thread}
	channelRuleActions                    // repos.{repo}.rules.{rule}.actions (stub)
	channelJob                            // jobs.{job} (stub)
	channelSync                           // sync (stub)
)

// parsedChannel is the structural result of parsing a client channel string,
// before any ref is resolved to a UUID.
type parsedChannel struct {
	kind      channelKind
	raw       string // original channel string, echoed back to the client
	repoRef   string // repo ref for repo-scoped channels
	entityRef string // node/thread/rule/job ref; empty for events and sync
}

// subscription is a resolved, active subscription stored on a connection.
type subscription struct {
	channel  string      // raw channel string; the map key and echoed to client
	kind     channelKind
	repoID   uuid.UUID   // resolved repo UUID for repo-scoped channels
	entityID uuid.UUID   // resolved node/thread UUID where applicable
	filter   *subFilter
}

// parseChannel parses a client channel string into its structural parts. It
// validates shape only; ref resolution happens later in resolveChannel.
func parseChannel(channel string) (parsedChannel, error) {
	if channel == "" {
		return parsedChannel{}, fmt.Errorf("empty channel")
	}
	if channel == "sync" {
		return parsedChannel{kind: channelSync, raw: channel}, nil
	}
	parts := strings.Split(channel, ".")
	switch parts[0] {
	case "jobs":
		if len(parts) != 2 || parts[1] == "" {
			return parsedChannel{}, fmt.Errorf("invalid job channel %q", channel)
		}
		return parsedChannel{kind: channelJob, raw: channel, entityRef: parts[1]}, nil
	case "repos":
		return parseRepoChannel(channel, parts)
	default:
		return parsedChannel{}, fmt.Errorf("unknown channel root %q", parts[0])
	}
}

// parseRepoChannel handles the repos.{repo_ref}.* family.
func parseRepoChannel(channel string, parts []string) (parsedChannel, error) {
	// parts[0] == "repos"; parts[1] == repo ref.
	if len(parts) < 3 || parts[1] == "" {
		return parsedChannel{}, fmt.Errorf("invalid repo channel %q", channel)
	}
	repoRef := parts[1]
	switch {
	case len(parts) == 3 && parts[2] == "events":
		return parsedChannel{kind: channelRepoEvents, raw: channel, repoRef: repoRef}, nil
	case len(parts) == 4 && parts[2] == "nodes" && parts[3] != "":
		return parsedChannel{kind: channelNode, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	case len(parts) == 4 && parts[2] == "threads" && parts[3] != "":
		return parsedChannel{kind: channelThread, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	case len(parts) == 5 && parts[2] == "rules" && parts[3] != "" && parts[4] == "actions":
		return parsedChannel{kind: channelRuleActions, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	default:
		return parsedChannel{}, fmt.Errorf("invalid repo channel %q", channel)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestParseChannel -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add realtime/subscription.go realtime/subscription_test.go
git commit -m "feat(realtime): channel string parsing"
```

---

## Task 4: Event matching and payload reference extraction

**Files:**
- Create: `realtime/match.go`
- Test: `realtime/match_test.go`

This implements channel scoping (repo + entity-ID match), `event_type` filtering, and best-effort `node_type` filtering. Per the design: `node_type` is evaluated only when the payload carries a full node (create events); events without a node (update/patch/delete) are NOT excluded by a `node_type` filter.

- [ ] **Step 1: Write the failing test**

Create `realtime/match_test.go`:
```go
package realtime

import (
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func nodeCreatedEvent(t *testing.T, repoID, nodeID uuid.UUID, nodeType string) *core.Event {
	t.Helper()
	ev, err := core.NewEvent(core.EventNodeCreated, repoID, core.EventNodeCreatePayload{
		Node: &core.Node{ID: nodeID, RepoID: repoID, Type: nodeType},
	})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	return ev
}

func nodeDeletedEvent(t *testing.T, repoID, nodeID uuid.UUID) *core.Event {
	t.Helper()
	ev, err := core.NewEvent(core.EventNodeDeleted, repoID, core.EventNodeDeletePayload{NodeID: nodeID})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	return ev
}

func TestMatchEvent_RepoChannel(t *testing.T) {
	repo := uuid.New()
	other := uuid.New()
	sub := &subscription{kind: channelRepoEvents, repoID: repo}

	if !sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("expected event in same repo to match")
	}
	if sub.matchEvent(nodeCreatedEvent(t, other, uuid.New(), "article")) {
		t.Error("expected event in other repo to not match")
	}
}

func TestMatchEvent_NodeChannel(t *testing.T) {
	repo := uuid.New()
	node := uuid.New()
	sub := &subscription{kind: channelNode, repoID: repo, entityID: node}

	if !sub.matchEvent(nodeCreatedEvent(t, repo, node, "article")) {
		t.Error("expected create of the subscribed node to match")
	}
	if !sub.matchEvent(nodeDeletedEvent(t, repo, node)) {
		t.Error("expected delete of the subscribed node to match")
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("expected create of a different node to not match")
	}
}

func TestMatchEvent_EventTypeFilter(t *testing.T) {
	repo := uuid.New()
	sub := &subscription{
		kind:   channelRepoEvents,
		repoID: repo,
		filter: &subFilter{EventType: []string{"NodeDeleted"}},
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("NodeCreated should be filtered out")
	}
	if !sub.matchEvent(nodeDeletedEvent(t, repo, uuid.New())) {
		t.Error("NodeDeleted should pass")
	}
}

func TestMatchEvent_NodeTypeFilterBestEffort(t *testing.T) {
	repo := uuid.New()
	sub := &subscription{
		kind:   channelRepoEvents,
		repoID: repo,
		filter: &subFilter{NodeType: []string{"article"}},
	}
	if !sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("article create should pass node_type filter")
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "note")) {
		t.Error("note create should be excluded by node_type filter")
	}
	// Delete carries no node body: filter cannot be evaluated, so it must NOT
	// exclude the event.
	if !sub.matchEvent(nodeDeletedEvent(t, repo, uuid.New())) {
		t.Error("delete should pass since node_type cannot be evaluated")
	}
}

func TestMatchEvent_StubChannelNeverMatches(t *testing.T) {
	repo := uuid.New()
	for _, k := range []channelKind{channelRuleActions, channelJob, channelSync} {
		sub := &subscription{kind: k, repoID: repo}
		if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
			t.Errorf("stub channel kind %v should never match", k)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestMatchEvent -v`
Expected: FAIL — `sub.matchEvent undefined`.

- [ ] **Step 3: Write the implementation**

Create `realtime/match.go`:
```go
package realtime

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// matchEvent reports whether the event should be delivered on this subscription.
// It applies channel scoping, then the optional event_type and node_type filters.
func (s *subscription) matchEvent(ev *core.Event) bool {
	if !s.matchChannel(ev) {
		return false
	}
	if !s.filter.matchEventType(ev) {
		return false
	}
	return s.filter.matchNodeType(ev)
}

// matchChannel reports whether the event falls within this subscription's
// channel scope. Stub channels (rule actions, jobs, sync) have no backing
// events yet and never match.
func (s *subscription) matchChannel(ev *core.Event) bool {
	switch s.kind {
	case channelRepoEvents:
		return ev.RepoID == s.repoID
	case channelNode:
		return ev.RepoID == s.repoID && eventReferencesNode(ev, s.entityID)
	case channelThread:
		return ev.RepoID == s.repoID && eventReferencesThread(ev, s.entityID)
	default:
		// TODO(phase-sync / phase-rules): wire channelRuleActions to
		// core.EventRuleTriggered, channelJob to a future JobUpdated event, and
		// channelSync to core.EventSyncCompleted / core.EventConflictDetected
		// once those events are published.
		return false
	}
}

// matchEventType applies the optional event_type filter. A nil or empty filter
// passes everything.
func (f *subFilter) matchEventType(ev *core.Event) bool {
	if f == nil || len(f.EventType) == 0 {
		return true
	}
	for _, t := range f.EventType {
		if core.EventType(t) == ev.Type {
			return true
		}
	}
	return false
}

// matchNodeType applies the optional node_type filter. It is best-effort: only
// create events carry a full node, so events without one (update/patch/delete)
// are not excluded by this filter.
func (f *subFilter) matchNodeType(ev *core.Event) bool {
	if f == nil || len(f.NodeType) == 0 {
		return true
	}
	node := nodeFromPayload(ev)
	if node == nil {
		return true
	}
	for _, t := range f.NodeType {
		if t == node.Type {
			return true
		}
	}
	return false
}

// nodeFromPayload extracts the embedded node from a NodeCreated payload. It
// returns nil for every other event type, since only create payloads carry a
// full node.
func nodeFromPayload(ev *core.Event) *core.Node {
	if ev.Type != core.EventNodeCreated {
		return nil
	}
	var p core.EventNodeCreatePayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return nil
	}
	return p.Node
}

// eventReferencesNode reports whether the event's payload references nodeID.
// It covers node-direct events and thread-node membership events.
func eventReferencesNode(ev *core.Event, nodeID uuid.UUID) bool {
	switch ev.Type {
	case core.EventNodeCreated:
		var p core.EventNodeCreatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.Node != nil && p.Node.ID == nodeID
	case core.EventNodeUpdated:
		var p core.EventNodeUpdatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	case core.EventNodePatched:
		var p core.EventNodePatchPayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	case core.EventNodeDeleted:
		var p core.EventNodeDeletePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	case core.EventThreadNodeAdded, core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	default:
		return false
	}
}

// eventReferencesThread reports whether the event's payload references threadID.
func eventReferencesThread(ev *core.Event, threadID uuid.UUID) bool {
	switch ev.Type {
	case core.EventThreadCreated:
		var p core.EventThreadCreatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.Thread != nil && p.Thread.ID == threadID
	case core.EventThreadUpdated:
		var p core.EventThreadUpdatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	case core.EventThreadDeleted:
		var p core.EventThreadDeletePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	case core.EventThreadNodeAdded, core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	default:
		return false
	}
}
```

> **Note for the implementer:** `core.EventThreadCreatePayload.Thread` is a `*core.Thread` and `core.EventNodeCreatePayload.Node` is a `*core.Node` (see `core/events.go`). The nil checks above guard against malformed payloads.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestMatchEvent -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add realtime/match.go realtime/match_test.go
git commit -m "feat(realtime): event matching and payload ref extraction"
```

---

## Task 5: Connection — send buffer and subscription set

**Files:**
- Create: `realtime/connection.go`
- Test: `realtime/connection_test.go`

This task builds the socket-agnostic core of a connection: a bounded outbound buffer with drop-oldest overflow handling, and a subscription set with the 10-subscription limit. The WebSocket read/write loops are added in Task 9.

- [ ] **Step 1: Write the failing test**

Create `realtime/connection_test.go`:
```go
package realtime

import (
	"testing"

	"github.com/google/uuid"
)

func TestConnection_AddSubscriptionLimit(t *testing.T) {
	c := newConnection("conn-1", "local")
	for i := 0; i < maxSubscriptions; i++ {
		ch := "repos.research.nodes.NODE-" + uuid.New().String()
		if err := c.addSubscription(&subscription{channel: ch}); err != nil {
			t.Fatalf("unexpected error adding subscription %d: %v", i, err)
		}
	}
	// The 11th distinct subscription must be rejected.
	err := c.addSubscription(&subscription{channel: "repos.research.events"})
	if err == nil {
		t.Fatal("expected subscription limit error, got nil")
	}
}

func TestConnection_AddSubscriptionReplaceSameChannel(t *testing.T) {
	c := newConnection("conn-1", "local")
	for i := 0; i < maxSubscriptions; i++ {
		ch := "repos.research.nodes.NODE-" + uuid.New().String()
		if err := c.addSubscription(&subscription{channel: ch}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	// Re-subscribing to an already-subscribed channel must not exceed the limit.
	existing := c.snapshotSubs()[0].channel
	if err := c.addSubscription(&subscription{channel: existing}); err != nil {
		t.Fatalf("re-subscribe to existing channel should succeed, got %v", err)
	}
}

func TestConnection_EnqueueOverflowDropsAndFlags(t *testing.T) {
	c := newConnection("conn-1", "local")
	// Fill the buffer beyond capacity.
	for i := 0; i < outboundBufferSize+10; i++ {
		c.enqueue([]byte("x"))
	}
	if len(c.out) != outboundBufferSize {
		t.Fatalf("expected buffer to be at capacity %d, got %d", outboundBufferSize, len(c.out))
	}
	if !c.takeDropped() {
		t.Fatal("expected dropped flag to be set after overflow")
	}
	// takeDropped clears the flag.
	if c.takeDropped() {
		t.Fatal("expected dropped flag to be cleared after takeDropped")
	}
}

func TestConnection_RemoveSubscription(t *testing.T) {
	c := newConnection("conn-1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events"})
	c.removeSubscription("repos.research.events")
	if len(c.snapshotSubs()) != 0 {
		t.Fatal("expected subscription to be removed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestConnection -v`
Expected: FAIL — `undefined: newConnection`.

- [ ] **Step 3: Write the implementation**

Create `realtime/connection.go`:
```go
package realtime

import (
	"sync"
	"sync/atomic"
)

// connection holds the server-side state for one WebSocket client: a bounded
// outbound buffer and the set of active subscriptions. The WebSocket read and
// write loops (see handlers.go) drive it; this type contains no socket logic so
// it can be tested in isolation.
type connection struct {
	id       string      // unique connection id, for logging
	identity string      // auth identity, for the per-identity connection cap
	out      chan []byte // buffered outbound frames

	mu   sync.Mutex
	subs map[string]*subscription // keyed by raw channel string

	dropped atomic.Bool // set when frames were dropped; cleared by takeDropped
}

// newConnection creates a connection with an empty subscription set and a
// buffer sized to outboundBufferSize.
func newConnection(id, identity string) *connection {
	return &connection{
		id:       id,
		identity: identity,
		out:      make(chan []byte, outboundBufferSize),
		subs:     make(map[string]*subscription),
	}
}

// addSubscription registers a subscription. Re-subscribing to an existing
// channel replaces it without counting against the limit. A new channel beyond
// maxSubscriptions is rejected.
func (c *connection) addSubscription(sub *subscription) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.subs[sub.channel]; !exists && len(c.subs) >= maxSubscriptions {
		return errSubscriptionLimit
	}
	c.subs[sub.channel] = sub
	return nil
}

// removeSubscription removes the subscription for the given channel, if present.
func (c *connection) removeSubscription(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, channel)
}

// snapshotSubs returns a copy of the current subscriptions so the hub can match
// without holding the connection lock during dispatch.
func (c *connection) snapshotSubs() []*subscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*subscription, 0, len(c.subs))
	for _, s := range c.subs {
		out = append(out, s)
	}
	return out
}

// enqueue queues a frame for delivery. When the buffer is full it drops the
// oldest queued frame to make room for the newest and records that a drop
// occurred, so a single messages_dropped notice can be sent.
func (c *connection) enqueue(frame []byte) {
	select {
	case c.out <- frame:
		return
	default:
	}
	// Buffer full: drop one old frame, then enqueue the new one.
	select {
	case <-c.out:
	default:
	}
	select {
	case c.out <- frame:
	default:
	}
	c.dropped.Store(true)
}

// takeDropped reports whether frames were dropped since the last call, clearing
// the flag.
func (c *connection) takeDropped() bool {
	return c.dropped.Swap(false)
}
```

Add the sentinel error to `realtime/subscription.go` (top-level, after the imports block):
```go
// errSubscriptionLimit is returned when a connection exceeds maxSubscriptions.
var errSubscriptionLimit = fmt.Errorf("subscription limit reached")
```

> The `fmt` import already exists in `subscription.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestConnection -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add realtime/connection.go realtime/connection_test.go realtime/subscription.go
git commit -m "feat(realtime): connection buffer and subscription set"
```

---

## Task 6: Hub — registry and dispatch

**Files:**
- Create: `realtime/hub.go`
- Test: `realtime/hub_test.go`

- [ ] **Step 1: Write the failing test**

Create `realtime/hub_test.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_DispatchDeliversToMatchingConnection(t *testing.T) {
	h := newHub(testLogger())
	repo := uuid.New()

	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: repo})
	if !h.add(c) {
		t.Fatal("expected add to succeed")
	}

	ev := nodeCreatedEvent(t, repo, uuid.New(), "article")
	h.dispatch(ev)

	select {
	case frame := <-c.out:
		var msg outboundEvent
		if err := json.Unmarshal(frame, &msg); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		if msg.Channel != "repos.research.events" {
			t.Errorf("channel: got %q", msg.Channel)
		}
		if msg.Event.Type != core.EventNodeCreated {
			t.Errorf("type: got %q", msg.Event.Type)
		}
	default:
		t.Fatal("expected a frame to be enqueued")
	}
}

func TestHub_DispatchSkipsNonMatching(t *testing.T) {
	h := newHub(testLogger())
	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: uuid.New()})
	h.add(c)

	h.dispatch(nodeCreatedEvent(t, uuid.New(), uuid.New(), "article")) // different repo
	if len(c.out) != 0 {
		t.Fatal("expected no frame for non-matching repo")
	}
}

func TestHub_PerIdentityConnectionCap(t *testing.T) {
	h := newHub(testLogger())
	for i := 0; i < maxConnsPerIdentity; i++ {
		if !h.add(newConnection("c", "local")) {
			t.Fatalf("add %d should succeed", i)
		}
	}
	if h.add(newConnection("over", "local")) {
		t.Fatal("expected the cap to reject the extra connection")
	}
}

func TestHub_RemoveFreesIdentitySlot(t *testing.T) {
	h := newHub(testLogger())
	conns := make([]*connection, maxConnsPerIdentity)
	for i := range conns {
		conns[i] = newConnection("c", "local")
		h.add(conns[i])
	}
	h.remove(conns[0])
	if !h.add(newConnection("new", "local")) {
		t.Fatal("expected a freed slot to allow a new connection")
	}
}

func TestHub_RunDispatchesIngestedEvents(t *testing.T) {
	h := newHub(testLogger())
	repo := uuid.New()
	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: repo})
	h.add(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.run(ctx)

	h.ingest(nodeCreatedEvent(t, repo, uuid.New(), "article"))

	select {
	case <-c.out:
	case <-time.After(time.Second):
		t.Fatal("expected ingested event to be dispatched")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestHub -v`
Expected: FAIL — `undefined: newHub`.

- [ ] **Step 3: Write the implementation**

Create `realtime/hub.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/upspeak/upspeak/core"
)

// hub is the in-process fan-out core. It holds the connection registry and
// dispatches each ingested event to every connection with a matching
// subscription. Ingestion is buffered so events can arrive (via the framework's
// repo.*.events.> subscription) before the dispatch loop starts.
type hub struct {
	logger   *slog.Logger
	ingestCh chan *core.Event

	mu         sync.RWMutex
	conns      map[*connection]struct{}
	byIdentity map[string]int
}

// newHub creates a hub with an empty registry and a buffered ingest channel.
func newHub(logger *slog.Logger) *hub {
	return &hub{
		logger:     logger,
		ingestCh:   make(chan *core.Event, ingestBufferSize),
		conns:      make(map[*connection]struct{}),
		byIdentity: make(map[string]int),
	}
}

// ingest hands an event to the dispatch loop. It never blocks: if the ingest
// backlog is full the event is dropped (logged), trading completeness for
// liveness on a real-time stream.
func (h *hub) ingest(ev *core.Event) {
	select {
	case h.ingestCh <- ev:
	default:
		h.logger.Warn("realtime: ingest buffer full, dropping event", "type", ev.Type)
	}
}

// add registers a connection, enforcing the per-identity cap. It returns false
// when the identity already holds maxConnsPerIdentity connections.
func (h *hub) add(c *connection) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byIdentity[c.identity] >= maxConnsPerIdentity {
		return false
	}
	h.conns[c] = struct{}{}
	h.byIdentity[c.identity]++
	return true
}

// remove unregisters a connection and frees its identity slot.
func (h *hub) remove(c *connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.conns[c]; !ok {
		return
	}
	delete(h.conns, c)
	h.byIdentity[c.identity]--
	if h.byIdentity[c.identity] <= 0 {
		delete(h.byIdentity, c.identity)
	}
}

// dispatch fans one event out to every matching subscription. A connection
// subscribed to several matching channels receives one frame per channel.
func (h *hub) dispatch(ev *core.Event) {
	h.mu.RLock()
	conns := make([]*connection, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		for _, sub := range c.snapshotSubs() {
			if !sub.matchEvent(ev) {
				continue
			}
			frame, err := buildOutbound(sub.channel, ev)
			if err != nil {
				h.logger.Warn("realtime: failed to encode event", "error", err)
				continue
			}
			c.enqueue(frame)
		}
	}
}

// run drains the ingest channel and dispatches each event until ctx is done.
func (h *hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-h.ingestCh:
			h.dispatch(ev)
		}
	}
}

// buildOutbound encodes the server-to-client envelope for an event on a channel.
func buildOutbound(channel string, ev *core.Event) ([]byte, error) {
	return json.Marshal(outboundEvent{
		Channel: channel,
		Event: outboundEventBody{
			ID:        ev.ID,
			Type:      ev.Type,
			Data:      ev.Payload,
			Timestamp: ev.Timestamp,
		},
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestHub -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add realtime/hub.go realtime/hub_test.go
git commit -m "feat(realtime): hub registry and event dispatch"
```

---

## Task 7: Channel resolution

**Files:**
- Create: `realtime/resolve.go`
- Test: `realtime/resolve_test.go`

Resolves a parsed channel's refs to UUIDs using the archive. Repo-scoped channels resolve `repoRef` via `ResolveRepoRef`; node/thread channels then resolve `entityRef` via `ResolveRef`. Stub channels are accepted without resolution (they never emit).

- [ ] **Step 1: Write the failing test**

Create `realtime/resolve_test.go`:
```go
package realtime

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// fakeResolver implements channelResolver for tests.
type fakeResolver struct {
	repoID    uuid.UUID
	repoSlug  string
	entityID  uuid.UUID
	entityTyp string
}

func (f fakeResolver) ResolveRepoRef(_ uuid.UUID, ref string) (*core.Repository, error) {
	if ref == f.repoSlug || ref == f.repoID.String() {
		return &core.Repository{ID: f.repoID}, nil
	}
	return nil, fmt.Errorf("repo not found: %s", ref)
}

func (f fakeResolver) ResolveRef(_ uuid.UUID, ref string) (uuid.UUID, string, error) {
	if ref == f.entityID.String() || ref == "NODE-1" || ref == "THREAD-1" {
		return f.entityID, f.entityTyp, nil
	}
	return uuid.Nil, "", fmt.Errorf("entity not found: %s", ref)
}

func TestResolveChannel_RepoEvents(t *testing.T) {
	r := fakeResolver{repoID: uuid.New(), repoSlug: "research"}
	sub, err := resolveChannel(r, "repos.research.events", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.kind != channelRepoEvents || sub.repoID != r.repoID {
		t.Fatalf("got kind=%v repoID=%v", sub.kind, sub.repoID)
	}
}

func TestResolveChannel_Node(t *testing.T) {
	r := fakeResolver{repoID: uuid.New(), repoSlug: "research", entityID: uuid.New(), entityTyp: "node"}
	sub, err := resolveChannel(r, "repos.research.nodes.NODE-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.kind != channelNode || sub.entityID != r.entityID {
		t.Fatalf("got kind=%v entityID=%v", sub.kind, sub.entityID)
	}
}

func TestResolveChannel_NodeKindMismatch(t *testing.T) {
	// entityRef resolves to a thread, but the channel claims a node.
	r := fakeResolver{repoID: uuid.New(), repoSlug: "research", entityID: uuid.New(), entityTyp: "thread"}
	_, err := resolveChannel(r, "repos.research.nodes.NODE-1", nil)
	if err == nil {
		t.Fatal("expected kind mismatch error")
	}
}

func TestResolveChannel_UnknownRepo(t *testing.T) {
	r := fakeResolver{repoID: uuid.New(), repoSlug: "research"}
	_, err := resolveChannel(r, "repos.ghost.events", nil)
	if err == nil {
		t.Fatal("expected unknown repo error")
	}
}

func TestResolveChannel_StubAcceptedWithoutResolution(t *testing.T) {
	r := fakeResolver{repoID: uuid.New(), repoSlug: "research"}
	for _, ch := range []string{"sync", "jobs.JOB-1"} {
		sub, err := resolveChannel(r, ch, nil)
		if err != nil {
			t.Fatalf("stub channel %q should resolve without error: %v", ch, err)
		}
		if sub.channel != ch {
			t.Fatalf("expected channel %q, got %q", ch, sub.channel)
		}
	}
}

func TestResolveChannel_InvalidSyntax(t *testing.T) {
	r := fakeResolver{}
	if _, err := resolveChannel(r, "not.a.channel", nil); err == nil {
		t.Fatal("expected parse error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestResolveChannel -v`
Expected: FAIL — `undefined: channelResolver` / `undefined: resolveChannel`.

- [ ] **Step 3: Write the implementation**

Create `realtime/resolve.go`:
```go
package realtime

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// defaultOwnerID is the personal-first single-user owner. It mirrors the same
// constant in the repo module and is replaced when real auth and multi-user
// support land.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// channelResolver is the subset of core.Archive needed to resolve channel refs
// to UUIDs. core.Archive satisfies it; tests supply a fake.
type channelResolver interface {
	ResolveRepoRef(ownerID uuid.UUID, ref string) (*core.Repository, error)
	ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error)
}

// resolveChannel parses a channel string and resolves its refs to UUIDs,
// producing an active subscription. Stub channels (rule actions, jobs, sync)
// are accepted after parsing without ref resolution, since they never emit yet.
func resolveChannel(r channelResolver, channel string, filter *subFilter) (*subscription, error) {
	pc, err := parseChannel(channel)
	if err != nil {
		return nil, err
	}

	sub := &subscription{channel: pc.raw, kind: pc.kind, filter: filter}

	switch pc.kind {
	case channelRepoEvents, channelNode, channelThread, channelRuleActions:
		repo, err := r.ResolveRepoRef(defaultOwnerID, pc.repoRef)
		if err != nil {
			return nil, fmt.Errorf("resolve repo %q: %w", pc.repoRef, err)
		}
		sub.repoID = repo.ID
	}

	switch pc.kind {
	case channelNode:
		if err := resolveEntity(r, sub, pc.entityRef, "node"); err != nil {
			return nil, err
		}
	case channelThread:
		if err := resolveEntity(r, sub, pc.entityRef, "thread"); err != nil {
			return nil, err
		}
	}

	return sub, nil
}

// resolveEntity resolves an entity ref within the subscription's repo and
// verifies the resolved entity type matches the channel kind.
func resolveEntity(r channelResolver, sub *subscription, ref, wantType string) error {
	id, entityType, err := r.ResolveRef(sub.repoID, ref)
	if err != nil {
		return fmt.Errorf("resolve entity %q: %w", ref, err)
	}
	if entityType != wantType {
		return fmt.Errorf("ref %q is a %s, not a %s", ref, entityType, wantType)
	}
	sub.entityID = id
	return nil
}
```

> **Implementer note:** Confirm the entity-type strings returned by `ResolveRef`. Run `grep -rn 'EntityType\|"node"\|"thread"' archive/*.go core/identity.go` and align the `wantType` literals (`"node"`, `"thread"`) with what the archive returns. Adjust the two literals if the archive uses different casing or values.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestResolveChannel -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add realtime/resolve.go realtime/resolve_test.go
git commit -m "feat(realtime): channel ref resolution"
```

---

## Task 8: Module skeleton and event ingestion

**Files:**
- Create: `realtime/realtime.go`
- Test: `realtime/realtime_test.go`

Ties the pieces together as an `app.Module`. `HTTPHandlers` returns nil for now (the `/ws` route is added in Task 9). `MsgHandlers` registers the `repo.*.events.>` ingest handler.

- [ ] **Step 1: Write the failing test**

Create `realtime/realtime_test.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestModule_IngestEventDispatchesToConnection(t *testing.T) {
	m := &Module{}
	if err := m.Init(nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	repo := uuid.New()
	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: repo})
	m.hub.add(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	// Find the ingest handler the framework would register.
	handlers := m.MsgHandlers()
	if len(handlers) != 1 || handlers[0].Subject != "repo.*.events.>" {
		t.Fatalf("expected one handler on repo.*.events.>, got %+v", handlers)
	}

	ev := nodeCreatedEvent(t, repo, uuid.New(), "article")
	data, _ := json.Marshal(ev)
	handlers[0].Handler("repo."+repo.String()+".events.NodeCreated", data)

	select {
	case <-c.out:
	case <-time.After(time.Second):
		t.Fatal("expected ingested event to reach the connection")
	}
}

func TestModule_Name(t *testing.T) {
	m := &Module{}
	if m.Name() != "realtime" {
		t.Fatalf("got name %q", m.Name())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestModule -v`
Expected: FAIL — `undefined: Module`.

- [ ] **Step 3: Write the implementation**

Create `realtime/realtime.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Module implements app.Module for real-time event streaming over WebSocket.
// It ingests every repository's domain events through a single core-NATS
// subscription and fans them out to subscribed clients via the hub.
type Module struct {
	archive core.Archive
	auth    Authenticator
	hub     *hub
	logger  *slog.Logger
}

// Name returns the module name.
func (m *Module) Name() string { return "realtime" }

// Init initialises the module, creating the hub and default authenticator. The
// hub is created here (not in Run) so that ingestEvent is safe to call as soon
// as the framework registers the MsgHandler, which happens during InitModules.
func (m *Module) Init(_ map[string]any) error {
	m.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	if m.auth == nil {
		m.auth = allowAllAuthenticator{}
	}
	m.hub = newHub(m.logger)
	m.logger.Info("Initialised realtime module")
	return nil
}

// SetArchive injects the archive, used to resolve channel refs to UUIDs.
func (m *Module) SetArchive(archive core.Archive) { m.archive = archive }

// SetAuthenticator overrides the default allow-all authenticator.
func (m *Module) SetAuthenticator(a Authenticator) { m.auth = a }

// HTTPHandlers returns the module's HTTP routes. The /ws route is added in the
// next task; returning nil here keeps the module compiling and registrable.
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return nil
}

// MsgHandlers subscribes to every repository's domain events for fan-out. The
// subject uses a NATS wildcard; the module never imports nats-io.
func (m *Module) MsgHandlers() []app.MsgHandler {
	return []app.MsgHandler{
		{Subject: "repo.*.events.>", Handler: m.ingestEvent},
	}
}

// ingestEvent unmarshals a domain event and hands it to the hub. It is safe to
// call before Run starts and before the archive is wired: the hub buffers
// events and no client can be connected before HTTP starts.
func (m *Module) ingestEvent(_ string, data []byte) {
	var ev core.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		m.logger.Warn("realtime: dropping malformed event", "error", err)
		return
	}
	m.hub.ingest(&ev)
}

// Run drives the hub's dispatch loop until ctx is cancelled. Start it with a
// goroutine after module wiring (see main.go).
func (m *Module) Run(ctx context.Context) { m.hub.run(ctx) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestModule -v`
Expected: PASS.

- [ ] **Step 5: Run the whole package and commit**

Run: `go test ./realtime/ -v`
Expected: PASS (all tests so far).

```bash
git add realtime/realtime.go realtime/realtime_test.go
git commit -m "feat(realtime): module skeleton and event ingestion"
```

---

## Task 9: WebSocket upgrade handler and connection loops

**Files:**
- Create: `realtime/handlers.go`
- Modify: `realtime/realtime.go` (HTTPHandlers returns the /ws route)
- Test: `realtime/handlers_test.go`

This task adds the real socket: the upgrade handler (auth seam → `Accept`), a read loop that handles subscribe/unsubscribe, and a write loop with ping keepalive and `messages_dropped` notices.

- [ ] **Step 1: Write the failing integration test and test seam**

Create `realtime/export_test.go` (the test-only resolver and helpers):
```go
package realtime

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// staticResolver is a test channelResolver mapping one slug to one repo UUID.
// It does not resolve entity refs, so tests using it subscribe only to
// repo-events channels.
type staticResolver struct {
	repoID   uuid.UUID
	repoSlug string
}

func (s staticResolver) ResolveRepoRef(_ uuid.UUID, ref string) (*core.Repository, error) {
	if ref == s.repoSlug || ref == s.repoID.String() {
		return &core.Repository{ID: s.repoID}, nil
	}
	return nil, errors.New("repo not found")
}

func (s staticResolver) ResolveRef(_ uuid.UUID, _ string) (uuid.UUID, string, error) {
	return uuid.Nil, "", errors.New("entity resolution not supported in static resolver")
}

// setResolver injects a channel resolver for tests, bypassing the archive.
func (m *Module) setResolver(r channelResolver) { m.testResolver = r }

// testingT is the minimal subset of *testing.T used by waitForSubscription.
type testingT interface{ Fatalf(format string, args ...any) }

// waitForSubscription spins until at least one connection holds a subscription
// or the deadline passes.
func waitForSubscription(t testingT, m *Module) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.hub.mu.RLock()
		var total int
		for c := range m.hub.conns {
			total += len(c.snapshotSubs())
		}
		m.hub.mu.RUnlock()
		if total > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no subscription registered before deadline")
}
```

Create `realtime/handlers_test.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func TestWS_SubscribeAndReceiveEvent(t *testing.T) {
	repo := uuid.New()
	m := &Module{}
	if err := m.Init(nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	m.setResolver(staticResolver{repoID: repo, repoSlug: "research"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	mux := http.NewServeMux()
	for _, h := range m.HTTPHandlers() {
		mux.HandleFunc(h.Method+" /api/v1"+h.Path, h.Handler)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := dialWS(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	wctx, wcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer wcancel()
	if err := wsjson.Write(wctx, c, clientMessage{Action: "subscribe", Channel: "repos.research.events"}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	// Give the read loop time to register the subscription, then publish.
	waitForSubscription(t, m)
	ev := nodeCreatedEvent(t, repo, uuid.New(), "article")
	data, _ := json.Marshal(ev)
	m.ingestEvent("", data)

	var msg outboundEvent
	rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rcancel()
	if err := wsjson.Read(rctx, c, &msg); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if msg.Channel != "repos.research.events" {
		t.Fatalf("channel: got %q", msg.Channel)
	}
}

func TestWS_InvalidChannelReturnsError(t *testing.T) {
	m := &Module{}
	_ = m.Init(nil)
	m.setResolver(staticResolver{repoID: uuid.New(), repoSlug: "research"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	mux := http.NewServeMux()
	for _, h := range m.HTTPHandlers() {
		mux.HandleFunc(h.Method+" /api/v1"+h.Path, h.Handler)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := dialWS(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	wctx, wcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer wcancel()
	_ = wsjson.Write(wctx, c, clientMessage{Action: "subscribe", Channel: "bogus"})

	var em errorMessage
	rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rcancel()
	if err := wsjson.Read(rctx, c, &em); err != nil {
		t.Fatalf("read error msg: %v", err)
	}
	if em.Type != "error" || em.Code != codeInvalidChannel {
		t.Fatalf("got %+v", em)
	}
}
```

> **Test seam:** `staticResolver`, `setResolver`, and `waitForSubscription` live in `realtime/export_test.go` (above) and are compiled only under test. The production struct gains one `testResolver` field (Step 3); `resolver()` prefers it when set, else uses the archive.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./realtime/ -run TestWS -v`
Expected: FAIL — `m.HTTPHandlers` returns nil so no `/ws` route; `m.resolver` / `m.wsHandler` undefined.

- [ ] **Step 3: Write the implementation**

Create `realtime/handlers.go`:
```go
package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// resolver returns the channel resolver: a test override if set, else the
// archive.
func (m *Module) resolver() channelResolver {
	if m.testResolver != nil {
		return m.testResolver
	}
	return m.archive
}

// wsHandler upgrades the connection, authenticates via the seam, registers it
// with the hub, and runs the read and write loops.
func (m *Module) wsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, err := m.auth.Authenticate(r)
		if err != nil {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			m.logger.Warn("realtime: accept failed", "error", err)
			return
		}

		c := newConnection(uuid.NewString(), identity)
		if !m.hub.add(c) {
			_ = conn.Close(websocket.StatusPolicyViolation, "connection limit reached")
			return
		}
		defer m.hub.remove(c)
		defer conn.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		go m.writeLoop(ctx, cancel, conn, c)
		m.readLoop(ctx, conn, c)
	}
}

// readLoop processes inbound control messages until the client disconnects or
// ctx is cancelled.
func (m *Module) readLoop(ctx context.Context, conn *websocket.Conn, c *connection) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // normal close or error: end the loop, deferred cleanup runs
		}
		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.enqueue(mustJSON(errorMessage{Type: "error", Code: codeInvalidChannel, Message: "malformed message"}))
			continue
		}
		m.handleControl(c, msg)
	}
}

// handleControl applies a subscribe or unsubscribe message.
func (m *Module) handleControl(c *connection, msg clientMessage) {
	switch msg.Action {
	case "subscribe":
		sub, err := resolveChannel(m.resolver(), msg.Channel, msg.Filter)
		if err != nil {
			c.enqueue(mustJSON(errorMessage{Type: "error", Code: codeInvalidChannel, Message: err.Error()}))
			return
		}
		if err := c.addSubscription(sub); err != nil {
			c.enqueue(mustJSON(errorMessage{Type: "error", Code: codeSubscriptionLimit, Message: err.Error()}))
		}
	case "unsubscribe":
		c.removeSubscription(msg.Channel)
	default:
		c.enqueue(mustJSON(errorMessage{Type: "error", Code: codeInvalidChannel, Message: "unknown action"}))
	}
}

// writeLoop drains queued frames to the socket and sends ping keepalives.
// It cancels the shared context (ending the read loop) when the socket fails.
func (m *Module) writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *connection) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	pingFailures := 0

	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-c.out:
			if c.takeDropped() {
				_ = writeFrame(ctx, conn, mustJSON(noticeMessage{Type: "messages_dropped", Message: "slow consumer: oldest messages dropped"}))
			}
			if err := writeFrame(ctx, conn, frame); err != nil {
				cancel()
				return
			}
		case <-ticker.C:
			pctx, pcancel := context.WithTimeout(ctx, pingInterval)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				pingFailures++
				if pingFailures >= maxPingFailures {
					cancel()
					return
				}
				continue
			}
			pingFailures = 0
		}
	}
}

// writeFrame writes a single text frame with a bounded deadline.
func writeFrame(ctx context.Context, conn *websocket.Conn, frame []byte) error {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, frame)
}

// mustJSON marshals a control message that is known to be encodable.
func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
```

Now wire the dependencies into the module. Modify `realtime/realtime.go`.

Add one field to the `Module` struct (the test-only resolver override):
```go
type Module struct {
	archive      core.Archive
	auth         Authenticator
	hub          *hub
	logger       *slog.Logger
	testResolver channelResolver // test-only override for resolver(); nil in production
}
```

Replace the `HTTPHandlers` method body to register the `/ws` route:
```go
// HTTPHandlers returns the module's HTTP routes.
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: "GET", Path: "/ws", Handler: m.wsHandler()},
	}
}
```

No other change to `realtime.go` is needed: `resolver()`, `wsHandler`, and the loops live in `handlers.go`; the test seam (`setResolver`, `waitForSubscription`, `staticResolver`) lives in `export_test.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./realtime/ -run TestWS -v`
Expected: PASS (both `TestWS_SubscribeAndReceiveEvent` and `TestWS_InvalidChannelReturnsError`).

- [ ] **Step 5: Run the whole package and vet**

Run: `go test ./realtime/ && go vet ./realtime/`
Expected: PASS, no vet complaints.

- [ ] **Step 6: Commit**

```bash
git add realtime/handlers.go realtime/realtime.go realtime/handlers_test.go realtime/export_test.go
git commit -m "feat(realtime): WebSocket upgrade handler with read/write loops"
```

---

## Task 10: Wire the module into main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add the module construction and registration**

In `main.go`, alongside the other module constructions (near the `searchModule`/`rulesModule` block around lines 70-120), add the import and module. Add to the import block:
```go
	"github.com/upspeak/upspeak/realtime"
```

After the other `&xxx.Module{}` constructions, add:
```go
	// Initialise realtime (WebSocket) module.
	realtimeModule := &realtime.Module{}
```

After the other `up.AddModuleOnPath(..., "/api/v1")` calls, add:
```go
	if err := up.AddModuleOnPath(realtimeModule, "/api/v1"); err != nil {
		slog.Error("Error adding realtime module", "error", err)
		os.Exit(1)
	}
```

- [ ] **Step 2: Wire archive and start the hub**

After the other `SetArchive` calls (where `a := archiveModule.GetArchive()`), add:
```go
	realtimeModule.SetArchive(a)
```

After the rules engine goroutine (`go rulesEngine.Run(runnerCtx)`), add:
```go
	// Start the realtime hub dispatch loop. Its MsgHandler (repo.*.events.>) was
	// registered during InitModules; the hub buffers events until this loop runs.
	go realtimeModule.Run(runnerCtx)
```

- [ ] **Step 3: Build and verify**

Run: `go build ./... && go vet ./...`
Expected: builds cleanly.

- [ ] **Step 4: Smoke test the endpoint manually (optional but recommended)**

Run the server (requires `upspeak.yaml`):
```bash
./build.sh dev
```
In another terminal, confirm the route is registered by checking the startup logs for `Registering HTTP handler path=/api/v1/ws method=GET`. If you have `websocat` installed:
```bash
websocat ws://localhost:8080/api/v1/ws
# then paste: {"action":"subscribe","channel":"repos.<your-repo-slug>.events"}
```
Create a node via the REST API in a third terminal and confirm an event frame arrives on the socket. Stop the server with Ctrl-C.

- [ ] **Step 5: Run the full test suite**

Run: `go test -tags sqlite_fts5 ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat(realtime): wire realtime module into main"
```

---

## Task 11: Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.claude/skills/upspeak-dev/SKILL.md`

- [ ] **Step 1: Update CLAUDE.md**

In the "Key packages" list, add after the `search/` entry:
```markdown
- `realtime/`: WebSocket event streaming. Single endpoint `GET /api/v1/ws`. Subscribes to `repo.*.events.>` via one `app.MsgHandler` (core-NATS fan-out) and dispatches matching events to connected clients with server-side filtering. Mounted at `/api/v1`
```

In the "Implementation Plan" section, update the status line:
```markdown
**Completed:** Phase 1 (foundation), Phase 2 (knowledge graph), Correction Pass, NATS hardening pass, Phase 3 (filters + jobs), Phase 4 (connectors + schedules), Phase 5 (rules + search), Phase 6a (realtime WebSocket)
**Next:** Phase 6b (multi-device sync)
```

In the NATS "Consumers" note, no change is needed (realtime uses core-NATS fan-out, not a durable consumer) — but add this sentence to the "NATS Communication" section after the consumers paragraph:
```markdown
The realtime module consumes events via core-NATS fan-out (an `app.MsgHandler` on `repo.*.events.>`), not a durable consumer — a live socket needs the current tail, not durable replay.
```

- [ ] **Step 2: Update the upspeak-dev skill status table**

In `.claude/skills/upspeak-dev/SKILL.md`, change the Phase 6 row of the Implementation Status table to two rows:
```markdown
| 6a. Real-time | Done | WebSocket fan-out of repo events, server-side filtering, auth seam |
| 6b. Sync | Planned | Multi-device sync, conflict resolution, peers |
```

- [ ] **Step 3: Verify docs build/read correctly**

Run: `git diff --stat`
Expected: shows `CLAUDE.md` and the skill file modified.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md .claude/skills/upspeak-dev/SKILL.md
git commit -m "docs: record realtime WebSocket module (Phase 6a)"
```

---

## Self-Review Notes (for the implementer)

**Spec coverage check** — every requirement in `14-api-realtime.md` maps to a task:
- Subscribe/unsubscribe messages → Task 9 (`handleControl`)
- Server→client event envelope → Task 6 (`buildOutbound`) + Task 2 (`outboundEvent`)
- Six channel patterns → Task 3 (parse) + Task 7 (resolve); three are stubs (never match, Task 4)
- Server-side filtering (event_type, node_type) → Task 4
- Auth during upgrade → Task 9 (`wsHandler`) + Task 2 (`Authenticator` seam)
- Ping every 30s, 3-miss termination → Task 9 (`writeLoop`)
- 10 subscriptions/connection → Task 5 (`addSubscription`)
- 5 connections/user → Task 6 (`add`); collapses to global cap of 5 under allow-all auth (documented)
- 1000-message buffer, drop-oldest, `messages_dropped` notice → Task 5 (`enqueue`) + Task 9 (`writeLoop`)
- Error messages `{type,code,message}` → Task 2 (`errorMessage`) + Task 9

**Deferred (not in this plan, by design):** sync module, peers, conflict resolution; real auth; backing events for the three stub channels.

**Known follow-ups to verify during implementation:**
1. Confirm `ResolveRef` entity-type strings (Task 7 implementer note).
2. Confirm `coder/websocket` version `v1.8.13` is current; if `go get` resolves a newer tag, use it.
3. The `testResolver` field on `Module` is the only test-only hook in production code; the `staticResolver` fake and helpers are compiled under test via `export_test.go`. If you would rather avoid the production field entirely, supply a full `core.Archive` fake instead — but that means stubbing every Archive method, which is why the one-field seam is preferred.
