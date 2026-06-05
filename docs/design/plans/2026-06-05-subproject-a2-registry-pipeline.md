# Connector Adapterisation — Sub-project A2 (Registry + Ingest Pipeline) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the adapter **registry** and the **ingest pipeline** — the machinery that turns adapter-emitted `IngestBatch`es into persisted graph entities by reusing the canonical write+event path — and migrate the `webhook` connector onto a real `core.Adapter`, breaking the `connector ↔ jobs` cycle via dependency injection.

**Architecture:** A neutral `app.AdapterRegistry` lookup interface (beside `app.Publisher`/`app.Consumer`) lets the job runner dispatch to adapters without importing the concrete integrations. A new `ingest/` package holds the concrete registry and the pipeline (`map → filter → persist → dedup → events`), reusing `core.Archive` + `app.Publisher.PublishEvent` exactly as `repo/` does. A new `integrations/webhook/` package implements `core.Adapter` + `core.Collector`. `main.go` builds the registry from the integrations and injects it into the job runner — so `jobs` depends only on the `app` interface, never on `connector` or `integrations`.

**Tech Stack:** Go, stdlib `net/http` + `net/http/httptest`, `google/uuid`, the A1 `core` domain model (`Adapter`/`Collector`/`IngestBatch`/`IngestContext`), the existing `filter` engine, `httptest`-backed adapter tests, and a real `archive.NewLocalArchive` over a temp dir.

---

## Plan family (context)

This is **A2 of the ingestion track** (`docs/design/2026-06-05-connector-adapterisation.md`, ADR-0013/0014/0015). A1 (foundation) shipped the `core` model + secrets cipher + archive stores. The track:

- **A — framework** (this spine):
  - **A1 (shipped): foundation** — core model + secure storage + archive.
  - **A2 (this plan): adapter registry + ingest pipeline** — registry, Item-path pipeline reusing the write/event path, jobs DI (cycle break), **migrate `webhook` onto an adapter**.
  - **A2b (new, deferred): repo→repo adapter** — net-new internal data movement with archive-read access + cycle-detection interplay (split out of A2 — see "Scope decisions").
  - **A3: connection module** — `/api/v1/connections` CRUD + `TestConnection` + OAuth flow; **wire the cipher at runtime**.
  - **A4: OAuth flow + Mastodon adapter.**
- **B — pull execution + RSS & Discourse** — wires `executeCollect`/`executePublish` (cursors, filter-on-normalised, dedup) through the registry+pipeline this plan builds.
- **C — streaming + Matrix.**

Each slice ends with a green build and passing tests.

---

## Scope decisions (resolved before planning)

