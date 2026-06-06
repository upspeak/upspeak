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

	nodes, _, err := a.ListNodes(repo.ID, core.NodeListOptions{ListOptions: core.DefaultListOptions()})
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
	if got.SourceID == nil || *got.SourceID != src.ID {
		t.Fatal("provenance SourceID not preserved after update")
	}
	if got.ExternalID == nil || *got.ExternalID != "ext-1" {
		t.Fatal("provenance ExternalID not preserved after update")
	}
	nodes, _, _ := a.ListNodes(repo.ID, core.NodeListOptions{ListOptions: core.DefaultListOptions()})
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

func TestPipeline_NilBatch(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	res, err := p.Ingest(IngestContext{RepoID: repo.ID, CreatedBy: repo.OwnerID}, nil)
	if err != nil {
		t.Fatalf("nil batch should not error: %v", err)
	}
	if res.Created != 0 || res.Updated != 0 || res.Skipped != 0 {
		t.Fatalf("nil batch should produce zero result, got %+v", res)
	}
}

// fakePublisher records the event types published, to verify the pipeline emits
// domain events on the canonical write path. The pipeline publishes via Publish
// (hop-stamping path), so the event type is recovered from the marshalled event.
type fakePublisher struct{ events []core.EventType }

func (f *fakePublisher) Publish(_ string, data []byte) error {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return err
	}
	f.events = append(f.events, evt.Type)
	return nil
}

func (f *fakePublisher) PublishEvent(core.EventType, uuid.UUID, any) error { return nil }

func TestPipeline_PublishesEvents(t *testing.T) {
	a := newTestArchive(t)
	repo := newTestRepo(t, a)
	fake := &fakePublisher{}
	p := NewPipeline(a, fake)

	src := &core.Source{
		ID: core.NewID(), RepoID: repo.ID, Name: "s",
		Connector: core.ConnectorWebhook, CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}
	mk := func(body string) *core.IngestBatch {
		return &core.IngestBatch{Items: []core.IngestItem{{
			ExternalID: "e1",
			Node:       &core.Node{Type: "post", Subject: "s", ContentType: "text/plain", Body: textBody(body)},
		}}}
	}

	if _, err := p.Ingest(ctx, mk("v1")); err != nil {
		t.Fatalf("ingest create: %v", err)
	}
	if len(fake.events) != 1 || fake.events[0] != core.EventNodeCreated {
		t.Fatalf("expected one NodeCreated event, got %v", fake.events)
	}

	if _, err := p.Ingest(ctx, mk("v2")); err != nil {
		t.Fatalf("ingest update: %v", err)
	}
	if len(fake.events) != 2 || fake.events[1] != core.EventNodeUpdated {
		t.Fatalf("expected a NodeUpdated event second, got %v", fake.events)
	}
}

// capturePublisher records the core.Events published via Publish (the pipeline's
// hop-stamping path), so tests can assert event subject and Hops. PublishEvent is
// a no-op because the pipeline never uses it.
type capturePublisher struct{ events []core.Event }

func (c *capturePublisher) Publish(subject string, data []byte) error {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return err
	}
	c.events = append(c.events, evt)
	return nil
}

func (c *capturePublisher) PublishEvent(core.EventType, uuid.UUID, any) error { return nil }

func TestIngestPropagatesHops(t *testing.T) {
	a := newTestArchive(t)
	repo := newTestRepo(t, a)
	cap := &capturePublisher{}
	p := NewPipeline(a, cap)
	src := newTestSource(t, a, repo)

	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID, InboundHops: 2}
	batch := &core.IngestBatch{Items: []core.IngestItem{nodeItem("e1", "body")}}
	if _, err := p.Ingest(ctx, batch); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if len(cap.events) != 1 {
		t.Fatalf("expected one event, got %d", len(cap.events))
	}
	evt := cap.events[0]
	if evt.Type != core.EventNodeCreated {
		t.Fatalf("expected NodeCreated, got %s", evt.Type)
	}
	if evt.Hops != 3 {
		t.Fatalf("expected Hops=3 (inbound 2 + 1), got %d", evt.Hops)
	}
	if evt.Subject() != "repo."+repo.ID.String()+".events."+string(core.EventNodeCreated) {
		t.Fatalf("unexpected subject: %s", evt.Subject())
	}
}

