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