- **Webhook-only adapter migration.** The design (§8) lists `webhook` *and* `repo`. We split `repo` into a separate **A2b** slice: it is net-new repo→repo data movement (today's `executeCollect`/`executePublish` are pure stubs — no repo data has ever moved), it is **not** in the validation set (RSS/Discourse/Matrix/Mastodon), and a repo→repo collect inherently **reads the archive**, which contradicts the "adapters never touch the archive" boundary that `webhook` respects. `webhook` alone exercises the entire A2 spine (registry, DI/cycle-break, pipeline, write-path reuse, and rules/search/realtime lighting up for free) cleanly.
- **`executeCollect`/`executePublish` stay stubs in A2.** Per design §8, wiring them (cursors, filter-on-normalised, dedup) is **Sub-project B**. A2 injects the registry + pipeline into the runner and proves them through `executeWebhook`; B reuses the same machinery for source/sink collection.
- **No connector-module change.** The `connector ↔ jobs` cycle is *prevented*, not pre-existing: today `connector` imports `jobs` (one-directional, fine). The cycle would form only if `jobs` imported `connector`/`integrations` to reach adapters. Injecting `app.AdapterRegistry` keeps `jobs` clean, so `connector` needs no edit in A2. (Registry-driven connector validation + connector↔connection match enforcement land in A3, when connections become creatable over HTTP.)

**Diagram check (`assets/high-level-concepts-0.1.png`):** the design still holds. The **Import** box ("integrate with other services and transform data to Nodes and Edges") is exactly the adapter + pipeline; the **filter** diamond between Input/Import and Repository confirms filters run on the *normalised* node (map → filter → persist); **populate → Repository → store → Archive** is the reused `SaveBatchNodes` + `PublishEvent` write path. No concept is contradicted.

---

## File map for A2

| File | Responsibility | Action |
|---|---|---|
| `app/app.go` | `AdapterRegistry` lookup interface | Modify |
| `ingest/registry.go` | concrete registry (`Register` / `AdapterFor`) | Create |
| `ingest/registry_test.go` | registry tests | Create |
| `ingest/pipeline.go` | `IngestContext`, `IngestResult`, `Pipeline`, `Ingest` (Item path) + helpers | Create |
| `ingest/pipeline_test.go` | pipeline tests | Create |
| `integrations/webhook/webhook.go` | webhook `core.Adapter` + `core.Collector` | Create |
| `integrations/webhook/webhook_test.go` | webhook adapter tests | Create |
| `jobs/runner.go` | runner holds `pub`/`registry`/`pipeline`; rewrite `executeWebhook` | Modify |
| `jobs/runner_test.go` | webhook end-to-end runner test | Create |
| `main.go` | build registry, register webhook, inject into `jobs.NewRunner` | Modify |
| `docs/next-steps.md` | mark A2 shipped; carry-forward watch-outs | Modify |

**Dependency direction (no cycle):** `integrations/webhook → core`; `ingest → app, core, filter`; `jobs → app, core, ingest`; `main → everything`. `app → core` only (already true). `ingest` is imported by `jobs` but imports neither `jobs` nor `connector` nor `integrations`.

---

## Task 1: The `AdapterRegistry` interface

A neutral lookup interface in `app/`, beside `Publisher`/`Consumer`, so `jobs` dispatches to adapters through an interface and never imports the concrete adapters.

**Files:**
- Modify: `app/app.go`

- [ ] **Step 1: Add the interface**

In `app/app.go`, immediately after the `Consumer` interface block (after the `Fetch` method's closing `}`), add:

```go
// AdapterRegistry resolves a connector type to its registered core.Adapter. It
// lets the job runner (and, later, the connector module) dispatch to adapters
// without importing the concrete integrations — breaking the connector↔jobs
// import cycle. The concrete registry is built in main.go from the registered
// integrations and injected via setter.
type AdapterRegistry interface {
	// AdapterFor returns the adapter registered for the connector type, and
	// false when none is registered.
	AdapterFor(connector core.ConnectorType) (core.Adapter, bool)
}
```

- [ ] **Step 2: Build and vet**

Run: `go build ./app/... && go vet ./app/...`
Expected: builds cleanly (the file already imports `core`).

- [ ] **Step 3: Commit**

```bash
git add app/app.go
git commit -m "feat(app): add AdapterRegistry lookup interface

Neutral seam (beside Publisher/Consumer) so the job runner dispatches to
adapters through an interface and never imports the concrete integrations,
preventing the connector<->jobs cycle (ADR-0013).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: The concrete registry (`ingest` package)

**Files:**
- Create: `ingest/registry.go`
- Test: `ingest/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `ingest/registry_test.go`:

```go
package ingest

import (
	"context"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// stubAdapter is a minimal core.Adapter for registry tests.
type stubAdapter struct{ t core.ConnectorType }

func (s stubAdapter) Type() core.ConnectorType                       { return s.t }
func (s stubAdapter) Capabilities() core.AdapterCapabilities         { return core.AdapterCapabilities{} }
func (s stubAdapter) ConnectionSchema() core.ConnectionSchema        { return core.ConnectionSchema{} }
func (s stubAdapter) ValidateConnectionConfig(map[string]any) error  { return nil }
func (s stubAdapter) ValidateSourceConfig(map[string]any) error      { return nil }
func (s stubAdapter) ValidateSinkConfig(map[string]any) error        { return nil }
func (s stubAdapter) TestConnection(context.Context, *core.Connection) error { return nil }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAdapter{t: core.ConnectorWebhook})

	got, ok := r.AdapterFor(core.ConnectorWebhook)
	if !ok {
		t.Fatal("expected webhook adapter to be registered")
	}
	if got.Type() != core.ConnectorWebhook {
		t.Fatalf("got adapter type %q, want webhook", got.Type())
	}

	if _, ok := r.AdapterFor(core.ConnectorRSS); ok {
		t.Fatal("expected no adapter for rss")
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAdapter{t: core.ConnectorWebhook})
	r.Register(stubAdapter{t: core.ConnectorWebhook}) // last write wins, no panic
	if _, ok := r.AdapterFor(core.ConnectorWebhook); !ok {
		t.Fatal("expected webhook adapter after re-register")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./ingest/ -run TestRegistry`
Expected: FAIL — `ingest` package / `NewRegistry` undefined.

- [ ] **Step 3: Implement the registry**

Create `ingest/registry.go`:

```go
// Package ingest holds the adapter registry and the ingest pipeline. The
// pipeline turns adapter-emitted IngestBatches into persisted graph entities by
// reusing the canonical write+event path (Archive + Publisher), so ingested
// data flows through rules, realtime, and search with no extra wiring.
package ingest

import (
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// Registry is the concrete app.AdapterRegistry. It is populated once at startup
// (main.go) and read concurrently thereafter; registration is not expected
// after the server starts, so it carries no lock.
type Registry struct {
	adapters map[core.ConnectorType]core.Adapter
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[core.ConnectorType]core.Adapter)}
}

// Register adds (or replaces) the adapter for its connector type.
func (r *Registry) Register(a core.Adapter) {
	r.adapters[a.Type()] = a
}

// AdapterFor returns the adapter for a connector type, and false when absent.
func (r *Registry) AdapterFor(connector core.ConnectorType) (core.Adapter, bool) {
	a, ok := r.adapters[connector]
	return a, ok
}

// Ensure Registry satisfies the app port.
var _ app.AdapterRegistry = (*Registry)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./ingest/ -run TestRegistry`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add ingest/registry.go ingest/registry_test.go
git commit -m "feat(ingest): adapter registry implementing app.AdapterRegistry

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: The ingest pipeline (Item path)

The pipeline persists an `IngestBatch`'s `Items` into a repo, reusing `SaveBatchNodes`/`SaveNode` + `PublishEvent`. Source-based ingestion dedups by `(Source.ID, ExternalID)`, applies the source filter chain, and stamps provenance; ad-hoc ingestion (one-shot webhook) does none of these. Threads, Annotations, Tombstones, reply Edges, and author→User resolution are **deferred to Sub-project B** (they need entities/infrastructure A2 does not build — e.g. thread provenance, a user store).

**Files:**
- Create: `ingest/pipeline.go`
- Test: `ingest/pipeline_test.go`

- [ ] **Step 1: Write the failing tests**

Create `ingest/pipeline_test.go`:

```go
package ingest

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/core"
)

// testOwnerID is a fixed owner for test repos (mirrors the archive test owner).
var testOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// newTestArchive builds a real LocalArchive in a temp dir via the public
// constructor — the archive package's own setupTestArchive is test-only and not
// importable, so sibling-package tests construct the archive directly.
func newTestArchive(t *testing.T) *archive.LocalArchive {
	t.Helper()
	a, err := archive.NewLocalArchive(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalArchive: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// newTestRepo persists a repo for use as an ingest destination.
func newTestRepo(t *testing.T, a *archive.LocalArchive) *core.Repository {
	t.Helper()
	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "test-repo",
		Name:    "Test Repository",
		OwnerID: testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("SaveRepository: %v", err)
	}
	return repo
}

// setupPipeline builds a pipeline over a temp archive and returns both. The
// concrete *archive.LocalArchive satisfies core.Archive for NewPipeline.
func setupPipeline(t *testing.T) (*Pipeline, *archive.LocalArchive) {
	t.Helper()
	a := newTestArchive(t)
	return NewPipeline(a, nil), a // nil publisher: pipeline tolerates it
}

func textBody(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func TestPipeline_AdHoc_CreatesNodeNoProvenance(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)

	batch := &core.IngestBatch{Items: []core.IngestItem{{
		ExternalID: "https://example.com/x",
		Node: &core.Node{
			Type: "webpage", Subject: "x", ContentType: "text/plain", Body: textBody("hello"),
		},
	}}}

	res, err := p.Ingest(IngestContext{RepoID: repo.ID, Source: nil, CreatedBy: repo.OwnerID}, batch)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Created != 1 || res.Updated != 0 || res.Skipped != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	nodes, _, err := a.ListNodes(repo.ID, core.NodeListOptions{})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].SourceID != nil || nodes[0].ExternalID != nil {
		t.Fatal("ad-hoc node must carry no provenance")
	}
}

func TestPipeline_SourceBased_DedupsOnReingest(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := &core.Source{
		ID: core.NewID(), RepoID: repo.ID, Name: "s",
		Connector: core.ConnectorWebhook, CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	mk := func(body string) *core.IngestBatch {
		return &core.IngestBatch{Items: []core.IngestItem{{
			ExternalID: "ext-1",
			Node:       &core.Node{Type: "post", Subject: "s", ContentType: "text/plain", Body: textBody(body)},
		}}}
	}
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	res1, err := p.Ingest(ctx, mk("v1"))
	if err != nil || res1.Created != 1 {
		t.Fatalf("first ingest: res=%+v err=%v", res1, err)
	}
	res2, err := p.Ingest(ctx, mk("v2"))
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if res2.Created != 0 || res2.Updated != 1 {
		t.Fatalf("re-ingest should update, got %+v", res2)
	}

	got, err := a.GetNodeBySourceExternalID(src.ID, "ext-1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if string(got.Body) != string(textBody("v2")) {
		t.Fatalf("body not updated: %s", got.Body)
	}
	nodes, _, _ := a.ListNodes(repo.ID, core.NodeListOptions{})
	if len(nodes) != 1 {
		t.Fatalf("dedup failed: %d nodes", len(nodes))
	}
}

func TestPipeline_SourceFilter_SkipsNonMatching(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)

	// Filter: type == "keep".
	cond := core.Condition{Field: "type", Op: core.OpEq, Value: json.RawMessage(strconv.Quote("keep"))}
	f := &core.Filter{ID: core.NewID(), RepoID: repo.ID, Name: "keep-only",
		Mode: core.FilterModeAll, Conditions: []core.Condition{cond}, CreatedBy: repo.OwnerID}
	if err := a.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}
	src := &core.Source{ID: core.NewID(), RepoID: repo.ID, Name: "s",
		Connector: core.ConnectorWebhook, FilterIDs: []uuid.UUID{f.ID},
		FilterChainMode: core.FilterModeAll, CreatedBy: repo.OwnerID}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	batch := &core.IngestBatch{Items: []core.IngestItem{
		{ExternalID: "a", Node: &core.Node{Type: "keep", Subject: "a", ContentType: "text/plain"}},
		{ExternalID: "b", Node: &core.Node{Type: "drop", Subject: "b", ContentType: "text/plain"}},
	}}
	res, err := p.Ingest(IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}, batch)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Created != 1 || res.Skipped != 1 {
		t.Fatalf("filter not applied: %+v", res)
	}
}
```

> The test builds the archive via the **public** `archive.NewLocalArchive` (the `archive` package's own `setupTestArchive`/`createTestRepo` live in `_test.go` and are **not importable** from another package — and a normal `.go` wrapper can't reference test-only symbols). The local `newTestArchive`/`newTestRepo` helpers above are reused by `registry_test.go` (same package). Field/const names below are already verified against the code: `core.Condition{Field, Op ConditionOp, Value json.RawMessage}`, `core.OpEq`, `core.Filter{Mode, Conditions}`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ingest/ -run TestPipeline`
Expected: FAIL — `Pipeline`/`NewPipeline`/`Ingest` undefined.

- [ ] **Step 3: Implement the pipeline**

Create `ingest/pipeline.go`:

```go
package ingest

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/filter"
)

// IngestContext carries the destination repo and ingestion provenance for a
// batch. Source is nil for ad-hoc ingestion (one-shot webhook): such nodes get
// no provenance, no dedup, and no source filters. When Source is non-nil, items
// are deduplicated by (Source.ID, ExternalID), filtered through the source's
// filter chain, and stamped with provenance.
type IngestContext struct {
	RepoID    uuid.UUID
	Source    *core.Source
	CreatedBy uuid.UUID
}

// IngestResult summarises what a batch produced.
type IngestResult struct {
	Created int
	Updated int
	Skipped int // filtered out by the source filter chain
}

// Pipeline turns adapter-emitted IngestBatches into persisted graph entities,
// reusing the canonical write+event path (SaveBatchNodes/SaveNode +
// PublishEvent) so rules, realtime, and search light up for ingested data with
// no extra wiring. A2 implements the Item path; Threads, Annotations,
// Tombstones, reply Edges, and author->User resolution land in Sub-project B.
type Pipeline struct {
	archive core.Archive
	pub     app.Publisher
}

// NewPipeline creates a pipeline. pub may be nil (events are then skipped).
func NewPipeline(archive core.Archive, pub app.Publisher) *Pipeline {
	return &Pipeline{archive: archive, pub: pub}
}

// Ingest persists a batch's Items into ctx.RepoID. Items entering through a
// Source are deduplicated by provenance (re-collection updates the existing
// node) and filtered through the source's chain.
func (p *Pipeline) Ingest(ctx IngestContext, batch *core.IngestBatch) (IngestResult, error) {
	var res IngestResult
	if batch == nil {
		return res, nil
	}

	created := make([]*core.Node, 0, len(batch.Items))
	var updated []*core.Node

	for _, item := range batch.Items {
		if item.Node == nil {
			continue
		}

		existing, err := p.lookupExisting(ctx, item)
		if err != nil {
			return res, err
		}

		node := mapNode(ctx, item, existing)

		if ctx.Source != nil {
			match, err := p.applySourceFilters(ctx.Source, node)
			if err != nil {
				return res, err
			}
			if !match {
				res.Skipped++
				continue
			}
		}

		if existing != nil {
			updated = append(updated, node)
		} else {
			created = append(created, node)
		}
	}

	if len(created) > 0 {
		if err := p.archive.SaveBatchNodes(created); err != nil {
			return res, fmt.Errorf("ingest: save created nodes: %w", err)
		}
		for _, n := range created {
			p.publish(ctx.RepoID, core.EventNodeCreated, core.EventNodeCreatePayload{Node: n})
		}
		res.Created = len(created)
	}

	for _, n := range updated {
		if err := p.archive.SaveNode(n); err != nil {
			return res, fmt.Errorf("ingest: update node: %w", err)
		}
		p.publish(ctx.RepoID, core.EventNodeUpdated, core.EventNodeUpdatePayload{NodeID: n.ID, UpdatedNode: n})
		res.Updated++
	}

	// Persist the advanced cursor (source-based ingestion only).
	if batch.Cursor != nil && ctx.Source != nil {
		batch.Cursor.SourceID = ctx.Source.ID
		if err := p.archive.SaveIngestCursor(batch.Cursor); err != nil {
			return res, fmt.Errorf("ingest: persist cursor: %w", err)
		}
	}

	return res, nil
}

// lookupExisting finds a prior node for dedup. Only source-based items with a
// non-empty ExternalID can dedup; ad-hoc items always create.
func (p *Pipeline) lookupExisting(ctx IngestContext, item core.IngestItem) (*core.Node, error) {
	if ctx.Source == nil || item.ExternalID == "" {
		return nil, nil
	}
	n, err := p.archive.GetNodeBySourceExternalID(ctx.Source.ID, item.ExternalID)
	if err == nil {
		return n, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: dedup lookup: %w", err)
}

// mapNode builds the node to persist. A new node (Version 0) is assigned
// identity + provenance by the archive on save; an existing node is updated in
// place, preserving ID/provenance/version (the archive bumps the version and
// leaves provenance columns untouched on update).
func mapNode(ctx IngestContext, item core.IngestItem, existing *core.Node) *core.Node {
	src := item.Node
	if existing != nil {
		existing.Type = src.Type
		existing.Subject = src.Subject
		existing.ContentType = src.ContentType
		existing.Body = src.Body
		existing.Metadata = src.Metadata
		return existing
	}
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      ctx.RepoID,
		Type:        src.Type,
		Subject:     src.Subject,
		ContentType: src.ContentType,
		Body:        src.Body,
		Metadata:    src.Metadata,
		CreatedBy:   ctx.CreatedBy,
	}
	if ctx.Source != nil {
		sid := ctx.Source.ID
		node.SourceID = &sid
		if item.ExternalID != "" {
			ext := item.ExternalID
			node.ExternalID = &ext
		}
	}
	return node
}

// applySourceFilters evaluates the source's filter chain against the normalised
// node. Empty chain matches. FilterModeAll requires every filter to match;
// FilterModeAny requires at least one.
func (p *Pipeline) applySourceFilters(source *core.Source, node *core.Node) (bool, error) {
	if len(source.FilterIDs) == 0 {
		return true, nil
	}
	payload := nodePayload(node)
	anyMatched := false
	for _, fid := range source.FilterIDs {
		f, err := p.archive.GetFilter(fid)
		if err != nil {
			return false, fmt.Errorf("ingest: load source filter %s: %w", fid, err)
		}
		matched := filter.Evaluate(f, payload).Matches
		if source.FilterChainMode == core.FilterModeAll && !matched {
			return false, nil
		}
		if matched {
			anyMatched = true
		}
	}
	if source.FilterChainMode == core.FilterModeAny {
		return anyMatched, nil
	}
	return true, nil
}

// nodePayload projects a node into the map the filter engine evaluates. Metadata
// is flattened to key->raw-value; body is exposed as a string for text filters.
func nodePayload(n *core.Node) map[string]any {
	meta := make(map[string]any, len(n.Metadata))
	for _, m := range n.Metadata {
		meta[m.Key] = m.Value
	}
	return map[string]any{
		"type":         n.Type,
		"subject":      n.Subject,
		"content_type": n.ContentType,
		"body":         string(n.Body),
		"metadata":     meta,
	}
}

// publish emits a domain event, fire-and-forget. A nil publisher is a no-op.
func (p *Pipeline) publish(repoID uuid.UUID, t core.EventType, payload any) {
	if p.pub == nil {
		return
	}
	if err := p.pub.PublishEvent(t, repoID, payload); err != nil {
		slog.Error("ingest: publish event failed", "type", t, "error", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./ingest/`
Expected: PASS (registry + pipeline tests). Use the FTS5 tag because the pipeline writes nodes, which the archive indexes for search.

- [ ] **Step 5: Commit**

```bash
git add ingest/pipeline.go ingest/pipeline_test.go
git commit -m "feat(ingest): Item-path pipeline reusing the write+event path

map -> filter -> persist -> dedup -> events for IngestBatch.Items. Source-based
ingestion dedups by (Source,ExternalID), applies the source filter chain, and
stamps provenance; ad-hoc ingestion does none. Threads/Annotations/Tombstones/
reply-Edges/author resolution deferred to Sub-project B.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: The webhook adapter (`integrations/webhook`)

A connection-less `core.Adapter` + `core.Collector`. It fetches a URL (read from `req.Source.Config`) and emits one `IngestItem`; identity, filtering, and persistence stay in the pipeline. `Node.Body` is `json.RawMessage`, so fetched bytes are wrapped as a JSON string.

**Files:**
- Create: `integrations/webhook/webhook.go`
- Test: `integrations/webhook/webhook_test.go`

- [ ] **Step 1: Write the failing tests**

Create `integrations/webhook/webhook_test.go`:

```go
package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestAdapter_Collect_FetchesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	a := New()
	src := &core.Source{Config: map[string]any{"url": srv.URL}}
	batch, err := a.Collect(context.Background(), core.CollectRequest{Source: src})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(batch.Items))
	}
	item := batch.Items[0]
	if item.ExternalID != srv.URL {
		t.Fatalf("external id = %q, want %q", item.ExternalID, srv.URL)
	}
	var body string
	if err := json.Unmarshal(item.Node.Body, &body); err != nil {
		t.Fatalf("body is not a JSON string: %v", err)
	}
	if body != "hello world" {
		t.Fatalf("body = %q", body)
	}
}

func TestAdapter_Collect_MissingURL(t *testing.T) {
	a := New()
	if _, err := a.Collect(context.Background(), core.CollectRequest{Source: &core.Source{Config: map[string]any{}}}); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestAdapter_Collect_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := New()
	src := &core.Source{Config: map[string]any{"url": srv.URL}}
	if _, err := a.Collect(context.Background(), core.CollectRequest{Source: src}); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestAdapter_Contract(t *testing.T) {
	a := New()
	if a.Type() != core.ConnectorWebhook {
		t.Fatalf("type = %q", a.Type())
	}
	if !a.Capabilities().Collect || a.Capabilities().RequiresConnection {
		t.Fatalf("unexpected capabilities: %+v", a.Capabilities())
	}
	if err := a.ValidateSourceConfig(map[string]any{}); err == nil {
		t.Fatal("expected source config to require url")
	}
	if err := a.ValidateSinkConfig(map[string]any{}); err == nil {
		t.Fatal("webhook must reject sink config (collect-only)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./integrations/webhook/`
Expected: FAIL — package / `New` undefined.

- [ ] **Step 3: Implement the adapter**

Create `integrations/webhook/webhook.go`:

```go
// Package webhook implements a connection-less core.Adapter that collects a
// single URL into one IngestItem. It is the proving adapter for the A2 registry
// + ingest pipeline. Identity, filtering, and persistence are the pipeline's
// job; this adapter only fetches and shapes content.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/upspeak/upspeak/core"
)

// maxBodyBytes bounds a fetched body to protect memory.
const maxBodyBytes = 8 << 20 // 8 MiB

// Adapter fetches a URL and emits one IngestItem.
type Adapter struct {
	client *http.Client
}

// New creates a webhook adapter with a bounded HTTP client.
func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 30 * time.Second}}
}

// Type identifies the connector.
func (a *Adapter) Type() core.ConnectorType { return core.ConnectorWebhook }

// Capabilities: collect-only, no Connection required.
func (a *Adapter) Capabilities() core.AdapterCapabilities {
	return core.AdapterCapabilities{Collect: true, RequiresConnection: false}
}

// ConnectionSchema is empty: webhook is connection-less.
func (a *Adapter) ConnectionSchema() core.ConnectionSchema { return core.ConnectionSchema{} }

// ValidateConnectionConfig accepts anything: there is no Connection.
func (a *Adapter) ValidateConnectionConfig(map[string]any) error { return nil }

// ValidateSourceConfig requires a non-empty url.
func (a *Adapter) ValidateSourceConfig(cfg map[string]any) error {
	if u, _ := cfg["url"].(string); u == "" {
		return errors.New("webhook source requires config.url")
	}
	return nil
}

// ValidateSinkConfig rejects: webhook is collect-only.
func (a *Adapter) ValidateSinkConfig(map[string]any) error {
	return errors.New("webhook connector does not support publishing")
}

// TestConnection is a no-op: there is no Connection to test.
func (a *Adapter) TestConnection(context.Context, *core.Connection) error { return nil }

// Collect fetches req.Source.Config["url"] and returns one IngestItem. For
// one-shot jobs the runner supplies an ephemeral Source carrying the URL.
func (a *Adapter) Collect(ctx context.Context, req core.CollectRequest) (*core.IngestBatch, error) {
	if req.Source == nil {
		return nil, errors.New("webhook collect requires a source")
	}
	url, _ := req.Source.Config["url"].(string)
	if url == "" {
		return nil, errors.New("webhook source missing config.url")
	}
	contentType, _ := req.Source.Config["content_type"].(string)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: unexpected status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", url, err)
	}
	if contentType == "" {
		contentType = resp.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "text/plain"
	}

	// Node.Body is json.RawMessage; wrap fetched content as a JSON string.
	bodyJSON, err := json.Marshal(string(raw))
	if err != nil {
		return nil, fmt.Errorf("encode body: %w", err)
	}
	urlJSON, _ := json.Marshal(url)

	item := core.IngestItem{
		ExternalID: url,
		Node: &core.Node{
			Type:        "webpage",
			Subject:     url,
			ContentType: contentType,
			Body:        bodyJSON,
			Metadata:    []core.Metadata{{Key: "source_url", Value: urlJSON}},
		},
	}
	return &core.IngestBatch{Items: []core.IngestItem{item}}, nil
}

// Compile-time contract checks.
var (
	_ core.Adapter   = (*Adapter)(nil)
	_ core.Collector = (*Adapter)(nil)
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./integrations/webhook/`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add integrations/webhook/
git commit -m "feat(integrations/webhook): connection-less Collector adapter

Fetches a URL into a single IngestItem; the A2 registry+pipeline proving
adapter. Body wrapped as a JSON string (Node.Body is json.RawMessage).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Migrate the job runner's webhook path onto the registry + pipeline

Inject `app.Publisher`, `app.AdapterRegistry`, and an `ingest.Pipeline` into the runner. Rewrite `executeWebhook` to dispatch through the registry and persist via the pipeline. `executeCollect`/`executePublish` stay as-is (their full wiring is Sub-project B).

**Files:**
- Modify: `jobs/runner.go`
- Test: `jobs/runner_test.go`

- [ ] **Step 1: Write the failing test**

Create `jobs/runner_test.go`:

```go
package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/ingest"
	"github.com/upspeak/upspeak/integrations/webhook"
)

var testOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// newTestArchive / newTestRepo build a real archive via the public constructor
// (the archive package's own test helpers are not importable across packages).
func newTestArchive(t *testing.T) *archive.LocalArchive {
	t.Helper()
	a, err := archive.NewLocalArchive(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalArchive: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func newTestRepo(t *testing.T, a *archive.LocalArchive) *core.Repository {
	t.Helper()
	repo := &core.Repository{ID: core.NewID(), Slug: "test-repo", Name: "Test Repository", OwnerID: testOwnerID}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("SaveRepository: %v", err)
	}
	return repo
}

func TestExecuteWebhook_PersistsNode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body-content"))
	}))
	defer srv.Close()

	a := newTestArchive(t)
	repo := newTestRepo(t, a)

	reg := ingest.NewRegistry()
	reg.Register(webhook.New())

	// nil consumer + nil publisher: we call executeWebhook directly, not Run.
	r := NewRunner(a, nil, nil, reg)

	params, _ := json.Marshal(map[string]string{"url": srv.URL})
	job := &core.Job{ID: core.NewID(), RepoID: repo.ID, Type: core.JobWebhook, Params: params}

	out, err := r.executeWebhook(job)
	if err != nil {
		t.Fatalf("executeWebhook: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if res["status"] != "success" {
		t.Fatalf("status = %v", res["status"])
	}

	nodes, _, err := a.ListNodes(repo.ID, core.NodeListOptions{})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 ingested node, got %d", len(nodes))
	}
	if nodes[0].SourceID != nil {
		t.Fatal("one-shot webhook node must have no source provenance")
	}
}

func TestExecuteWebhook_NoAdapter(t *testing.T) {
	a := newTestArchive(t)
	repo := newTestRepo(t, a)
	r := NewRunner(a, nil, nil, ingest.NewRegistry()) // empty registry

	params, _ := json.Marshal(map[string]string{"url": "http://example.com"})
	job := &core.Job{ID: core.NewID(), RepoID: repo.ID, Type: core.JobWebhook, Params: params}
	if _, err := r.executeWebhook(job); err == nil {
		t.Fatal("expected error when no webhook adapter is registered")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags sqlite_fts5 ./jobs/ -run TestExecuteWebhook`
Expected: FAIL — `NewRunner` takes 2 args, not 4.

- [ ] **Step 3: Extend the runner struct + constructor**

In `jobs/runner.go`, update the imports to add `"context"` (already present) and:

```go
	"github.com/upspeak/upspeak/ingest"
```

Replace the `Runner` struct and `NewRunner` with:

```go
// Runner consumes jobs from the JOBS JetStream stream and executes them.
// It runs in a goroutine started via Run and stopped via the context.
type Runner struct {
	archive  core.Archive
	consumer app.Consumer
	pub      app.Publisher
	registry app.AdapterRegistry
	pipeline *ingest.Pipeline
}

// NewRunner creates a new job runner. pub and registry are injected so the
// runner dispatches to adapters through interfaces and never imports the
// concrete integrations (breaking the connector<->jobs cycle). pub may be nil
// in tests that call execute* directly.
func NewRunner(archive core.Archive, consumer app.Consumer, pub app.Publisher, registry app.AdapterRegistry) *Runner {
	return &Runner{
		archive:  archive,
		consumer: consumer,
		pub:      pub,
		registry: registry,
		pipeline: ingest.NewPipeline(archive, pub),
	}
}
```

- [ ] **Step 4: Rewrite executeWebhook + add the repoOwner helper**

In `jobs/runner.go`, replace the body of `executeWebhook` with:

```go
func (r *Runner) executeWebhook(job *core.Job) (json.RawMessage, error) {
	var params webhookParams
	if len(job.Params) > 0 {
		if err := json.Unmarshal(job.Params, &params); err != nil {
			return nil, fmt.Errorf("invalid webhook params: %w", err)
		}
	}
	if params.URL == "" {
		return nil, errors.New("url is required for webhook jobs")
	}

	adapter, ok := r.registry.AdapterFor(core.ConnectorWebhook)
	if !ok {
		return nil, errors.New("no adapter registered for connector: webhook")
	}
	collector, ok := adapter.(core.Collector)
	if !ok {
		return nil, errors.New("webhook adapter does not support collect")
	}

	createdBy, err := r.repoOwner(job.RepoID)
	if err != nil {
		return nil, err
	}

	// Ephemeral, unpersisted Source carries the one-shot URL to the adapter.
	// Because it is not persisted, ingested nodes get no source provenance.
	ephemeral := &core.Source{
		RepoID:    job.RepoID,
		Connector: core.ConnectorWebhook,
		Config:    map[string]any{"url": params.URL, "content_type": params.ContentType},
	}

	batch, err := collector.Collect(context.Background(), core.CollectRequest{Source: ephemeral})
	if err != nil {
		return nil, fmt.Errorf("webhook collect failed: %w", err)
	}

	res, err := r.pipeline.Ingest(ingest.IngestContext{
		RepoID:    job.RepoID,
		Source:    nil, // ad-hoc: no provenance, no dedup, no filters
		CreatedBy: createdBy,
	}, batch)
	if err != nil {
		return nil, fmt.Errorf("webhook ingest failed: %w", err)
	}

	result, _ := json.Marshal(map[string]any{
		"url":     params.URL,
		"created": res.Created,
		"updated": res.Updated,
		"skipped": res.Skipped,
		"status":  "success",
	})
	return result, nil
}

// repoOwner resolves a repo's owner, used as CreatedBy for ingested nodes until
// author->User resolution lands in Sub-project B.
func (r *Runner) repoOwner(repoID uuid.UUID) (uuid.UUID, error) {
	repo, err := r.archive.GetRepository(repoID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve repo owner: %w", err)
	}
	return repo.OwnerID, nil
}
```

> Leave `executeCollect`, `executePublish`, and `executeSync` unchanged. Add a one-line comment above `executeCollect` and `executePublish`: `// NOTE: registry+pipeline dispatch (cursors, filter-on-normalised, dedup) lands in Sub-project B.`

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags sqlite_fts5 ./jobs/ -run TestExecuteWebhook`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add jobs/runner.go jobs/runner_test.go
git commit -m "feat(jobs): migrate webhook execution onto registry + ingest pipeline

The runner now dispatches webhook jobs through the injected AdapterRegistry and
persists via the ingest pipeline (reusing the write+event path), so it imports
no integrations -- preventing the connector<->jobs cycle. executeCollect/
executePublish stay stubs until Sub-project B.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Wire the registry in main.go

Build the registry, register the webhook adapter, and inject it into the job runner.

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add imports**

In `main.go`, add to the import block:

```go
	"github.com/upspeak/upspeak/ingest"
	"github.com/upspeak/upspeak/integrations/webhook"
```

- [ ] **Step 2: Build the registry before the runners slice**

In `main.go`, immediately before the `runners := []app.Runner{` line, add:

```go
	// Build the adapter registry from the compiled-in integrations. main.go is
	// the only place that knows the concrete adapters; jobs/connector consume
	// the app.AdapterRegistry interface, so no import cycle forms.
	adapterRegistry := ingest.NewRegistry()
	adapterRegistry.Register(webhook.New())
```

- [ ] **Step 3: Inject into the job runner**

In the `runners := []app.Runner{ ... }` literal, replace:

```go
		jobs.NewRunner(a, jobConsumer),
```

with:

```go
		jobs.NewRunner(a, jobConsumer, bus.Publisher(), adapterRegistry),
```

- [ ] **Step 4: Build and run the full sweep**

Run: `go build ./... && go test -tags sqlite_fts5 ./...`
Expected: builds; all packages pass (including the new `ingest`, `integrations/webhook`, and `jobs` tests). `go test ./...` without the tag also passes (search skipped).

- [ ] **Step 5: Verify no import cycle and no stray integration imports**

Run:
```bash
go list -deps ./jobs/ | grep -E 'upspeak/(connector|integrations)' && echo "LEAK" || echo "clean"
```
Expected: `clean` — the job package depends on neither `connector` nor `integrations`.

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat(main): build adapter registry and inject into the job runner

Registers the webhook adapter and wires the registry into jobs.NewRunner.
main.go is the sole owner of concrete-adapter knowledge.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Documentation

**Files:**
- Modify: `docs/next-steps.md`

- [ ] **Step 1: Update status + carry-forward watch-outs**

In `docs/next-steps.md`, find the connector-adapterisation / ingestion-track status and:
- Mark **A2 (registry + ingest pipeline + webhook adapter) shipped**.
- Record the **new A2b slice**: repo→repo adapter (net-new internal data movement, archive-read exception, cycle-detection interplay) — split out of A2.
- Carry these watch-outs forward to **B**:
  - `executeCollect`/`executePublish` are still stubs — B wires them through the registry + pipeline (cursors via `Get/SaveIngestCursor`, filter-on-normalised, dedup), then emits `CollectionCompleted`/`PublishCompleted`.
  - The pipeline implements the **Item path only**; B adds Threads (needs thread provenance — not in the A1 schema), Annotations, Tombstones, reply Edges (`ParentExternalID`), and author→User resolution (needs a user store).
- Carry these to **A3** (unchanged from A1's review): wire `secrets.NewCipherFromEnv()` / `SetSecretCipher` at runtime; enforce the connector↔connection match on source/sink create/update; 409 on deleting an in-use connection; credential update is load-modify-save.

- [ ] **Step 2: Commit**

```bash
git add docs/next-steps.md
git commit -m "docs(next-steps): mark A2 shipped; add A2b; carry watch-outs to B/A3

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-review

**Spec coverage (design §3.6, §6.1, §7; handoff A2):**
- `app.AdapterRegistry` interface (design §3.6: "lives alongside app.Publisher/app.Consumer, injected via setter") — Task 1. ✓
- Concrete registry in `ingest` — Task 2. ✓
- Ingest pipeline `map → filter → persist → dedup → events`, reusing `SaveBatchNodes`/`SaveNode` + `PublishEvent` (design §6.1) — Task 3. ✓ (Item path; Threads/Annotations/Tombstones/Edges/author → B, documented.)
- Webhook migrated onto a `core.Adapter` + `core.Collector` (handoff "migrate webhook") — Tasks 4, 5. ✓
- jobs DI breaking the `connector ↔ jobs` cycle (`jobs` → `app.AdapterRegistry`, never `connector`/`integrations`) — Tasks 1, 5, 6; verified in Task 6 Step 5. ✓
- Registry built from integrations in `main.go`, sole owner of concrete adapters (design §7) — Task 6. ✓
- `repo` migration — **deferred to A2b** (Scope decisions); recorded in Task 7. ✓ (intentional, not a gap.)
- `executeCollect`/`executePublish` full wiring — **Sub-project B** (design §8); left as stubs, noted in Tasks 5, 7. ✓ (intentional.)

**Placeholder scan:** none — every step has concrete code or an exact command. All cross-package names are verified against the code: the public constructor `archive.NewLocalArchive` (the package's own `setupTestArchive` is test-only and not importable, so the `ingest`/`jobs` tests define local `newTestArchive`/`newTestRepo` helpers); `core.Condition{Field, Op ConditionOp, Value json.RawMessage}` with `core.OpEq`; `core.Filter{Mode, Conditions}`; `filter.Evaluate(...).Matches`; `Repository.OwnerID`; `Node.Body json.RawMessage`.

**Type consistency:** `NewRunner(core.Archive, app.Consumer, app.Publisher, app.AdapterRegistry)` (Task 5) matches the call site (Task 6 Step 3). `ingest.NewPipeline(core.Archive, app.Publisher)` (Task 3) matches its use in `NewRunner` (Task 5) and tests (Tasks 3, 5). `ingest.IngestContext{RepoID, Source, CreatedBy}` and `IngestResult{Created, Updated, Skipped}` (Task 3) match every call site (Tasks 3, 5 tests + `executeWebhook`). `Registry.AdapterFor` / `Register` (Task 2) match `app.AdapterRegistry` (Task 1) and `main.go` (Task 6). `core.Adapter` + `core.Collector` method sets (Task 4) match the A1 `core/adapter.go` contract. `Node.Body` (`json.RawMessage`) is JSON-wrapped in both the webhook adapter (Task 4) and the test helper `textBody` (Task 3). `filter.Evaluate(f, payload).Matches` (Task 3) matches `rules/engine.go` usage.

> **Tag reminder:** archive-touching tests/builds (`ingest`, `jobs`, the full sweep) use `-tags sqlite_fts5`, because writing nodes triggers the search-index path.

---

## Watch-outs for B / A2b / A3 (carried forward)

- **B — collect/publish wiring.** `executeCollect`/`executePublish` still stub. B resolves Source/Sink (+ Connection), looks up the adapter via `r.registry.AdapterFor`, loads/persists the cursor (`Get/SaveIngestCursor`), dispatches `Collect`/`Publish`, runs `pipeline.Ingest`, records history, and emits `CollectionCompleted`/`PublishCompleted`. The machinery (registry, pipeline, runner DI) is already in place from A2.
- **B — pipeline beyond Items.** Threads need **thread provenance** (the A1 schema only added provenance to `nodes`; adding `(source_id, external_thread_id)` to threads is a B schema change). Annotations need target-node resolution; reply Edges need `ParentExternalID` → parent-node lookup; author resolution needs a **user store** (not on `core.Archive` yet).
- **A2b — repo→repo adapter.** Net-new internal data movement; decide archive-read access (documented internal exception vs. runner-side read with a thin mapping adapter), and reconcile with `connector/cycle.go`.
- **A3 — cipher + connection lifecycle** (from A1 review): wire `secrets.NewCipherFromEnv()` / `LocalArchive.SetSecretCipher` at runtime; enforce connector↔connection match; 409 on in-use connection delete; credential update is load-modify-save.
- **Trust the build, not the editor.** Per the A1 session, LSP diagnostics lagged the real compiler repeatedly — verify with `go build` / `go test -tags sqlite_fts5`, and confirm `git branch --show-current` before risky ops.