// newTestSource persists a webhook source for provenance-based ingestion.
func newTestSource(t *testing.T, a *archive.LocalArchive, repo *core.Repository) *core.Source {
	t.Helper()
	src := &core.Source{
		ID: core.NewID(), RepoID: repo.ID, Name: "s",
		Connector: core.ConnectorWebhook, CreatedBy: repo.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	return src
}

// nodeItem builds a source IngestItem whose node carries the given external ID.
func nodeItem(ext, body string) core.IngestItem {
	return core.IngestItem{
		ExternalID: ext,
		Node:       &core.Node{Type: "post", Subject: ext, ContentType: "text/plain", Body: textBody(body)},
	}
}

func TestIngestResolvesEdgeByProvenance(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Persist two nodes the edge will reference.
	if _, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{
		nodeItem("n-src", "source body"),
		nodeItem("n-tgt", "target body"),
	}}); err != nil {
		t.Fatalf("seed nodes: %v", err)
	}

	res, err := p.Ingest(ctx, &core.IngestBatch{Edges: []core.IngestEdge{{
		ExternalID:       "e-1",
		SourceExternalID: "n-src",
		TargetExternalID: "n-tgt",
		Type:             "reply",
		Label:            "in-reply-to",
		Weight:           2.0,
	}}})
	if err != nil {
		t.Fatalf("Ingest edges: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected one edge created, got %+v", res)
	}

	srcNode, err := a.GetNodeBySourceExternalID(src.ID, "n-src")
	if err != nil {
		t.Fatalf("resolve source node: %v", err)
	}
	tgtNode, err := a.GetNodeBySourceExternalID(src.ID, "n-tgt")
	if err != nil {
		t.Fatalf("resolve target node: %v", err)
	}

	edge, err := a.GetEdgeBySourceExternalID(src.ID, "e-1")
	if err != nil {
		t.Fatalf("resolve edge: %v", err)
	}
	if edge.Source != srcNode.ID || edge.Target != tgtNode.ID {
		t.Fatalf("edge endpoints not resolved: src=%s tgt=%s", edge.Source, edge.Target)
	}
	if edge.Type != "reply" || edge.Label != "in-reply-to" || edge.Weight != 2.0 {
		t.Fatalf("edge fields not mapped: %+v", edge)
	}
	if edge.SourceID == nil || *edge.SourceID != src.ID {
		t.Fatal("edge provenance SourceID missing")
	}
	if edge.ExternalID == nil || *edge.ExternalID != "e-1" {
		t.Fatal("edge provenance ExternalID missing")
	}

	// Re-ingest: should update, not duplicate.
	res2, err := p.Ingest(ctx, &core.IngestBatch{Edges: []core.IngestEdge{{
		ExternalID: "e-1", SourceExternalID: "n-src", TargetExternalID: "n-tgt",
		Type: "reply", Label: "updated", Weight: 3.0,
	}}})
	if err != nil {
		t.Fatalf("re-ingest edge: %v", err)
	}
	if res2.Updated != 1 || res2.Created != 0 {
		t.Fatalf("edge re-ingest should update, got %+v", res2)
	}
	edge2, err := a.GetEdgeBySourceExternalID(src.ID, "e-1")
	if err != nil {
		t.Fatalf("resolve edge after update: %v", err)
	}
	if edge2.Label != "updated" || edge2.Weight != 3.0 {
		t.Fatalf("edge not updated in place: %+v", edge2)
	}
}

