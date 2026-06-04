package rules

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

// mockPublisher is a no-op publisher that records published messages.
type mockPublisher struct {
	published []publishedMsg
}

type publishedMsg struct {
	Subject string
	Data    []byte
}

func (p *mockPublisher) Publish(subject string, data []byte) error {
	p.published = append(p.published, publishedMsg{Subject: subject, Data: data})
	return nil
}

// setupTestModule creates a rules Module wired to a temporary archive.
func setupTestModule(t *testing.T) (*Module, *mockPublisher) {
	t.Helper()
	dir := t.TempDir()
	a, err := archive.NewLocalArchive(dir)
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}
	t.Cleanup(func() { a.Close() })

	pub := &mockPublisher{}
	m := &Module{}
	m.Init(nil)
	m.SetArchive(a)
	m.SetPublisher(pub)
	return m, pub
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

// createTestFilterForRepo creates a filter in the repo for trigger-reference tests.
func createTestFilterForRepo(t *testing.T, m *Module, repo *core.Repository, field, value string) *core.Filter {
	t.Helper()
	f := &core.Filter{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Name:   "test filter",
		Conditions: []core.Condition{
			{Field: field, Op: core.OpEq, Value: json.RawMessage(`"` + value + `"`)},
		},
		Mode:      core.FilterModeAll,
		CreatedBy: defaultOwnerID,
	}
	if err := m.archive.SaveFilter(f); err != nil {
		t.Fatalf("Failed to create test filter: %v", err)
	}
	return f
}

func doRequest(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequestWithHeaders(t, mux, method, path, body, nil)
}

func doRequestWithHeaders(t *testing.T, mux *http.ServeMux, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

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

// validRuleBody returns a minimal valid create-rule request body.
func validRuleBody() map[string]any {
	return map[string]any{
		"name":    "notify",
		"trigger": map[string]any{"event": "NodeCreated"},
		"actions": []map[string]any{
			{"type": "webhook", "params": map[string]any{"url": "https://example.test"}},
		},
	}
}

func TestCreateRule_Success(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var rule core.Rule
	parseResponseData(t, w, &rule)
	if rule.ShortID != "RULE-1" {
		t.Errorf("Expected short ID RULE-1, got %q", rule.ShortID)
	}
	if rule.Version != 1 {
		t.Errorf("Expected version 1, got %d", rule.Version)
	}
	if rule.Status != core.StatusActive {
		t.Errorf("Expected status active, got %q", rule.Status)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("Expected ETag header")
	}
}

func TestCreateRule_MissingName(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	body := validRuleBody()
	delete(body, "name")
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

func TestCreateRule_NoActions(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	body := validRuleBody()
	body["actions"] = []map[string]any{}
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

func TestCreateRule_InvalidActionType(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	body := validRuleBody()
	body["actions"] = []map[string]any{{"type": "explode", "params": map[string]any{}}}
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

func TestCreateRule_InvalidFilterRef(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	body := validRuleBody()
	body["trigger"] = map[string]any{"event": "NodeCreated", "filter_ids": []string{core.NewID().String()}}
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "invalid_filter_ref" {
		t.Errorf("Expected invalid_filter_ref, got %q", code)
	}
}

func TestCreateRule_ValidFilterRef(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	f := createTestFilterForRepo(t, m, repo, "node.type", "article")
	body := validRuleBody()
	body["trigger"] = map[string]any{"event": "NodeCreated", "filter_ids": []string{f.ID.String()}}
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetRule_NotFound(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/rules/RULE-999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRules_StatusFilter(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	// Create an active rule and a paused rule.
	doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	pausedBody := validRuleBody()
	pausedBody["status"] = "paused"
	doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", pausedBody)

	w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/rules?status=active", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data []core.Rule `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if envelope.Meta.Total != 1 || len(envelope.Data) != 1 {
		t.Fatalf("Expected 1 active rule, got %d (total %d)", len(envelope.Data), envelope.Meta.Total)
	}
	if envelope.Data[0].Status != core.StatusActive {
		t.Errorf("Expected active rule, got %q", envelope.Data[0].Status)
	}
}

func TestUpdateRule_Success(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	var created core.Rule
	parseResponseData(t, w, &created)

	upd := map[string]any{"name": "renamed"}
	w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID, upd, map[string]string{"If-Match": `"1"`})
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated core.Rule
	parseResponseData(t, w, &updated)
	if updated.Name != "renamed" {
		t.Errorf("Expected name 'renamed', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Errorf("Expected version 2, got %d", updated.Version)
	}
}

func TestUpdateRule_MissingIfMatch(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	var created core.Rule
	parseResponseData(t, w, &created)

	w = doRequest(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID, map[string]any{"name": "x"})
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("Expected 428, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "if_match_required" {
		t.Errorf("Expected if_match_required, got %q", code)
	}
}

func TestPauseAndResumeRule(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	var created core.Rule
	parseResponseData(t, w, &created)

	w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID+"/pause", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on pause, got %d: %s", w.Code, w.Body.String())
	}
	var paused core.Rule
	parseResponseData(t, w, &paused)
	if paused.Status != core.StatusPaused {
		t.Errorf("Expected paused, got %q", paused.Status)
	}

	w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID+"/resume", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on resume, got %d: %s", w.Code, w.Body.String())
	}
	var resumed core.Rule
	parseResponseData(t, w, &resumed)
	if resumed.Status != core.StatusActive {
		t.Errorf("Expected active, got %q", resumed.Status)
	}
}

func TestDeleteRule(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", validRuleBody())
	var created core.Rule
	parseResponseData(t, w, &created)

	w = doRequest(t, mux, "DELETE", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/rules/"+created.ShortID, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 after delete, got %d", w.Code)
	}
}
