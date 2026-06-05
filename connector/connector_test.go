package connector

import (
"bytes"
"encoding/json"
"io"
"net/http"
"net/http/httptest"
"testing"

"github.com/google/uuid"
"github.com/upspeak/upspeak/archive"
"github.com/upspeak/upspeak/core"
)

// mockPublisher is a no-op publisher for testing.
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

// PublishEvent builds a core.Event envelope and records it via Publish so tests
// observe the same subject/data the nats publisher would emit.
func (p *mockPublisher) PublishEvent(eventType core.EventType, repoID uuid.UUID, payload any) error {
evt, err := core.NewEvent(eventType, repoID, payload)
if err != nil {
return err
}
data, err := json.Marshal(evt)
if err != nil {
return err
}
return p.Publish(evt.Subject(), data)
}

// setupTestModule creates a connector Module wired to a temporary archive.
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
m.SetPublisher(&mockPublisher{})
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

// doRequest executes an HTTP request against a mux and returns the response.
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

// doRequestWithHeaders executes an HTTP request with custom headers.
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

// parseErrorResponse extracts the "error" field from the response envelope.
func parseErrorResponse(t *testing.T, w *httptest.ResponseRecorder) (code, message string) {
t.Helper()
var envelope struct {
Error struct {
Code    string `json:"code"`
Message string `json:"message"`
} `json:"error"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse error envelope: %v (body: %s)", err, w.Body.String())
}
return envelope.Error.Code, envelope.Error.Message
}

// buildMux registers all connector HTTP handlers on a ServeMux.
func buildMux(m *Module) *http.ServeMux {
mux := http.NewServeMux()
for _, h := range m.HTTPHandlers() {
mux.HandleFunc(h.Method+" /api/v1"+h.Path, h.Handler)
}
return mux
}

// --- Source CRUD Tests ---

func TestCreateSource_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "My Webhook Source",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

var source core.Source
parseResponseData(t, w, &source)
if source.Name != "My Webhook Source" {
t.Errorf("Expected name 'My Webhook Source', got %q", source.Name)
}
if source.Connector != core.ConnectorWebhook {
t.Errorf("Expected connector 'webhook', got %q", source.Connector)
}
if source.ShortID != "SRC-1" {
t.Errorf("Expected short ID SRC-1, got %q", source.ShortID)
}
if source.Version != 1 {
t.Errorf("Expected version 1, got %d", source.Version)
}
if source.Status != core.StatusActive {
t.Errorf("Expected status 'active', got %q", source.Status)
}
if w.Header().Get("ETag") == "" {
t.Error("Expected ETag header")
}
}

func TestCreateSource_InvalidBody(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

req := httptest.NewRequest("POST", "/api/v1/repos/"+repo.Slug+"/sources", bytes.NewReader([]byte("not json")))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)

if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_body" {
t.Errorf("Expected error code 'invalid_body', got %q", code)
}
}

func TestCreateSource_MissingName(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "validation_failed" {
t.Errorf("Expected error code 'validation_failed', got %q", code)
}
}

func TestCreateSource_UnsupportedConnector(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "RSS Source",
"connector": "rss",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "unsupported_connector" {
t.Errorf("Expected error code 'unsupported_connector', got %q", code)
}
}

func TestGetSource_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Get Me",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Source
parseResponseData(t, w, &created)

// Get by short ID.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ShortID, nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
var got core.Source
parseResponseData(t, w, &got)
if got.ID != created.ID {
t.Errorf("Expected ID %s, got %s", created.ID, got.ID)
}
if got.Name != "Get Me" {
t.Errorf("Expected name 'Get Me', got %q", got.Name)
}

// Get by UUID.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ID.String(), nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200 for UUID access, got %d: %s", w.Code, w.Body.String())
}
}

func TestGetSource_NotFound(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources/SRC-999", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "not_found" {
t.Errorf("Expected error code 'not_found', got %q", code)
}
}

func TestListSources_Pagination(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Create 3 sources.
for i := range 3 {
body := map[string]any{
"name":      "Source " + string(rune('A'+i)),
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Failed to create source %d: %s", i, w.Body.String())
}
}

// List with limit=2.
w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources?limit=2&offset=0", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var envelope struct {
Data []core.Source `json:"data"`
Meta struct {
Total  int `json:"total"`
Limit  int `json:"limit"`
Offset int `json:"offset"`
} `json:"meta"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if len(envelope.Data) != 2 {
t.Errorf("Expected 2 sources, got %d", len(envelope.Data))
}
if envelope.Meta.Total != 3 {
t.Errorf("Expected total 3, got %d", envelope.Meta.Total)
}

// List with offset=2.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources?limit=2&offset=2", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if len(envelope.Data) != 1 {
t.Errorf("Expected 1 source at offset 2, got %d", len(envelope.Data))
}
}