func TestIngestDefersEdgeWithMissingEndpoint(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Only the source node exists; target is absent.
	if _, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{
		nodeItem("n-src", "source body"),
	}}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	res, err := p.Ingest(ctx, &core.IngestBatch{Edges: []core.IngestEdge{{
		ExternalID: "e-1", SourceExternalID: "n-src", TargetExternalID: "n-missing", Type: "reply",
	}}})
	if err != nil {
		t.Fatalf("Ingest edges: %v", err)
	}
	if res.Created != 0 || res.Deferred != 1 || res.Skipped != 0 {
		t.Fatalf("expected edge deferred, got %+v", res)
	}
	if _, err := a.GetEdgeBySourceExternalID(src.ID, "e-1"); err == nil {
		t.Fatal("edge with missing endpoint must not be persisted")
	}
}

func TestIngestThread(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	res, err := p.Ingest(ctx, &core.IngestBatch{Threads: []core.IngestThread{{
		ExternalThreadID: "topic-1",
		Subject:          "A topic",
		Metadata:         []core.Metadata{{Key: "k", Value: textBody("v")}},
	}}})
	if err != nil {
		t.Fatalf("Ingest thread: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected one thread created, got %+v", res)
	}

	thread, err := a.GetThreadBySourceExternalID(src.ID, "topic-1")
	if err != nil {
		t.Fatalf("resolve thread: %v", err)
	}
	if thread.Node.Subject != "A topic" || thread.Node.Type != "thread" {
		t.Fatalf("thread root node not built correctly: %+v", thread.Node)
	}
	if thread.SourceID == nil || *thread.SourceID != src.ID {
		t.Fatal("thread provenance SourceID missing")
	}
	if thread.ExternalID == nil || *thread.ExternalID != "topic-1" {
		t.Fatal("thread provenance ExternalID missing")
	}

	// Re-ingest the same thread: should update, not duplicate.
	res2, err := p.Ingest(ctx, &core.IngestBatch{Threads: []core.IngestThread{{
		ExternalThreadID: "topic-1", Subject: "A topic",
	}}})
	if err != nil {
		t.Fatalf("re-ingest thread: %v", err)
	}
	if res2.Updated != 1 || res2.Created != 0 {
		t.Fatalf("thread re-ingest should update, got %+v", res2)
	}
	threads, total, err := a.ListThreads(repo.ID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if total != 1 || len(threads) != 1 {
		t.Fatalf("thread dedup failed: total=%d len=%d", total, len(threads))
	}
}

func TestIngestItemAttachesToThread(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Thread first, then an item referencing it.
	if _, err := p.Ingest(ctx, &core.IngestBatch{Threads: []core.IngestThread{{
		ExternalThreadID: "topic-1", Subject: "A topic",
	}}}); err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	item := nodeItem("msg-1", "first reply")
	item.ThreadExternalID = "topic-1"
	if _, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{item}}); err != nil {
		t.Fatalf("ingest item: %v", err)
	}

	thread, err := a.GetThreadBySourceExternalID(src.ID, "topic-1")
	if err != nil {
		t.Fatalf("resolve thread: %v", err)
	}
	node, err := a.GetNodeBySourceExternalID(src.ID, "msg-1")
	if err != nil {
		t.Fatalf("resolve node: %v", err)
	}
	// Membership manifests as a thread edge from the root node to the member node.
	full, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if !threadHasMember(full, node.ID) {
		t.Fatalf("item not attached to thread: edges=%+v", full.Edges)
	}
}

func TestIngestDefersItemWithMissingThread(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Item references a thread that has not been ingested.
	item := nodeItem("msg-orphan", "reply without a thread")
	item.ThreadExternalID = "topic-absent"
	res, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{item}})
	if err != nil {
		t.Fatalf("ingest item: %v", err)
	}
	// The node itself persists; only the attachment is deferred (retryable).
	if res.Created != 1 || res.Deferred != 1 || res.Skipped != 0 {
		t.Fatalf("expected node created and attachment deferred, got %+v", res)
	}
	if _, err := a.GetNodeBySourceExternalID(src.ID, "msg-orphan"); err != nil {
		t.Fatalf("node should persist even when thread attachment is deferred: %v", err)
	}
}

