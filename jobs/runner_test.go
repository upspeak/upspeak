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

// newTestArchive builds a real archive via the public constructor.
// The archive package's own test helpers are not importable across packages.
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
	if res["created"] != float64(1) {
		t.Fatalf("expected created=1, got %v", res["created"])
	}
	if res["url"] != srv.URL {
		t.Fatalf("expected redacted url %q, got %v", srv.URL, res["url"])
	}

	nodes, _, err := a.ListNodes(repo.ID, core.NodeListOptions{ListOptions: core.DefaultListOptions()})
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