func TestUpdateSource_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Original",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Source
parseResponseData(t, w, &created)

updateBody := map[string]any{
"name":   "Updated",
"status": "paused",
}
headers := map[string]string{"If-Match": `"1"`}
w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ShortID, updateBody, headers)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var updated core.Source
parseResponseData(t, w, &updated)
if updated.Name != "Updated" {
t.Errorf("Expected name 'Updated', got %q", updated.Name)
}
if updated.Status != core.StatusPaused {
t.Errorf("Expected status 'paused', got %q", updated.Status)
}
if updated.Version != 2 {
t.Errorf("Expected version 2, got %d", updated.Version)
}
}

func TestUpdateSource_MissingIfMatch(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Source",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Source
parseResponseData(t, w, &created)

updateBody := map[string]any{"name": "Updated"}
w = doRequest(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ShortID, updateBody)
if w.Code != http.StatusPreconditionRequired {
t.Fatalf("Expected 428, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "if_match_required" {
t.Errorf("Expected error code 'if_match_required', got %q", code)
}
}

func TestDeleteSource_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Delete Me",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Source
parseResponseData(t, w, &created)

w = doRequest(t, mux, "DELETE", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ShortID, nil)
if w.Code != http.StatusNoContent {
t.Fatalf("Expected 204, got %d: %s", w.Code, w.Body.String())
}

// Verify gone.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sources/"+created.ShortID, nil)
if w.Code != http.StatusNotFound {
t.Errorf("Expected 404 after delete, got %d", w.Code)
}
}

// --- Sink CRUD Tests ---

func TestCreateSink_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "My Webhook Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

var sink core.Sink
parseResponseData(t, w, &sink)
if sink.Name != "My Webhook Sink" {
t.Errorf("Expected name 'My Webhook Sink', got %q", sink.Name)
}
if sink.Connector != core.ConnectorWebhook {
t.Errorf("Expected connector 'webhook', got %q", sink.Connector)
}
if sink.ShortID != "SINK-1" {
t.Errorf("Expected short ID SINK-1, got %q", sink.ShortID)
}
if sink.Version != 1 {
t.Errorf("Expected version 1, got %d", sink.Version)
}
if sink.Status != core.StatusActive {
t.Errorf("Expected status 'active', got %q", sink.Status)
}
if w.Header().Get("ETag") == "" {
t.Error("Expected ETag header")
}
}

func TestCreateSink_InvalidBody(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

req := httptest.NewRequest("POST", "/api/v1/repos/"+repo.Slug+"/sinks", bytes.NewReader([]byte("{")))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)

if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_body" {
t.Errorf("Expected error code 'invalid_body', got %q", code)
}
}

func TestCreateSink_MissingName(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "validation_failed" {
t.Errorf("Expected error code 'validation_failed', got %q", code)
}
}

func TestGetSink_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Get Me Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Sink
parseResponseData(t, w, &created)

// Get by short ID.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ShortID, nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
var got core.Sink
parseResponseData(t, w, &got)
if got.ID != created.ID {
t.Errorf("Expected ID %s, got %s", created.ID, got.ID)
}

// Get by UUID.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ID.String(), nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200 for UUID access, got %d: %s", w.Code, w.Body.String())
}
}