func TestIngestAnnotation(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Seed the target node.
	if _, err := p.Ingest(ctx, &core.IngestBatch{Items: []core.IngestItem{
		nodeItem("n-tgt", "target body"),
	}}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	res, err := p.Ingest(ctx, &core.IngestBatch{Annotations: []core.IngestAnnotation{{
		ExternalID:       "a-1",
		TargetExternalID: "n-tgt",
		Motivation:       "liking",
		Body:             textBody("👍"),
	}}})
	if err != nil {
		t.Fatalf("Ingest annotation: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected one annotation created, got %+v", res)
	}

	tgt, err := a.GetNodeBySourceExternalID(src.ID, "n-tgt")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	anno, err := a.GetAnnotationBySourceExternalID(src.ID, "a-1")
	if err != nil {
		t.Fatalf("resolve annotation: %v", err)
	}
	if anno.Edge.Target != tgt.ID {
		t.Fatalf("annotation edge target not resolved: got %s want %s", anno.Edge.Target, tgt.ID)
	}
	if anno.Edge.Type != "annotates" || anno.Motivation != "liking" {
		t.Fatalf("annotation fields not mapped: %+v", anno)
	}
	if anno.SourceID == nil || *anno.SourceID != src.ID {
		t.Fatal("annotation provenance SourceID missing")
	}
	if anno.ExternalID == nil || *anno.ExternalID != "a-1" {
		t.Fatal("annotation provenance ExternalID missing")
	}

	// Re-ingest with a changed motivation and body: should update both, not
	// duplicate.
	res2, err := p.Ingest(ctx, &core.IngestBatch{Annotations: []core.IngestAnnotation{{
		ExternalID: "a-1", TargetExternalID: "n-tgt", Motivation: "disliking",
		Body: textBody("👎"),
	}}})
	if err != nil {
		t.Fatalf("re-ingest annotation: %v", err)
	}
	if res2.Updated != 1 || res2.Created != 0 {
		t.Fatalf("annotation re-ingest should update, got %+v", res2)
	}
	anno2, err := a.GetAnnotationBySourceExternalID(src.ID, "a-1")
	if err != nil {
		t.Fatalf("resolve annotation after update: %v", err)
	}
	if anno2.Motivation != "disliking" {
		t.Fatalf("annotation motivation not updated: %s", anno2.Motivation)
	}
	if string(anno2.Node.Body) != string(textBody("👎")) {
		t.Fatalf("annotation body not updated: %s", anno2.Node.Body)
	}
}

func TestIngestDefersAnnotationWithMissingTarget(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	res, err := p.Ingest(ctx, &core.IngestBatch{Annotations: []core.IngestAnnotation{{
		ExternalID: "a-1", TargetExternalID: "n-missing", Motivation: "liking",
	}}})
	if err != nil {
		t.Fatalf("Ingest annotation: %v", err)
	}
	if res.Created != 0 || res.Deferred != 1 || res.Skipped != 0 {
		t.Fatalf("expected annotation deferred, got %+v", res)
	}
	if _, err := a.GetAnnotationBySourceExternalID(src.ID, "a-1"); err == nil {
		t.Fatal("annotation with missing target must not be persisted")
	}
}

func TestMatchesFilterChain(t *testing.T) {
	_, a := setupPipeline(t)
	repo := newTestRepo(t, a)

	cond := core.Condition{Field: "type", Op: core.OpEq, Value: json.RawMessage(strconv.Quote("keep"))}
	f := &core.Filter{ID: core.NewID(), RepoID: repo.ID, Name: "keep-only",
		Mode: core.FilterModeAll, Conditions: []core.Condition{cond}, CreatedBy: repo.OwnerID}
	if err := a.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	keep := &core.Node{Type: "keep", ContentType: "text/plain"}
	drop := &core.Node{Type: "drop", ContentType: "text/plain"}

	// Empty chain always matches.
	ok, err := MatchesFilterChain(a, nil, core.FilterModeAll, drop)
	if err != nil || !ok {
		t.Fatalf("empty chain should match: ok=%v err=%v", ok, err)
	}

	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID}, core.FilterModeAll, keep)
	if err != nil || !ok {
		t.Fatalf("matching node should pass: ok=%v err=%v", ok, err)
	}
	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID}, core.FilterModeAll, drop)
	if err != nil || ok {
		t.Fatalf("non-matching node should fail: ok=%v err=%v", ok, err)
	}

	// An unset chain mode defaults to "all" (AND).
	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID}, "", keep)
	if err != nil || !ok {
		t.Fatalf("empty mode should default to all and pass: ok=%v err=%v", ok, err)
	}
	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID}, "", drop)
	if err != nil || ok {
		t.Fatalf("empty mode should default to all and fail: ok=%v err=%v", ok, err)
	}

	// Second filter: type == "other". Under FilterModeAny, a node matching ANY
	// filter passes; one matching NONE fails.
	cond2 := core.Condition{Field: "type", Op: core.OpEq, Value: json.RawMessage(strconv.Quote("other"))}
	f2 := &core.Filter{ID: core.NewID(), RepoID: repo.ID, Name: "other-only",
		Mode: core.FilterModeAll, Conditions: []core.Condition{cond2}, CreatedBy: repo.OwnerID}
	if err := a.SaveFilter(f2); err != nil {
		t.Fatalf("SaveFilter f2: %v", err)
	}

	// keep matches f but not f2: ANY passes.
	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID, f2.ID}, core.FilterModeAny, keep)
	if err != nil || !ok {
		t.Fatalf("FilterModeAny with one match should pass: ok=%v err=%v", ok, err)
	}
	// drop matches neither f nor f2: ANY fails.
	ok, err = MatchesFilterChain(a, []uuid.UUID{f.ID, f2.ID}, core.FilterModeAny, drop)
	if err != nil || ok {
		t.Fatalf("FilterModeAny with no match should fail: ok=%v err=%v", ok, err)
	}
}

