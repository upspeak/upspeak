# A2b — repo→repo over NATS: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move a knowledge graph (nodes + edges + threads + annotations) from one Upspeak repository to another entirely over NATS, with each repo controlling what it publishes (its Sink) and what it ingests (its Source).

**Architecture:** Two `app.Runner` supervisors, both single global durable JetStream consumers (the `rules-engine` pattern). The **publish supervisor** consumes `REPO_EVENTS`, applies each Sink's publication filter, and republishes the curated subset to a new `SINK_EVENTS` stream. The **ingest supervisor** consumes `SINK_EVENTS` and runs each subscribing Source's events through the (now extended) `ingest` pipeline into the destination repo. repo→repo carries no external transport, so there is **no adapter** — the supervisors + pipeline + filter engine are the whole mechanism.

**Tech Stack:** Go; SQLite (`archive/`, build tag `sqlite_fts5`); NATS JetStream (isolated in `nats/`, reached via `app.Publisher`/`app.Consumer`); the existing `filter` engine and `ingest` pipeline.

**Spec:** `docs/design/2026-06-06-subproject-a2b-repo-repo.md`. Read it first.

**Conventions (CLAUDE.md):** GoDoc on all exported symbols; en-IN spelling (organise, behaviour); check errors immediately; no `panic`; small commits per task. Build/test archive paths with the FTS5 tag: `go test -tags sqlite_fts5 ./...`. **Trust `go build`/`go test`, not the LSP** (this repo's LSP reports phantom diagnostics).

---

## File Map

| Path | Change | Responsibility |
|---|---|---|
| `core/core.go` (`Edge`), `core/thread.go`, `core/annotation.go` | modify | add `SourceID`/`ExternalID` provenance |
| `core/ingest.go` | modify | add `IngestEdge` + `IngestBatch.Edges` |
| `core/subjects.go` | modify | add `SinkSubject` + `SinkEventsSubject` |
| `archive/schema.go` | modify | provenance columns + indexes on edges/threads/annotations |
| `archive/edge_store.go`, `thread_store.go`, `annotation_store.go` | modify | persist + scan provenance; `GetXBySourceExternalID` |
| `archive/source_store.go` | modify | `ListRepoSourcesForSink` (cross-repo) |
| `core/archive.go` | modify | extend `EdgeStore`/`ThreadStore`/`AnnotationStore`/`SourceStore` |
| `nats/streams.go` | modify | `CreateSinkEventsStream` + names |
| `nats/consumers.go` | modify | `sink-publisher` + `repo-ingest` durable consumers |
| `ingest/pipeline.go` | modify | process Threads/Edges/Annotations; shared filter helper |
| `ingest/supervisor.go` | create | ingest supervisor (`SINK_EVENTS` → pipeline) |
| `connector/publish_supervisor.go` | create | publish supervisor (`REPO_EVENTS` → filter → `SINK_EVENTS`) |
| `connector/cycle.go` | modify | Source-graph BFS via `sink_id` |
| `connector/handlers_source.go`, `handlers_sink.go` | modify | repo Source `sink_id`; repo Sink target-agnostic |
| `main.go` | modify | create stream + consumers; start both supervisors |

---

## Task 1: Core provenance fields + `IngestEdge`

**Files:**
- Modify: `core/core.go` (`Edge` struct, ~`38-52`), `core/thread.go`, `core/annotation.go`
- Modify: `core/ingest.go`
- Test: `core/ingest_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
// core/ingest_test.go
package core

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEdgeProvenanceRoundTrip(t *testing.T) {
	sid := uuid.New()
	ext := "ext-42"
	e := Edge{ID: uuid.New(), SourceID: &sid, ExternalID: &ext}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Edge
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid || got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("provenance not preserved: %+v", got)
	}
}

func TestIngestBatchHasEdges(t *testing.T) {
	b := IngestBatch{Edges: []IngestEdge{{ExternalID: "e1", SourceExternalID: "n1", TargetExternalID: "n2", Type: "reply"}}}
	if len(b.Edges) != 1 || b.Edges[0].SourceExternalID != "n1" {
		t.Fatalf("edges field missing/wrong: %+v", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/ -run 'TestEdgeProvenance|TestIngestBatchHasEdges' -v`
Expected: FAIL — `Edge` has no `SourceID`/`ExternalID`; `IngestEdge`/`IngestBatch.Edges` undefined.

- [ ] **Step 3: Add provenance to `Edge`, `Thread`, `Annotation`**

In `core/core.go`, inside `Edge` (after `Weight`, before `CreatedBy`), add — copy the GoDoc style from `Node` (`core/core.go:26-31`):

```go
	// SourceID and ExternalID record ingestion provenance: which Source produced
	// this edge and its stable identifier in the source system. Both nil for
	// locally-created edges. Together they give idempotent re-ingestion.
	SourceID   *uuid.UUID `json:"source_id,omitempty"`
	ExternalID *string    `json:"external_id,omitempty"`
```

Add the **same two fields** (with wording "thread"/"annotation") to the `Thread` struct in `core/thread.go` and the `Annotation` struct in `core/annotation.go`, each just before `CreatedBy`.

- [ ] **Step 4: Add `IngestEdge` + `Edges` to `core/ingest.go`**

In `IngestBatch` (after the `Items` line), add:

```go
	Edges []IngestEdge `json:"edges,omitempty"` // explicit relationships → Edges
```

After the `IngestThread` type, add:

```go
// IngestEdge is one explicit relationship between two external entities. The
// pipeline resolves SourceExternalID/TargetExternalID to the destination repo's
// node UUIDs via node provenance and skips the edge when either is absent.
type IngestEdge struct {
	ExternalID       string  `json:"external_id"`
	SourceExternalID string  `json:"source_external_id"`
	TargetExternalID string  `json:"target_external_id"`
	Type             string  `json:"type"`
	Label            string  `json:"label,omitempty"`
	Weight           float64 `json:"weight,omitempty"`
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./core/ -run 'TestEdgeProvenance|TestIngestBatchHasEdges' -v` → PASS
Then `go build ./...` → clean.

- [ ] **Step 6: Commit**

```bash
git add core/
git commit -m "feat(core): add provenance to Edge/Thread/Annotation; IngestEdge"
```

---

## Task 2: `core.SinkSubject`

**Files:**
- Modify: `core/subjects.go`
- Test: `core/subjects_test.go` (create or extend)

- [ ] **Step 1: Write the failing test**

```go
// core/subjects_test.go
package core

import (
	"testing"

	"github.com/google/uuid"
)

func TestSinkSubject(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := SinkSubject(id, EventNodeCreated)
	want := "sink.11111111-1111-1111-1111-111111111111.events.NodeCreated"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test** → FAIL (`SinkSubject` undefined). `go test ./core/ -run TestSinkSubject -v`

- [ ] **Step 3: Implement** in `core/subjects.go`:

```go
// SinkEventsSubjectAll matches every Sink's curated outbound events. Used as the
// JetStream stream subject and as the ingest supervisor's consumer filter.
const SinkEventsSubjectAll = "sink.*.events.>"

// SinkSubject returns the canonical subject for a Sink's curated outbound event.
// Format: sink.{sink_id}.events.{EventType}. The publish supervisor publishes
// here; a subscribing repo's ingest supervisor consumes it.
func SinkSubject(sinkID uuid.UUID, eventType EventType) string {
	return fmt.Sprintf("sink.%s.events.%s", sinkID.String(), eventType)
}
```

Ensure `core/subjects.go` imports `fmt` and `github.com/google/uuid`.

- [ ] **Step 4: Run test** → PASS. `go build ./...` → clean.

- [ ] **Step 5: Commit**

```bash
git add core/subjects.go core/subjects_test.go
git commit -m "feat(core): add SinkSubject for curated outbound events"
```

---

## Task 3: Archive provenance — schema + store persistence

**Files:**
- Modify: `archive/schema.go` (edges `41-57`, threads `60-72`, annotations `84+`)
- Modify: `archive/edge_store.go`, `archive/thread_store.go`, `archive/annotation_store.go`
- Test: `archive/provenance_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
// archive/provenance_test.go
//go:build sqlite_fts5

package archive

import (
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func TestEdgeProvenancePersists(t *testing.T) {
	a, err := NewLocalArchive(t.TempDir())
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	repo := &core.Repository{ID: core.NewID(), Name: "r", Slug: "r", OwnerID: core.NewID()}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("repo: %v", err)
	}
	src, tgt := saveBareNode(t, a, repo.ID), saveBareNode(t, a, repo.ID)
	sid := uuid.New()
	ext := "edge-ext-1"
	e := &core.Edge{RepoID: repo.ID, Type: "rel", Source: src, Target: tgt, Weight: 1, CreatedBy: repo.OwnerID, SourceID: &sid, ExternalID: &ext}
	if err := a.SaveEdge(e); err != nil {
		t.Fatalf("save edge: %v", err)
	}
	got, err := a.GetEdge(e.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid || got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("edge provenance lost: %+v", got)
	}
}

// saveBareNode creates a minimal node and returns its ID.
func saveBareNode(t *testing.T, a *LocalArchive, repoID uuid.UUID) uuid.UUID {
	t.Helper()
	n := &core.Node{RepoID: repoID, Type: "note", Subject: "s", ContentType: "text/plain", Body: []byte(`""`), CreatedBy: core.NewID()}
	if err := a.SaveNode(n); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return n.ID
}
```

- [ ] **Step 2: Run test** → FAIL (columns/scan missing).
Run: `go test -tags sqlite_fts5 ./archive/ -run TestEdgeProvenancePersists -v`

- [ ] **Step 3: Add provenance columns in `archive/schema.go`**

In the `edges` table (after `weight`, before `created_by`):

```sql
	source_id   TEXT,
	external_id TEXT,
```

After the edges indexes, add:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_source_external ON edges(source_id, external_id) WHERE source_id IS NOT NULL;
```

Repeat the same two columns + a matching `idx_threads_source_external` / `idx_annotations_source_external` partial unique index for the `threads` and `annotations` tables (mirror `nodes`, schema.go `30-31,38`).

- [ ] **Step 4: Persist + scan provenance in the three stores**

In `archive/edge_store.go`'s insert: add `source_id, external_id` to the column list and two values `uuidPtrString(edge.SourceID), strPtrAny(edge.ExternalID)` (helpers already exist in `archive/node_store.go:531,539`). Add the same two columns to every `SELECT` of edges and scan them into `&edge.SourceID`/`&edge.ExternalID` via the same nullable-scan approach `node_store.go` uses (see `scanNodeFromSingleRow` and `node_store.go:480-490`). Do the same in `thread_store.go` and `annotation_store.go`.

> If the stores scan into a `core.Edge` with `sql.Null*` temporaries, follow `node_store.go`'s exact pattern: scan into `sql.NullString`, then set the pointer only when valid.

- [ ] **Step 5: Run test** → PASS. Add equivalent `TestThreadProvenancePersists` / `TestAnnotationProvenancePersists` and make them pass.

- [ ] **Step 6: Commit**

```bash
git add archive/schema.go archive/edge_store.go archive/thread_store.go archive/annotation_store.go archive/provenance_test.go
git commit -m "feat(archive): persist Edge/Thread/Annotation ingestion provenance"
```

---

## Task 4: Archive `GetXBySourceExternalID` + interface

**Files:**
- Modify: `core/archive.go` (`EdgeStore`, `ThreadStore`, `AnnotationStore`)
- Modify: `archive/edge_store.go`, `thread_store.go`, `annotation_store.go`, and the `LocalArchive` wrappers in `archive/local.go`
- Test: `archive/provenance_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestGetEdgeBySourceExternalID(t *testing.T) {
	a, _ := NewLocalArchive(t.TempDir())
	repo := &core.Repository{ID: core.NewID(), Name: "r", Slug: "r", OwnerID: core.NewID()}
	_ = a.SaveRepository(repo)
	src, tgt := saveBareNode(t, a, repo.ID), saveBareNode(t, a, repo.ID)
	sid := uuid.New()
	ext := "edge-ext-9"
	_ = a.SaveEdge(&core.Edge{RepoID: repo.ID, Type: "rel", Source: src, Target: tgt, Weight: 1, CreatedBy: repo.OwnerID, SourceID: &sid, ExternalID: &ext})

	got, err := a.GetEdgeBySourceExternalID(sid, ext)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("wrong edge: %+v", got)
	}
	if _, err := a.GetEdgeBySourceExternalID(sid, "missing"); err == nil {
		t.Fatalf("expected ErrorNotFound for missing external id")
	}
}
```

- [ ] **Step 2: Run test** → FAIL (method undefined).
Run: `go test -tags sqlite_fts5 ./archive/ -run TestGetEdgeBySourceExternalID -v`

- [ ] **Step 3: Add interface methods in `core/archive.go`**

In `EdgeStore`: `GetEdgeBySourceExternalID(sourceID uuid.UUID, externalID string) (*Edge, error)`.
In `ThreadStore`: `GetThreadBySourceExternalID(sourceID uuid.UUID, externalID string) (*Thread, error)`.
In `AnnotationStore`: `GetAnnotationBySourceExternalID(sourceID uuid.UUID, externalID string) (*Annotation, error)`.

- [ ] **Step 4: Implement, mirroring `getNodeBySourceExternalID` (`node_store.go:509-528`)**

In `archive/edge_store.go`:

```go
// getEdgeBySourceExternalID finds an edge by ingestion provenance for idempotent
// re-ingestion. Returns ErrorNotFound when no matching edge exists.
func (a *LocalArchive) getEdgeBySourceExternalID(sourceID uuid.UUID, externalID string) (*core.Edge, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, type, source, target, label, weight, source_id, external_id, created_by, version, created_at, updated_at
		FROM edges WHERE source_id = ? AND external_id = ?
	`, sourceID.String(), externalID)
	return scanEdgeFromSingleRow(row) // reuse the existing single-row scanner; extend it to read the two new columns
}
```

Add the `LocalArchive` wrapper in `archive/local.go` next to `GetEdge`:

```go
func (a *LocalArchive) GetEdgeBySourceExternalID(sourceID uuid.UUID, externalID string) (*core.Edge, error) {
	return a.getEdgeBySourceExternalID(sourceID, externalID)
}
```

Repeat for threads (`threads WHERE source_id = ? AND external_id = ?`) and annotations. Use each store's existing single-row scan helper, extended for the two columns in Task 3.

- [ ] **Step 5: Run test** → PASS. `go build ./...` → clean (interface satisfied).

- [ ] **Step 6: Commit**

```bash
git add core/archive.go archive/
git commit -m "feat(archive): GetEdge/Thread/AnnotationBySourceExternalID"
```

---

## Task 5: Archive `ListRepoSourcesForSink`

**Files:**
- Modify: `core/archive.go` (`SourceStore`)
- Modify: `archive/source_store.go`, `archive/local.go`
- Test: `archive/source_store_test.go` (extend) or `archive/provenance_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestListRepoSourcesForSink(t *testing.T) {
	a, _ := NewLocalArchive(t.TempDir())
	repoB := &core.Repository{ID: core.NewID(), Name: "b", Slug: "b", OwnerID: core.NewID()}
	_ = a.SaveRepository(repoB)
	sinkID := uuid.New()
	want := &core.Source{RepoID: repoB.ID, Name: "from-a", Connector: core.ConnectorRepo,
		Config: map[string]any{"sink_id": sinkID.String()}, Status: core.StatusActive, CreatedBy: repoB.OwnerID}
	if err := a.SaveSource(want); err != nil {
		t.Fatalf("save source: %v", err)
	}
	// A repo source pointing at a *different* sink must not match.
	_ = a.SaveSource(&core.Source{RepoID: repoB.ID, Name: "other", Connector: core.ConnectorRepo,
		Config: map[string]any{"sink_id": uuid.New().String()}, Status: core.StatusActive, CreatedBy: repoB.OwnerID})

	got, err := a.ListRepoSourcesForSink(sinkID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != want.ID {
		t.Fatalf("expected exactly the subscribing source, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test** → FAIL. `go test -tags sqlite_fts5 ./archive/ -run TestListRepoSourcesForSink -v`

- [ ] **Step 3: Interface** — add to `SourceStore` in `core/archive.go`:

```go
	// ListRepoSourcesForSink returns every repo-connector Source (across all
	// repositories) whose config references the given upstream Sink id. Used by
	// the ingest supervisor to fan a sink event out to its subscribers.
	ListRepoSourcesForSink(sinkID uuid.UUID) ([]Source, error)
```

- [ ] **Step 4: Implement** in `archive/source_store.go` (cross-repo; filter in Go to avoid SQLite-JSON assumptions):

```go
func (a *LocalArchive) listRepoSourcesForSink(sinkID uuid.UUID) ([]core.Source, error) {
	rows, err := a.db.Query(`
		SELECT id, short_id, repo_id, connection_id, name, connector, config, filter_ids, filter_chain_mode, rate_limit, status, version, created_by, created_at, updated_at
		FROM sources WHERE connector = ?`, string(core.ConnectorRepo))
	if err != nil {
		return nil, fmt.Errorf("query repo sources: %w", err)
	}
	defer rows.Close()

	var out []core.Source
	for rows.Next() {
		s, err := scanSource(rows) // reuse the existing row scanner used by listSources
		if err != nil {
			return nil, err
		}
		if ref, _ := s.Config["sink_id"].(string); ref == sinkID.String() {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}
```

> If `listSources` inlines its scan rather than exposing `scanSource`, extract a `scanSource(rows)` helper first and have both call it (DRY).

Add the `LocalArchive` wrapper in `archive/local.go`:

```go
func (a *LocalArchive) ListRepoSourcesForSink(sinkID uuid.UUID) ([]core.Source, error) {
	return a.listRepoSourcesForSink(sinkID)
}
```

- [ ] **Step 5: Run test** → PASS. `go build ./...` → clean.

- [ ] **Step 6: Commit**

```bash
git add core/archive.go archive/source_store.go archive/local.go archive/*_test.go
git commit -m "feat(archive): ListRepoSourcesForSink (cross-repo subscriber lookup)"
```

---

## Task 6: NATS — `SINK_EVENTS` stream + two consumers

**Files:**
- Modify: `nats/streams.go`, `nats/consumers.go`
- Test: `nats/nats_test.go` (extend, mirror the `JobsStream`/`ConsumerJobRunner` tests)

- [ ] **Step 1: Write the failing test** (mirror `nats_test.go:156-161`)

```go
func TestCreateSinkEventsStream(t *testing.T) {
	bus := startTestBus(t) // use whatever helper the existing nats tests use
	sm := NewStreamManager(bus)
	if err := sm.CreateSinkEventsStream(); err != nil {
		t.Fatalf("create: %v", err)
	}
	info, err := bus.js.StreamInfo(SinkEventsStreamName)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.Config.Retention != nats.LimitsPolicy {
		t.Fatalf("expected Limits retention for fan-out")
	}
}
```

- [ ] **Step 2: Run test** → FAIL. `go test ./nats/ -run TestCreateSinkEventsStream -v`

- [ ] **Step 3: Add the stream** in `nats/streams.go` (mirror `CreateRepoEventsStream:61-76`):

```go
// SinkEventsStreamName is the name of the global curated-outbound stream.
const SinkEventsStreamName = "SINK_EVENTS"

// SinkEventsSubject captures every Sink's curated outbound events.
const SinkEventsSubject = "sink.*.events.>"

// CreateSinkEventsStream creates the global stream that captures every Sink's
// curated outbound events (sink.*.events.>) with Limits retention. Limits (not
// WorkQueue) because many Sources may subscribe to one Sink — the stream is
// fan-out, like REPO_EVENTS.
func (sm *StreamManager) CreateSinkEventsStream() error {
	_, err := sm.js.AddStream(&nats.StreamConfig{
		Name:      SinkEventsStreamName,
		Subjects:  []string{SinkEventsSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("failed to create SINK_EVENTS stream: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Add the two consumers** in `nats/consumers.go` (mirror `CreateRulesEngineConsumer:74-87`). Add constants beside `51-53`:

```go
	ConsumerSinkPublisher = "sink-publisher" // on REPO_EVENTS
	ConsumerRepoIngest    = "repo-ingest"    // on SINK_EVENTS
```

```go
// CreateSinkPublisherConsumer creates the durable consumer the publish
// supervisor uses to read every repository's domain events from REPO_EVENTS.
func (cm *ConsumerManager) CreateSinkPublisherConsumer() error {
	return cm.CreateConsumer(RepoEventsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerSinkPublisher,
		FilterSubject: RepoEventsSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}

// CreateRepoIngestConsumer creates the durable consumer the ingest supervisor
// uses to read curated Sink events from SINK_EVENTS.
func (cm *ConsumerManager) CreateRepoIngestConsumer() error {
	return cm.CreateConsumer(SinkEventsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerRepoIngest,
		FilterSubject: SinkEventsSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}
```

> Match the exact `ConsumerConfig` fields the existing `CreateRulesEngineConsumer` uses (copy them verbatim, changing only `Durable`/`FilterSubject`/stream name); ensure `time` is imported.

- [ ] **Step 5: Run test** → PASS. `go build ./...` → clean.

- [ ] **Step 6: Commit**

```bash
git add nats/streams.go nats/consumers.go nats/nats_test.go
git commit -m "feat(nats): SINK_EVENTS stream + sink-publisher/repo-ingest consumers"
```

---

## Task 7: Pipeline — process Threads, Edges, Annotations

**Files:**
- Modify: `ingest/pipeline.go`
- Test: `ingest/pipeline_test.go` (extend)

The pipeline currently handles Items only (`ingest/pipeline.go:51-117`). Extend `Ingest` to process, in order: Threads → Items (existing) → Edges → Annotations. Reference resolution uses node provenance; an unresolved reference is skipped.

- [ ] **Step 1: Write the failing test**

```go
func TestIngestResolvesEdgeByProvenance(t *testing.T) {
	a, _ := archive.NewLocalArchive(t.TempDir())
	repo := mkRepo(t, a) // existing helper in pipeline_test.go; otherwise create one
	src := mkRepoSource(t, a, repo.ID)

	p := NewPipeline(a, nil)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Two nodes arrive first (external ids n1, n2).
	_, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{
		{ExternalID: "n1", Node: contentNode("a")},
		{ExternalID: "n2", Node: contentNode("b")},
	}})
	if err != nil {
		t.Fatalf("ingest nodes: %v", err)
	}

	// Then an edge referencing them by external id.
	_, err = p.Ingest(ctx, &core.IngestBatch{Edges: []core.IngestEdge{
		{ExternalID: "e1", SourceExternalID: "n1", TargetExternalID: "n2", Type: "reply", Weight: 1},
	}})
	if err != nil {
		t.Fatalf("ingest edge: %v", err)
	}

	got, err := a.GetEdgeBySourceExternalID(src.ID, "e1")
	if err != nil {
		t.Fatalf("edge not persisted: %v", err)
	}
	n1, _ := a.GetNodeBySourceExternalID(src.ID, "n1")
	n2, _ := a.GetNodeBySourceExternalID(src.ID, "n2")
	if got.Source != n1.ID || got.Target != n2.ID {
		t.Fatalf("edge endpoints not re-resolved to local node ids")
	}
}

func TestIngestSkipsEdgeWithMissingEndpoint(t *testing.T) {
	a, _ := archive.NewLocalArchive(t.TempDir())
	repo := mkRepo(t, a)
	src := mkRepoSource(t, a, repo.ID)
	p := NewPipeline(a, nil)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}
	_, _ = p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{{ExternalID: "n1", Node: contentNode("a")}}})

	_, err := p.Ingest(ctx, &core.IngestBatch{Edges: []core.IngestEdge{
		{ExternalID: "e-skip", SourceExternalID: "n1", TargetExternalID: "n-absent", Type: "reply"},
	}})
	if err != nil {
		t.Fatalf("ingest should not error on missing endpoint: %v", err)
	}
	if _, err := a.GetEdgeBySourceExternalID(src.ID, "e-skip"); err == nil {
		t.Fatalf("edge with missing endpoint must be skipped")
	}
}
```

Add small test helpers `contentNode(s string) *core.Node` (returns `&core.Node{Type:"note",Subject:s,ContentType:"text/plain",Body:[]byte(`""`)}`), `mkRepo`, `mkRepoSource` if not already present.

- [ ] **Step 2: Run test** → FAIL (edges not processed). `go test -tags sqlite_fts5 ./ingest/ -run TestIngest -v`

- [ ] **Step 3: Implement edge processing in `Ingest`**

After the Items block (before the cursor persistence at `pipeline.go:108`), add a call `p.ingestEdges(ctx, batch, &res)` and implement:

```go
// ingestEdges persists batch.Edges, resolving each endpoint's external id to a
// local node via provenance. An edge whose source or target node is absent
// (filtered out, or not yet ingested) is skipped, not errored.
func (p *Pipeline) ingestEdges(ctx IngestContext, batch *core.IngestBatch, res *IngestResult) error {
	if ctx.Source == nil {
		return nil // edges require provenance to resolve endpoints
	}
	for _, ie := range batch.Edges {
		srcNode, err := p.resolveNode(ctx.Source.ID, ie.SourceExternalID)
		if err != nil {
			return err
		}
		tgtNode, err := p.resolveNode(ctx.Source.ID, ie.TargetExternalID)
		if err != nil {
			return err
		}
		if srcNode == nil || tgtNode == nil {
			res.Skipped++
			continue
		}
		existing, err := p.lookupExistingEdge(ctx.Source.ID, ie.ExternalID)
		if err != nil {
			return err
		}
		edge := mapEdge(ctx, ie, srcNode.ID, tgtNode.ID, existing)
		if err := p.archive.SaveEdge(edge); err != nil {
			return fmt.Errorf("ingest: save edge: %w", err)
		}
		if existing != nil {
			p.publish(ctx.RepoID, core.EventEdgeUpdated, core.EventEdgeUpdatePayload{EdgeID: edge.ID, UpdatedEdge: edge})
		} else {
			p.publish(ctx.RepoID, core.EventEdgeCreated, core.EventEdgeCreatePayload{Edge: edge})
		}
	}
	return nil
}

// resolveNode finds a local node by source provenance, returning (nil, nil) when
// absent so callers can skip.
func (p *Pipeline) resolveNode(sourceID uuid.UUID, externalID string) (*core.Node, error) {
	if externalID == "" {
		return nil, nil
	}
	n, err := p.archive.GetNodeBySourceExternalID(sourceID, externalID)
	if err == nil {
		return n, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: resolve node %q: %w", externalID, err)
}

func (p *Pipeline) lookupExistingEdge(sourceID uuid.UUID, externalID string) (*core.Edge, error) {
	if externalID == "" {
		return nil, nil
	}
	e, err := p.archive.GetEdgeBySourceExternalID(sourceID, externalID)
	if err == nil {
		return e, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: dedup edge: %w", err)
}

// mapEdge builds the edge to persist, preserving identity/provenance on update.
func mapEdge(ctx IngestContext, ie core.IngestEdge, src, tgt uuid.UUID, existing *core.Edge) *core.Edge {
	if existing != nil {
		existing.Type, existing.Label, existing.Weight = ie.Type, ie.Label, ie.Weight
		existing.Source, existing.Target = src, tgt
		return existing
	}
	sid := ctx.Source.ID
	ext := ie.ExternalID
	return &core.Edge{
		ID: core.NewID(), RepoID: ctx.RepoID, Type: ie.Type, Source: src, Target: tgt,
		Label: ie.Label, Weight: ie.Weight, CreatedBy: ctx.CreatedBy,
		SourceID: &sid, ExternalID: &ext,
	}
}
```

Confirm `core.EventEdgeCreatePayload`/`EventEdgeUpdatePayload` field names against `core/events.go:74-90` and adjust the struct literals to match.

- [ ] **Step 4: Run edge tests** → PASS.

- [ ] **Step 5: Repeat for Threads and Annotations**

Add `ingestThreads` (run **before** Items so `IngestItem.ThreadExternalID` can attach; find/create by `GetThreadBySourceExternalID`, then `AddNodeToThread` for items carrying `ThreadExternalID`) and `ingestAnnotations` (resolve `TargetExternalID` via `resolveNode`, skip if absent, dedup via `GetAnnotationBySourceExternalID`, persist, emit `AnnotationCreated`/`Updated`). Mirror the edge structure. Add a `TestIngestThread`/`TestIngestAnnotation` and make them pass. Also add the discrete-membership path: an `ApplyThreadMembership(ctx, threadExternalID, nodeExternalID, add bool)` used later by the supervisor for `ThreadNodeAdded`/`Removed` (resolve both via provenance; no-op when unresolved).

- [ ] **Step 6: Extract the shared filter helper**

Move `applySourceFilters`/`nodePayload` (`pipeline.go:173-220`) logic into an exported helper usable by the publish supervisor, e.g.:

```go
// MatchesFilterChain reports whether a node satisfies a filter chain (by id)
// under the given mode. An empty chain matches. Shared by the pipeline's source
// filtering and the publish supervisor's Sink filtering.
func MatchesFilterChain(archive core.Archive, filterIDs []uuid.UUID, mode core.FilterMode, node *core.Node) (bool, error)
```

Re-implement `applySourceFilters` to call it. Keep existing pipeline tests green.

- [ ] **Step 7: Commit**

```bash
git add ingest/pipeline.go ingest/pipeline_test.go
git commit -m "feat(ingest): pipeline processes Threads, Edges, Annotations with provenance"
```

---

## Task 8: Ingest supervisor (`SINK_EVENTS` → pipeline)

**Files:**
- Create: `ingest/supervisor.go`, `ingest/supervisor_test.go`

The supervisor is an `app.Runner` and a durable consumer on `SINK_EVENTS`, structured exactly like `rules.Engine` (`rules/engine.go:46-146`).

- [ ] **Step 1: Write the failing test** — decode a sink `NodeCreated` event, assert it ingests into each subscribing repo with provenance.

```go
//go:build sqlite_fts5

package ingest

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func TestSupervisorIngestsNodeEventToSubscribers(t *testing.T) {
	a := newTestArchive(t)
	repoB := mkRepo(t, a)
	sinkID := uuid.New()
	src := &core.Source{RepoID: repoB.ID, Name: "from-a", Connector: core.ConnectorRepo,
		Config: map[string]any{"sink_id": sinkID.String()}, Status: core.StatusActive, CreatedBy: repoB.OwnerID}
	_ = a.SaveSource(src)

	sup := NewSupervisor(a, nil, nil) // consumer nil: we call handleSinkEvent directly
	evt, _ := core.NewEvent(core.EventNodeCreated, uuid.New(), // repo A id (informational)
		core.EventNodeCreatePayload{Node: &core.Node{ID: uuid.New(), Type: "note", Subject: "hi", ContentType: "text/plain", Body: []byte(`"x"`)}})
	data, _ := json.Marshal(evt)

	if d := sup.handleSinkEvent(sinkID, data); d != ackOK {
		t.Fatalf("expected ackOK, got %v", d)
	}
	// The A node's id becomes the external id in B.
	var p core.EventNodeCreatePayload
	_ = json.Unmarshal(evt.Payload, &p)
	got, err := a.GetNodeBySourceExternalID(src.ID, p.Node.ID.String())
	if err != nil {
		t.Fatalf("node not ingested into subscriber: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != src.ID {
		t.Fatalf("provenance not set")
	}
}
```

- [ ] **Step 2: Run test** → FAIL. `go test -tags sqlite_fts5 ./ingest/ -run TestSupervisorIngests -v`

- [ ] **Step 3: Implement `ingest/supervisor.go`**

```go
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

const (
	supFetchBatchSize = 10
	supFetchTimeout   = 5 * time.Second
)

type ackDecision int

const (
	ackOK ackDecision = iota
	ackRetry
	ackTerm
)

// Supervisor is the ingest side of repo→repo: a durable JetStream pull consumer
// on SINK_EVENTS that maps each curated Sink event into the ingest pipeline for
// every repository whose repo-Source subscribes to that Sink. It is an
// app.Runner, started from main after archive wiring.
type Supervisor struct {
	archive  core.Archive
	pipeline *Pipeline
	consumer app.Consumer
}

// NewSupervisor constructs the ingest supervisor.
func NewSupervisor(archive core.Archive, pub app.Publisher, consumer app.Consumer) *Supervisor {
	return &Supervisor{archive: archive, pipeline: NewPipeline(archive, pub), consumer: consumer}
}

// Run consumes SINK_EVENTS until the context is cancelled.
func (s *Supervisor) Run(ctx context.Context) {
	slog.Info("Ingest supervisor started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("Ingest supervisor stopping")
			return
		default:
		}
		msgs, err := s.consumer.Fetch(supFetchBatchSize, supFetchTimeout)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue
			}
			slog.Error("Ingest supervisor fetch failed", "error", err)
			continue
		}
		for _, msg := range msgs {
			s.process(msg)
		}
	}
}

func (s *Supervisor) process(msg *app.Msg) {
	sinkID, err := sinkIDFromSubject(msg.Subject)
	if err != nil {
		slog.Error("Ingest supervisor: bad sink subject", "subject", msg.Subject, "error", err)
		_ = msg.Term()
		return
	}
	switch s.handleSinkEvent(sinkID, msg.Data) {
	case ackRetry:
		_ = msg.Nak()
	case ackTerm:
		_ = msg.Term()
	default:
		_ = msg.Ack()
	}
}

// handleSinkEvent fans one curated event out to every subscribing repo-Source.
func (s *Supervisor) handleSinkEvent(sinkID uuid.UUID, data []byte) ackDecision {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return ackTerm
	}
	sources, err := s.archive.ListRepoSourcesForSink(sinkID)
	if err != nil {
		slog.Error("Ingest supervisor: list subscribers failed", "sink", sinkID, "error", err)
		return ackRetry // no side effects yet; redeliver
	}
	for i := range sources {
		if err := s.ingestForSource(&sources[i], &evt); err != nil {
			slog.Error("Ingest supervisor: ingest failed", "source", sources[i].ID, "error", err)
			return ackRetry
		}
	}
	return ackOK
}