func TestGetSink_NotFound(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sinks/SINK-999", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}

func TestListSinks_Pagination(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Create 3 sinks.
for i := range 3 {
body := map[string]any{
"name":      "Sink " + string(rune('A'+i)),
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Failed to create sink %d: %s", i, w.Body.String())
}
}

// List with limit=2.
w := doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sinks?limit=2&offset=0", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var envelope struct {
Data []core.Sink `json:"data"`
Meta struct {
Total  int `json:"total"`
Limit  int `json:"limit"`
Offset int `json:"offset"`
} `json:"meta"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if len(envelope.Data) != 2 {
t.Errorf("Expected 2 sinks, got %d", len(envelope.Data))
}
if envelope.Meta.Total != 3 {
t.Errorf("Expected total 3, got %d", envelope.Meta.Total)
}
}

func TestUpdateSink_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Original Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Sink
parseResponseData(t, w, &created)

updateBody := map[string]any{
"name":   "Updated Sink",
"status": "paused",
}
headers := map[string]string{"If-Match": `"1"`}
w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ShortID, updateBody, headers)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var updated core.Sink
parseResponseData(t, w, &updated)
if updated.Name != "Updated Sink" {
t.Errorf("Expected name 'Updated Sink', got %q", updated.Name)
}
if updated.Status != core.StatusPaused {
t.Errorf("Expected status 'paused', got %q", updated.Status)
}
if updated.Version != 2 {
t.Errorf("Expected version 2, got %d", updated.Version)
}
}

func TestUpdateSink_MissingIfMatch(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Sink
parseResponseData(t, w, &created)

updateBody := map[string]any{"name": "Updated"}
w = doRequest(t, mux, "PUT", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ShortID, updateBody)
if w.Code != http.StatusPreconditionRequired {
t.Fatalf("Expected 428, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "if_match_required" {
t.Errorf("Expected error code 'if_match_required', got %q", code)
}
}

func TestDeleteSink_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Delete Me Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Sink
parseResponseData(t, w, &created)

w = doRequest(t, mux, "DELETE", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ShortID, nil)
if w.Code != http.StatusNoContent {
t.Fatalf("Expected 204, got %d: %s", w.Code, w.Body.String())
}

// Verify gone.
w = doRequest(t, mux, "GET", "/api/v1/repos/"+repo.Slug+"/sinks/"+created.ShortID, nil)
if w.Code != http.StatusNotFound {
t.Errorf("Expected 404 after delete, got %d", w.Code)
}
}

// --- Collect Trigger Tests ---

func TestTriggerCollect_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Collect Source",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var source core.Source
parseResponseData(t, w, &source)

// Trigger collect.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources/"+source.ShortID+"/collect", nil)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202, got %d: %s", w.Code, w.Body.String())
}

var job core.Job
parseResponseData(t, w, &job)
if job.Type != core.JobCollect {
t.Errorf("Expected job type 'collect', got %q", job.Type)
}
if job.Status != core.JobStatusPending {
t.Errorf("Expected job status 'pending', got %q", job.Status)
}
}

func TestTriggerCollect_SourceNotFound(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources/SRC-999/collect", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}

func TestTriggerCollect_RateLimited(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Create a source with rate limit of 1 request per 60 seconds.
body := map[string]any{
"name":      "Rate Limited Source",
"connector": "webhook",
"config":    map[string]any{},
"rate_limit": map[string]any{
"max_requests":        1,
"window_seconds":      60,
"retry_after_seconds": 30,
},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var source core.Source
parseResponseData(t, w, &source)

// First collect should succeed.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources/"+source.ShortID+"/collect", nil)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202 for first collect, got %d: %s", w.Code, w.Body.String())
}

// Second collect should be rate limited.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources/"+source.ShortID+"/collect", nil)
if w.Code != http.StatusTooManyRequests {
t.Fatalf("Expected 429, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "rate_limited" {
t.Errorf("Expected error code 'rate_limited', got %q", code)
}
}

// --- Publish Trigger Tests ---

func TestTriggerPublish_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"name":      "Publish Sink",
"connector": "webhook",
"config":    map[string]any{},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var sink core.Sink
parseResponseData(t, w, &sink)

// Trigger publish.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks/"+sink.ShortID+"/publish", nil)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202, got %d: %s", w.Code, w.Body.String())
}

var job core.Job
parseResponseData(t, w, &job)
if job.Type != core.JobPublish {
t.Errorf("Expected job type 'publish', got %q", job.Type)
}
if job.Status != core.JobStatusPending {
t.Errorf("Expected job status 'pending', got %q", job.Status)
}
}

func TestTriggerPublish_SinkNotFound(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks/SINK-999/publish", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}

func TestTriggerPublish_RateLimited(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Create a sink with rate limit of 1 request per 60 seconds.
body := map[string]any{
"name":      "Rate Limited Sink",
"connector": "webhook",
"config":    map[string]any{},
"rate_limit": map[string]any{
"max_requests":        1,
"window_seconds":      60,
"retry_after_seconds": 30,
},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var sink core.Sink
parseResponseData(t, w, &sink)

// First publish should succeed.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks/"+sink.ShortID+"/publish", nil)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202 for first publish, got %d: %s", w.Code, w.Body.String())
}

// Second publish should be rate limited.
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks/"+sink.ShortID+"/publish", nil)
if w.Code != http.StatusTooManyRequests {
t.Fatalf("Expected 429, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "rate_limited" {
t.Errorf("Expected error code 'rate_limited', got %q", code)
}
}

// --- Cycle Detection Tests ---

func TestCreateSource_CycleDetected(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Try to create a repo-type source that references the same repo (self-cycle).
body := map[string]any{
"name":      "Self-referencing Source",
"connector": "repo",
"config":    map[string]any{"repo_id": repo.ID.String()},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sources", body)
if w.Code != http.StatusConflict {
t.Fatalf("Expected 409, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "cycle_detected" {
t.Errorf("Expected error code 'cycle_detected', got %q", code)
}
}

func TestCreateSink_CycleDetected(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

// Try to create a repo-type sink that references the same repo (self-cycle).
body := map[string]any{
"name":      "Self-referencing Sink",
"connector": "repo",
"config":    map[string]any{"repo_id": repo.ID.String()},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/sinks", body)
if w.Code != http.StatusConflict {
t.Fatalf("Expected 409, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "cycle_detected" {
t.Errorf("Expected error code 'cycle_detected', got %q", code)
}
}

func TestCreateSource_IndirectCycleDetected(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

// Create two repos: A and B.
repoA := &core.Repository{
ID:      core.NewID(),
Slug:    "repo-a",
Name:    "Repo A",
OwnerID: defaultOwnerID,
}
if err := m.archive.SaveRepository(repoA); err != nil {
t.Fatalf("Failed to create repo A: %v", err)
}
repoB := &core.Repository{
ID:      core.NewID(),
Slug:    "repo-b",
Name:    "Repo B",
OwnerID: defaultOwnerID,
}
if err := m.archive.SaveRepository(repoB); err != nil {
t.Fatalf("Failed to create repo B: %v", err)
}

// Create a source in repo B that points to repo A (B -> A).
body := map[string]any{
"name":      "B to A Source",
"connector": "repo",
"config":    map[string]any{"repo_id": repoA.ID.String()},
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repoB.Slug+"/sources", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

// Now try to create a source in repo A that points to repo B (A -> B).
// This would form a cycle: A -> B -> A.
body = map[string]any{
"name":      "A to B Source",
"connector": "repo",
"config":    map[string]any{"repo_id": repoB.ID.String()},
}
w = doRequest(t, mux, "POST", "/api/v1/repos/"+repoA.Slug+"/sources", body)
if w.Code != http.StatusConflict {
t.Fatalf("Expected 409 for indirect cycle, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "cycle_detected" {
t.Errorf("Expected error code 'cycle_detected', got %q", code)
}
}

// --- One-shot Collect Tests ---

func TestOneShotCollect_Success(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"url":          "https://example.com/article",
"content_type": "text/html",
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/collect", body)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202, got %d: %s", w.Code, w.Body.String())
}

var job core.Job
parseResponseData(t, w, &job)
if job.Type != core.JobWebhook {
t.Errorf("Expected job type 'webhook', got %q", job.Type)
}
}

func TestOneShotCollect_MissingURL(t *testing.T) {
m := setupTestModule(t)
repo := createTestRepo(t, m)
mux := buildMux(m)

body := map[string]any{
"content_type": "text/html",
}
w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/collect", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "validation_failed" {
t.Errorf("Expected error code 'validation_failed', got %q", code)
}
}