func TestApplyThreadMembership(t *testing.T) {
	p, a := setupPipeline(t)
	repo := newTestRepo(t, a)
	src := newTestSource(t, a, repo)
	ctx := IngestContext{RepoID: repo.ID, Source: src, CreatedBy: repo.OwnerID}

	// Seed a thread and a node.
	if _, err := p.Ingest(ctx, &core.IngestBatch{
		Threads: []core.IngestThread{{ExternalThreadID: "topic-1", Subject: "T"}},
		Items:   []core.IngestItem{nodeItem("msg-1", "body")},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Unresolved thread/node: deferred no-op (false), no error.
	applied, err := p.ApplyThreadMembership(ctx, "topic-missing", "msg-1", true)
	if err != nil {
		t.Fatalf("unresolved membership should not error: %v", err)
	}
	if applied {
		t.Fatal("unresolved membership should report applied=false")
	}

	// Add membership.
	applied, err = p.ApplyThreadMembership(ctx, "topic-1", "msg-1", true)
	if err != nil {
		t.Fatalf("add membership: %v", err)
	}
	if !applied {
		t.Fatal("resolved membership should report applied=true")
	}
	thread, err := a.GetThreadBySourceExternalID(src.ID, "topic-1")
	if err != nil {
		t.Fatalf("resolve thread: %v", err)
	}
	node, err := a.GetNodeBySourceExternalID(src.ID, "msg-1")
	if err != nil {
		t.Fatalf("resolve node: %v", err)
	}
	full, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if !threadHasMember(full, node.ID) {
		t.Fatalf("membership not added: edges=%+v", full.Edges)
	}

	// Remove membership.
	applied, err = p.ApplyThreadMembership(ctx, "topic-1", "msg-1", false)
	if err != nil {
		t.Fatalf("remove membership: %v", err)
	}
	if !applied {
		t.Fatal("resolved removal should report applied=true")
	}
	full, err = a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread after remove: %v", err)
	}
	if threadHasMember(full, node.ID) {
		t.Fatalf("membership not removed: edges=%+v", full.Edges)
	}
}

// threadHasMember reports whether the thread has an edge from its root node to
// the given member node.
func threadHasMember(thread *core.Thread, nodeID uuid.UUID) bool {
	for _, e := range thread.Edges {
		if e.Source == thread.Node.ID && e.Target == nodeID {
			return true
		}
	}
	return false
}
