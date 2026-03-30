package repo

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

// setupTestModule creates a repo Module wired to a temporary archive.
func setupTestModule(t *testing.T) *Module {
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
	return m
}

// createTestRepo creates a repository via the archive and returns it.
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

// doRequest executes an HTTP request against a handler and returns the response.
func doRequest(t *testing.T, handler http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
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
	handler.ServeHTTP(w, req)
	return w
}

// parseResponseData extracts the "data" field from the response envelope.
func parseResponseData(t *testing.T, w *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v (body: %s)", err, w.Body.String())
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("Failed to parse data: %v", err)
	}
}

func TestCreateAndGetRepo(t *testing.T) {
	m := setupTestModule(t)

	// Create repo.
	body := map[string]string{"slug": "research", "name": "Research", "description": "AI research"}
	w := doRequest(t, m.createRepoHandler(), "POST", "/repos", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var repo core.Repository
	parseResponseData(t, w, &repo)
	if repo.Slug != "research" {
		t.Errorf("Expected slug 'research', got %q", repo.Slug)
	}
	if repo.ShortID != "REPO-1" {
		t.Errorf("Expected short ID REPO-1, got %q", repo.ShortID)
	}
	if repo.Version != 1 {
		t.Errorf("Expected version 1, got %d", repo.Version)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("Expected ETag header")
	}
}

func TestCreateNode(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Need to set path value for the handler. Use a mux.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{repo_ref}/nodes", m.createNodeHandler())

	body := map[string]any{
		"type":         "article",
		"subject":      "Test Article",
		"content_type": "text/markdown",
		"body":         "# Hello",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/repos/"+repo.Slug+"/nodes", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var node core.Node
	parseResponseData(t, w, &node)
	if node.ShortID != "NODE-1" {
		t.Errorf("Expected short ID NODE-1, got %q", node.ShortID)
	}
	if node.Type != "article" {
		t.Errorf("Expected type 'article', got %q", node.Type)
	}
	if node.Version != 1 {
		t.Errorf("Expected version 1, got %d", node.Version)
	}
}

func TestCreateEdge(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Create two nodes first.
	node1 := &core.Node{ID: core.NewID(), RepoID: repo.ID, Type: "a", Subject: "A", ContentType: "text/plain", CreatedBy: defaultOwnerID}
	node2 := &core.Node{ID: core.NewID(), RepoID: repo.ID, Type: "b", Subject: "B", ContentType: "text/plain", CreatedBy: defaultOwnerID}
	m.archive.SaveNode(node1)
	m.archive.SaveNode(node2)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{repo_ref}/edges", m.createEdgeHandler())

	body := map[string]any{
		"type":   "reply",
		"source": node1.ShortID,
		"target": node2.ShortID,
		"label":  "replies to",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/repos/"+repo.Slug+"/edges", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var edge core.Edge
	parseResponseData(t, w, &edge)
	if edge.ShortID != "EDGE-1" {
		t.Errorf("Expected short ID EDGE-1, got %q", edge.ShortID)
	}
}

func TestCreateThread(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{repo_ref}/threads", m.createThreadHandler())

	body := map[string]any{
		"node": map[string]any{
			"type":         "collection",
			"subject":      "Thread Subject",
			"content_type": "text/plain",
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/repos/"+repo.Slug+"/threads", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var thread core.Thread
	parseResponseData(t, w, &thread)
	if thread.ShortID != "THREAD-1" {
		t.Errorf("Expected short ID THREAD-1, got %q", thread.ShortID)
	}
	if thread.Node.ShortID != "NODE-1" {
		t.Errorf("Expected root node short ID NODE-1, got %q", thread.Node.ShortID)
	}
}

func TestCreateAnnotation(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Create target node.
	target := &core.Node{ID: core.NewID(), RepoID: repo.ID, Type: "article", Subject: "Target", ContentType: "text/plain", CreatedBy: defaultOwnerID}
	m.archive.SaveNode(target)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{repo_ref}/annotations", m.createAnnotationHandler())

	body := map[string]any{
		"target_node_id": target.ShortID,
		"motivation":     "commenting",
		"node": map[string]any{
			"type":    "comment",
			"subject": "My Comment",
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/repos/"+repo.Slug+"/annotations", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var anno core.Annotation
	parseResponseData(t, w, &anno)
	if anno.ShortID != "ANNO-1" {
		t.Errorf("Expected short ID ANNO-1, got %q", anno.ShortID)
	}
	if anno.Motivation != "commenting" {
		t.Errorf("Expected motivation 'commenting', got %q", anno.Motivation)
	}
}

func TestFlatURLEntityAccess(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Create a node directly.
	node := &core.Node{
		ID: core.NewID(), RepoID: repo.ID, Type: "article",
		Subject: "Test Article", ContentType: "text/plain",
		Body: json.RawMessage(`"hello"`), CreatedBy: defaultOwnerID,
	}
	m.archive.SaveNode(node)

	// Access via flat URL with short ID.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{repo_ref}/{entity_ref}", m.entityHandler())

	req := httptest.NewRequest("GET", "/repos/"+repo.Slug+"/"+node.ShortID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got core.Node
	parseResponseData(t, w, &got)
	if got.ID != node.ID {
		t.Errorf("Expected node ID %s, got %s", node.ID, got.ID)
	}

	// Access via UUID should also work.
	req2 := httptest.NewRequest("GET", "/repos/"+repo.Slug+"/"+node.ID.String(), nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200 for UUID access, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestDeleteEntityViaFlatURL(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	node := &core.Node{
		ID: core.NewID(), RepoID: repo.ID, Type: "note",
		Subject: "Delete Me", ContentType: "text/plain", CreatedBy: defaultOwnerID,
	}
	m.archive.SaveNode(node)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /repos/{repo_ref}/{entity_ref}", m.entityHandler())
	mux.HandleFunc("GET /repos/{repo_ref}/{entity_ref}", m.entityHandler())

	// Delete.
	req := httptest.NewRequest("DELETE", "/repos/"+repo.Slug+"/"+node.ShortID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gone.
	req2 := httptest.NewRequest("GET", "/repos/"+repo.Slug+"/"+node.ShortID, nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("Expected 404 after delete, got %d", w2.Code)
	}
}

func TestMetadataMerge(t *testing.T) {
	existing := []core.Metadata{
		{Key: "a", Value: json.RawMessage(`"1"`)},
		{Key: "b", Value: json.RawMessage(`"2"`)},
	}

	patch := []core.Metadata{
		{Key: "b", Value: json.RawMessage(`"updated"`)}, // Update existing.
		{Key: "c", Value: json.RawMessage(`"3"`)},       // Add new.
		{Key: "a", Value: nil},                           // Delete.
	}

	result := mergeMetadata(existing, patch)

	if len(result) != 2 {
		t.Fatalf("Expected 2 metadata entries, got %d", len(result))
	}

	m := make(map[string]string)
	for _, md := range result {
		m[md.Key] = string(md.Value)
	}

	if m["b"] != `"updated"` {
		t.Errorf("Expected b='updated', got %s", m["b"])
	}
	if m["c"] != `"3"` {
		t.Errorf("Expected c='3', got %s", m["c"])
	}
	if _, ok := m["a"]; ok {
		t.Error("Expected key 'a' to be deleted")
	}
}