// ingestForSource maps one event into a single-entity IngestBatch and runs it
// through the pipeline into the source's repository.
func (s *Supervisor) ingestForSource(src *core.Source, evt *core.Event) error {
	owner, err := s.repoOwner(src.RepoID)
	if err != nil {
		return err
	}
	batch, membership, err := batchFromEvent(evt)
	if err != nil || batch == nil {
		return err
	}
	ctx := IngestContext{RepoID: src.RepoID, Source: src, CreatedBy: owner}
	if membership != nil {
		return s.pipeline.ApplyThreadMembership(ctx, membership.thread, membership.node, membership.add)
	}
	_, err = s.pipeline.Ingest(ctx, batch)
	return err
}
```

Add: `sinkIDFromSubject(subject string) (uuid.UUID, error)` (parse `sink.{id}.events.{type}`), `repoOwner(repoID)` (load the repo, return `OwnerID` — copy the approach `jobs/runner.go` uses), and `batchFromEvent(evt)` which switches on `evt.Type`:
- `NodeCreated`/`NodeUpdated` → `IngestBatch{Items: [...]}` with `ExternalID = payload node.ID`, `Node` = content copy (Type/Subject/ContentType/Body/Metadata; **strip** ID/RepoID/ShortID/Version/provenance), `ThreadExternalID` left empty.
- `EdgeCreated`/`EdgeUpdated` → `IngestBatch{Edges: [...]}` with external ids = the payload edge's `Source`/`Target`/`ID` UUIDs as strings.
- `ThreadCreated`/`ThreadUpdated` → `IngestBatch{Threads: [...]}`.
- `AnnotationCreated`/`AnnotationUpdated` → `IngestBatch{Annotations: [...]}`.
- `ThreadNodeAdded`/`ThreadNodeRemoved` → return a `membership` struct `{thread, node string, add bool}` (handled via `ApplyThreadMembership`).
- default → `(nil, nil, nil)` (ignored, acked).

- [ ] **Step 4: Run test** → PASS. Add `TestSupervisorIngestsEdgeEvent` (publish nodes first, then an edge event, assert edge resolved). PASS.

- [ ] **Step 5: Commit**

```bash
git add ingest/supervisor.go ingest/supervisor_test.go
git commit -m "feat(ingest): ingest supervisor maps SINK_EVENTS to the pipeline"
```

---

## Task 9: Publish supervisor (`REPO_EVENTS` → filter → `SINK_EVENTS`)

**Files:**
- Create: `connector/publish_supervisor.go`, `connector/publish_supervisor_test.go`

Structured like `rules.Engine`, but consuming `REPO_EVENTS` via the `sink-publisher` consumer and **publishing** curated events to `SINK_EVENTS`.

- [ ] **Step 1: Write the failing test** — a node passing the Sink filter is republished; one failing it is not.

```go
//go:build sqlite_fts5

