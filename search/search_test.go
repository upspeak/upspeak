package search

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/upspeak/upspeak/archive"
	"github.com/upspeak/upspeak/core"
)

// setupTestModule creates a search Module wired to a temporary archive and
// returns both the module and the concrete archive (for fixtures and the FTS
// availability check).
func setupTestModule(t *testing.T) (*Module, *archive.LocalArchive) {
	t.Helper()
	dir := t.TempDir()
	a, err := archive.NewLocalArchive(dir)
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}
	t.Cleanup(func() { a.Close() })

	m := &Module{}
	m.Init(nil)
	m.SetArchive(a)
	return m, a
}

func createTestRepo(t *testing.T, m *Module) *core.Repository {
	t.Helper()
	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    "test-repo",
		Name:    "Test Repo",
		OwnerID: defaultOwnerID,
	}
	if err := m.archive.SaveRepository(repo); err != nil {
		t.Fatalf("Failed to create test repo: %v", err)
	}
	return repo
}

// makeNode saves a text node so that its subject and body are searchable.
func makeNode(t *testing.T, m *Module, repo *core.Repository, nodeType, subject, body string) *core.Node {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        nodeType,
		Subject:     subject,
		ContentType: "text/plain",
		Body:        bodyJSON,
		CreatedBy:   defaultOwnerID,
	}
	if err := m.archive.SaveNode(node); err != nil {
		t.Fatalf("Failed to save node: %v", err)
	}
	return node
}

func makeEdge(t *testing.T, m *Module, repo *core.Repository, edgeType string, source, target *core.Node) {
	t.Helper()
	edge := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      edgeType,
		Source:    source.ID,
		Target:    target.ID,
		CreatedBy: defaultOwnerID,
	}
	if err := m.archive.SaveEdge(edge); err != nil {
		t.Fatalf("Failed to save edge: %v", err)
	}
}

func doRequest(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func parseErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse error envelope: %v (body: %s)", err, w.Body.String())
	}
	return envelope.Error.Code
}

func buildMux(m *Module) *http.ServeMux {
	mux := http.NewServeMux()
	for _, h := range m.HTTPHandlers() {
		mux.HandleFunc(h.Method+" /api/v1"+h.Path, h.Handler)
	}
	return mux
}

func TestSearch_FindsNode(t *testing.T) {
	m, a := setupTestModule(t)
	if !a.FTSAvailable() {
		t.Skip("FTS5 not available; rebuild with -tags sqlite_fts5")
	}
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	makeNode(t, m, repo, "article", "EU AI Act passed", "the body discusses governance")
	makeNode(t, m, repo, "note", "unrelated grocery list", "milk and eggs")

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/search", map[string]any{"query": "AI Act"})
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data []core.SearchResult `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if envelope.Meta.Total != 1 || len(envelope.Data) != 1 {
		t.Fatalf("Expected 1 result, got %d (total %d)", len(envelope.Data), envelope.Meta.Total)
	}
	if envelope.Data[0].Node.Subject != "EU AI Act passed" {
		t.Errorf("Unexpected matched node: %q", envelope.Data[0].Node.Subject)
	}
	if len(envelope.Data[0].Highlights) == 0 {
		t.Error("Expected highlights in search result")
	}
}

func TestSearch_MissingQuery(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/search", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

func TestSearch_RepoNotFound(t *testing.T) {
	m, _ := setupTestModule(t)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/does-not-exist/search", map[string]any{"query": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBrowse_ReturnsNodes(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	makeNode(t, m, repo, "article", "first", "a")
	makeNode(t, m, repo, "note", "second", "b")

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/browse", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data []core.Node `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if envelope.Meta.Total != 2 {
		t.Errorf("Expected total 2, got %d", envelope.Meta.Total)
	}
}

func TestBrowse_TypeFilter(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	makeNode(t, m, repo, "article", "an article", "a")
	makeNode(t, m, repo, "note", "a note", "b")

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/browse?type=article", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data []core.Node `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if envelope.Meta.Total != 1 || len(envelope.Data) != 1 {
		t.Fatalf("Expected 1 article, got %d (total %d)", len(envelope.Data), envelope.Meta.Total)
	}
	if envelope.Data[0].Type != "article" {
		t.Errorf("Expected article, got %q", envelope.Data[0].Type)
	}
}

func TestGraph_Traversal(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	a := makeNode(t, m, repo, "article", "root", "r")
	b := makeNode(t, m, repo, "article", "child", "c")
	c := makeNode(t, m, repo, "article", "grandchild", "g")
	makeEdge(t, m, repo, "cite", a, b)
	makeEdge(t, m, repo, "cite", b, c)

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/graph?node_id="+a.ShortID+"&depth=2&direction=outgoing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data core.GraphResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if envelope.Data.Root == nil || envelope.Data.Root.ID != a.ID {
		t.Fatalf("Expected root node %s, got %+v", a.ID, envelope.Data.Root)
	}
	if len(envelope.Data.Nodes) != 2 {
		t.Errorf("Expected 2 reachable nodes at depth 2, got %d", len(envelope.Data.Nodes))
	}
}

func TestGraph_MissingNodeID(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/graph", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

func TestGraph_NodeNotFound(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/graph?node_id=NODE-999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