package connector

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestPublishSupervisorRepublishesMatchingNode(t *testing.T) {
	a := newTestArchive(t)
	repo := mkRepo(t, a)
	sink := &core.Sink{RepoID: repo.ID, Name: "out", Connector: core.ConnectorRepo,
		Status: core.StatusActive, CreatedBy: repo.OwnerID} // empty FilterIDs → publish everything
	_ = a.SaveSink(sink)

	pub := &capturePublisher{}
	sup := NewPublishSupervisor(a, pub, nil)

	node := &core.Node{ID: core.NewID(), RepoID: repo.ID, Type: "note", Subject: "s", ContentType: "text/plain", Body: []byte(`"x"`)}
	evt, _ := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	data, _ := json.Marshal(evt)

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	if len(pub.subjects) != 1 || pub.subjects[0] != core.SinkSubject(sink.ID, core.EventNodeCreated) {
		t.Fatalf("expected republish on sink subject, got %v", pub.subjects)
	}
}

// capturePublisher records Publish calls.
type capturePublisher struct{ subjects []string }

func (c *capturePublisher) Publish(subject string, data []byte) error {
	c.subjects = append(c.subjects, subject)
	return nil
}
func (c *capturePublisher) PublishEvent(core.EventType, uuid.UUID, any) error { return nil }
```

- [ ] **Step 2: Run test** → FAIL. `go test -tags sqlite_fts5 ./connector/ -run TestPublishSupervisor -v`

- [ ] **Step 3: Implement `connector/publish_supervisor.go`**

Mirror `rules.Engine`'s `Run`/`processMessage` loop. **Define this package's own** `ackDecision` type and `ackOK`/`ackRetry`/`ackTerm` constants at the top of the file (copy `rules/engine.go:28-35` verbatim — they are package-private, so `connector` needs its own copy; do **not** import `ingest`'s). The core method:

```go
// dispatch evaluates one domain event against the producing repo's Sinks and
// republishes the curated subset to SINK_EVENTS.
func (s *PublishSupervisor) dispatch(data []byte) ackDecision {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return ackTerm
	}
	if !isPublishableEntityEvent(evt.Type) {
		return ackOK // lifecycle/meta events never leave the repo
	}

	node, normType, payload, ok, err := s.resolvePublishNode(&evt)
	if err != nil {
		return ackRetry // archive read failed before any publish
	}
	if !ok {
		return ackOK // unresolvable target (e.g. deleted) — nothing to publish
	}

	sinks, _, err := s.archive.ListSinks(evt.RepoID, core.SinkListOptions{
		ListOptions: core.ListOptions{Limit: 1000, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		return ackRetry
	}
	for i := range sinks {
		sk := &sinks[i]
		if sk.Connector != core.ConnectorRepo || sk.Status != core.StatusActive {
			continue
		}
		match, err := ingest.MatchesFilterChain(s.archive, sk.FilterIDs, sk.FilterChainMode, node)
		if err != nil {
			return ackRetry
		}
		if !match {
			continue
		}
		s.republish(sk.ID, normType, payload, evt.RepoID)
	}
	return ackOK
}
```

Where:
- `isPublishableEntityEvent` → true for Node/Edge/Thread/Annotation Created/Updated/Patched + ThreadNodeAdded/Removed.
- `resolvePublishNode(evt)` returns the **node used for filtering** plus the normalised event to republish:
  - Node events: the node is in the payload; `NodePatched` → load the full node from the archive and set `normType = EventNodeUpdated`, payload = `EventNodeUpdatePayload{NodeID, UpdatedNode}`.
  - Edge events: load **both** endpoint nodes; require **both** to pass (caller treats a non-match as skip) — return one node for the chain plus enforce the other explicitly (evaluate both inside `dispatch` for edges; see note below).
  - Annotation events: load the target node.
  - Thread / ThreadNode: load member node(s); for `ThreadCreated`/`Updated` publish iff ≥1 member passes and prune the membership; for `ThreadNode*` filter on that one node.
- `republish(sinkID, eventType, payload, repoID)` builds a `core.Event` (`core.NewEvent(eventType, repoID, payload)`) and calls `s.pub.Publish(core.SinkSubject(sinkID, eventType), marshalled)`. Fire-and-forget; log on error.

> **Edge/thread two-node rule:** because `MatchesFilterChain` takes a single node, evaluate each endpoint and AND the results in `dispatch` for edge events (load both, both must pass). Keep this explicit rather than forcing it through the single-node helper.

`connector` already imports `ingest`? It does not today — adding `connector → ingest` is acyclic (`ingest` imports `app`/`core`/`filter`, not `connector`). Confirm with `go build ./...`.

- [ ] **Step 4: Run test** → PASS. Add `TestPublishSupervisorDropsFilteredNode` (a Sink with a filter that the node fails → no republish) and `TestPublishSupervisorEdgeRequiresBothEndpoints`. PASS.

- [ ] **Step 5: Commit**

```bash
git add connector/publish_supervisor.go connector/publish_supervisor_test.go
git commit -m "feat(connector): publish supervisor enforces Sink publication control"
```

---

## Task 10: Connector config + cycle detection

**Files:**
- Modify: `connector/handlers_source.go` (`~320-340`), `connector/handlers_sink.go` (`~325-340`), `connector/cycle.go`
- Test: `connector/connector_test.go` (update repo cases `906-979`)

- [ ] **Step 1: Update the failing tests first**

In `connector/connector_test.go`, change repo-**Source** fixtures to use `config: {"sink_id": <some sink uuid>}` and repo-**Sink** fixtures to use `config: {}` (drop `repo_id`). Add:

```go
func TestRepoSourceRequiresSinkID(t *testing.T) {
	// creating a repo source without config.sink_id → 400
}
func TestRepoSinkNeedsNoTarget(t *testing.T) {
	// creating a repo sink with empty config → 201
}
func TestCycleDetectedViaSourceSinkRef(t *testing.T) {
	// repoA has Source{sink_id: sinkB(of repoB)}; creating repoB Source{sink_id: sinkA(of repoA)} → 409 cycle
}
```

- [ ] **Step 2: Run tests** → FAIL. `go test -tags sqlite_fts5 ./connector/ -v`

- [ ] **Step 3: Source validation** — in `handlers_source.go`'s repo case, replace the `repo_id` requirement with:

```go
case core.ConnectorRepo:
	ref, ok := config["sink_id"].(string)
	if !ok || ref == "" {
		return errors.New("config.sink_id is required for repo connector")
	}
	sinkID, err := uuid.Parse(ref)
	if err != nil {
		return errors.New("config.sink_id must be a valid UUID")
	}
	if _, err := m.archive.GetSink(sinkID); err != nil {
		return fmt.Errorf("config.sink_id does not resolve to a sink: %w", err)
	}
```

(Adjust to the validator's actual signature/receiver — match the surrounding cases.)

- [ ] **Step 4: Sink validation** — in `handlers_sink.go`'s repo case, **remove** the `repo_id` requirement; repo sinks need no target. Keep `core.ConnectorRepo` in `isSupportedConnector` for sinks. The Sink's curation is its `FilterIDs`, validated by the generic filter-ref check already in place.

- [ ] **Step 5: cycle.go** — rewrite `repoConnectorTargets` to walk **Source → sink → repo** and drop the sink loop:

```go
// repoConnectorTargets returns the repo IDs that repo-type Sources in the given
// repository subscribe to, resolved via each Source's referenced Sink.
func (m *Module) repoConnectorTargets(repoID uuid.UUID) ([]uuid.UUID, error) {
	sources, _, err := m.archive.ListSources(repoID, core.SourceListOptions{
		Connector:   core.ConnectorRepo,
		ListOptions: core.ListOptions{Limit: 1000, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		return nil, err
	}
	var targets []uuid.UUID
	for _, src := range sources {
		ref, _ := src.Config["sink_id"].(string)
		sinkID, err := uuid.Parse(ref)
		if err != nil {
			continue
		}
		sink, err := m.archive.GetSink(sinkID)
		if err != nil {
			continue
		}
		targets = append(targets, sink.RepoID)
	}
	return targets, nil
}
```

Delete `extractRepoID` if now unused. Cycle detection must run on repo-Source create/update (confirm the create path calls `detectCycle(startRepoID=newSource.RepoID, targetRepoID=sink.RepoID)`).

- [ ] **Step 6: Run tests** → PASS. `go build ./...` → clean.

- [ ] **Step 7: Commit**

```bash
git add connector/
git commit -m "feat(connector): repo Source references sink_id; Sink target-agnostic; cycle via source graph"
```

---

## Task 11: `main.go` wiring + end-to-end integration test

**Files:**
- Modify: `main.go` (`~160-235`)
- Test: `connector/repo_repo_e2e_test.go` (create) — drives the two supervisors directly

- [ ] **Step 1: Wire the stream, consumers, and runners in `main.go`**

After `CreateRepoEventsStream` (`main.go:160`):

```go
	if err := sm.CreateSinkEventsStream(); err != nil {
		slog.Error("Error creating SINK_EVENTS stream", "error", err)
		os.Exit(1)
	}
```

After the rules-engine consumer (`main.go:173`):

```go
	if err := cm.CreateSinkPublisherConsumer(); err != nil {
		slog.Error("Error creating sink-publisher consumer", "error", err)
		os.Exit(1)
	}
	if err := cm.CreateRepoIngestConsumer(); err != nil {
		slog.Error("Error creating repo-ingest consumer", "error", err)
		os.Exit(1)
	}
```

Near the other `NewConsumer` calls (`main.go:197`):

```go
	sinkPubConsumer, err := usnats.NewConsumer(bus, usnats.RepoEventsSubject, usnats.ConsumerSinkPublisher)
	// handle err like the others
	repoIngestConsumer, err := usnats.NewConsumer(bus, usnats.SinkEventsSubject, usnats.ConsumerRepoIngest)
	// handle err like the others
```

Add to the `runners` slice (`main.go:213`):

```go
		connector.NewPublishSupervisor(a, bus.Publisher(), sinkPubConsumer),
		ingest.NewSupervisor(a, bus.Publisher(), repoIngestConsumer),
```

Match the exact constructor signatures from Tasks 8–9. `go build ./...` → clean.

- [ ] **Step 2: Write the end-to-end test**

```go
//go:build sqlite_fts5

package connector

// TestRepoToRepoEndToEnd: a node created in repoA, with an active repo Sink and
// a repoB repo-Source subscribing to it, lands in repoB with provenance and
// honours both filters. Drives publish supervisor → capture subject → ingest
// supervisor, asserting the node arrives in B.
func TestRepoToRepoEndToEnd(t *testing.T) {
	// 1. archive: repoA, repoB; sinkA (active, empty filter); sourceB{sink_id: sinkA.ID}
	// 2. publish supervisor.dispatch(NodeCreated in A) → assert it published on SinkSubject(sinkA, NodeCreated)
	// 3. feed that published payload into ingest.Supervisor.handleSinkEvent(sinkA.ID, data)
	// 4. assert a.GetNodeBySourceExternalID(sourceB.ID, aNode.ID.String()) exists with SourceID == sourceB.ID
}
```

Implement it fully using the `capturePublisher` from Task 9 to bridge the two supervisors in-process (no live NATS needed). Assert node content, provenance, and that a node failing the Sink filter never reaches B.

- [ ] **Step 3: Run** → PASS. `go test -tags sqlite_fts5 ./...` → all green.

- [ ] **Step 4: Commit**

```bash
git add main.go connector/repo_repo_e2e_test.go
git commit -m "feat: wire repo→repo publish + ingest supervisors; end-to-end test"
```

---

## Final verification

- [ ] `go build ./...` → clean
- [ ] `go test -tags sqlite_fts5 ./...` → all packages pass
- [ ] `git log --oneline` shows ~11 focused commits
- [ ] Re-read the spec §3/§5/§8; confirm every propagated event type (Node/Edge/Thread/Annotation Created/Updated, NodePatched-normalised, ThreadNode membership) has a handling path, and deletes/tombstones are absent (deferred).

---

## Notes for the implementer

- **Provenance is the spine.** Every relational entity is re-resolved through node provenance in the destination repo. If a node was filtered out, its relations are skipped — that is correct, not a bug.
- **`cycle.go` is load-bearing.** It is the *only* loop bound; provenance dedup gives idempotency but cannot stop a topological cycle. Never weaken the create-time cycle check.
- **NATS isolation.** Supervisors touch NATS only through `app.Publisher`/`app.Consumer`. Never import `nats-io` outside `nats/`.
- **FTS5 tag.** Archive paths need `-tags sqlite_fts5`; a bare `go test ./...` silently skips search and may mis-build archive tests.
- **Trust the compiler, not the LSP** — this repo's LSP reports phantom errors.
