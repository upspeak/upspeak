package scheduler

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

// setupTestModule creates a scheduler Module wired to a temporary archive.
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

// buildMux registers all scheduler HTTP handlers on a ServeMux.
func buildMux(m *Module) *http.ServeMux {
mux := http.NewServeMux()
for _, h := range m.HTTPHandlers() {
mux.HandleFunc(h.Method+" /api/v1"+h.Path, h.Handler)
}
return mux
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

// validCollectAction returns a valid collect action for testing.
func validCollectAction() map[string]any {
sourceID := uuid.New().String()
repoID := uuid.New().String()
return map[string]any{
"type":      "collect",
"source_id": sourceID,
"repo_id":   repoID,
}
}

// validWebhookAction returns a valid webhook action for testing.
func validWebhookAction() map[string]any {
return map[string]any{
"type":   "webhook",
"params": map[string]any{"url": "https://example.com/hook"},
}
}

// --- Create Schedule Tests ---

func TestCreateSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Daily Collect",
"cron":   "0 9 * * *",
"action": validCollectAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

var schedule core.Schedule
parseResponseData(t, w, &schedule)
if schedule.Name != "Daily Collect" {
t.Errorf("Expected name 'Daily Collect', got %q", schedule.Name)
}
if schedule.Cron != "0 9 * * *" {
t.Errorf("Expected cron '0 9 * * *', got %q", schedule.Cron)
}
if !schedule.Enabled {
t.Error("Expected schedule to be enabled")
}
if schedule.NextRun == nil {
t.Error("Expected next_run to be set")
}
if schedule.Version != 1 {
t.Errorf("Expected version 1, got %d", schedule.Version)
}
if schedule.ShortID == "" {
t.Error("Expected short_id to be set")
}
if w.Header().Get("ETag") == "" {
t.Error("Expected ETag header")
}
}

func TestCreateSchedule_InvalidBody(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

req := httptest.NewRequest("POST", "/api/v1/schedules", bytes.NewReader([]byte("not json")))
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

func TestCreateSchedule_MissingName(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"cron":   "0 9 * * *",
"action": validCollectAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "name_required" {
t.Errorf("Expected error code 'name_required', got %q", code)
}
}

func TestCreateSchedule_MissingCron(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "No Cron",
"action": validCollectAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "cron_required" {
t.Errorf("Expected error code 'cron_required', got %q", code)
}
}

func TestCreateSchedule_InvalidCron(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Bad Cron",
"cron":   "not a cron",
"action": validCollectAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_cron" {
t.Errorf("Expected error code 'invalid_cron', got %q", code)
}
}

func TestCreateSchedule_InvalidActionType(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Bad Action",
"cron":   "0 9 * * *",
"action": map[string]any{"type": "invalid"},
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_action" {
t.Errorf("Expected error code 'invalid_action', got %q", code)
}
}

func TestCreateSchedule_CollectMissingSourceID(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name": "No Source",
"cron": "0 9 * * *",
"action": map[string]any{
"type":    "collect",
"repo_id": uuid.New().String(),
},
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_action" {
t.Errorf("Expected error code 'invalid_action', got %q", code)
}
}

func TestCreateSchedule_CollectMissingRepoID(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name": "No Repo",
"cron": "0 9 * * *",
"action": map[string]any{
"type":      "collect",
"source_id": uuid.New().String(),
},
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_action" {
t.Errorf("Expected error code 'invalid_action', got %q", code)
}
}

func TestCreateSchedule_WebhookMissingURL(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name": "No URL",
"cron": "0 9 * * *",
"action": map[string]any{
"type":   "webhook",
"params": map[string]any{"method": "POST"},
},
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_action" {
t.Errorf("Expected error code 'invalid_action', got %q", code)
}
}

// --- Get Schedule Tests ---

func TestGetSchedule_ByShortID(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Get Me",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Get by short ID.
w = doRequest(t, mux, "GET", "/api/v1/schedules/"+created.ShortID, nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
var got core.Schedule
parseResponseData(t, w, &got)
if got.ID != created.ID {
t.Errorf("Expected ID %s, got %s", created.ID, got.ID)
}
if got.Name != "Get Me" {
t.Errorf("Expected name 'Get Me', got %q", got.Name)
}
}

func TestGetSchedule_ByUUID(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "UUID Access",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Get by UUID.
w = doRequest(t, mux, "GET", "/api/v1/schedules/"+created.ID.String(), nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200 for UUID access, got %d: %s", w.Code, w.Body.String())
}
}

func TestGetSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

w := doRequest(t, mux, "GET", "/api/v1/schedules/SCHED-999", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "not_found" {
t.Errorf("Expected error code 'not_found', got %q", code)
}
}

// --- List Schedules Tests ---

func TestListSchedules_Default(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

// Create 3 schedules.
for i := range 3 {
body := map[string]any{
"name":   "Schedule " + string(rune('A'+i)),
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Failed to create schedule %d: %s", i, w.Body.String())
}
}

w := doRequest(t, mux, "GET", "/api/v1/schedules", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var envelope struct {
Data []core.Schedule `json:"data"`
Meta struct {
Total  int `json:"total"`
Limit  int `json:"limit"`
Offset int `json:"offset"`
} `json:"meta"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if len(envelope.Data) != 3 {
t.Errorf("Expected 3 schedules, got %d", len(envelope.Data))
}
if envelope.Meta.Total != 3 {
t.Errorf("Expected total 3, got %d", envelope.Meta.Total)
}
}

func TestListSchedules_EnabledFilter(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

// Create a schedule then pause it.
body := map[string]any{
"name":   "Will Pause",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Pause it.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/pause", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

// Create an active schedule.
body["name"] = "Still Active"
w = doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

// Filter enabled=true.
w = doRequest(t, mux, "GET", "/api/v1/schedules?enabled=true", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
var envelope struct {
Data []core.Schedule `json:"data"`
Meta struct {
Total int `json:"total"`
} `json:"meta"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if envelope.Meta.Total != 1 {
t.Errorf("Expected 1 enabled schedule, got %d", envelope.Meta.Total)
}

// Filter enabled=false.
w = doRequest(t, mux, "GET", "/api/v1/schedules?enabled=false", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if envelope.Meta.Total != 1 {
t.Errorf("Expected 1 disabled schedule, got %d", envelope.Meta.Total)
}
}

func TestListSchedules_ActionTypeFilter(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

// Create a webhook schedule.
body := map[string]any{
"name":   "Webhook Schedule",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

// Create a collect schedule.
body = map[string]any{
"name":   "Collect Schedule",
"cron":   "0 10 * * *",
"action": validCollectAction(),
}
w = doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}

// Filter by action_type=webhook.
w = doRequest(t, mux, "GET", "/api/v1/schedules?action_type=webhook", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}
var envelope struct {
Data []core.Schedule `json:"data"`
Meta struct {
Total int `json:"total"`
} `json:"meta"`
}
if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
t.Fatalf("Failed to parse response: %v", err)
}
if envelope.Meta.Total != 1 {
t.Errorf("Expected 1 webhook schedule, got %d", envelope.Meta.Total)
}
if len(envelope.Data) > 0 && envelope.Data[0].Action.Type != "webhook" {
t.Errorf("Expected action type 'webhook', got %q", envelope.Data[0].Action.Type)
}
}

// --- Update Schedule Tests ---

func TestUpdateSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Original",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

updateBody := map[string]any{
"name": "Updated",
"cron": "0 10 * * *",
}
headers := map[string]string{"If-Match": `"1"`}
w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/schedules/"+created.ShortID, updateBody, headers)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var updated core.Schedule
parseResponseData(t, w, &updated)
if updated.Name != "Updated" {
t.Errorf("Expected name 'Updated', got %q", updated.Name)
}
if updated.Cron != "0 10 * * *" {
t.Errorf("Expected cron '0 10 * * *', got %q", updated.Cron)
}
if updated.Version != 2 {
t.Errorf("Expected version 2, got %d", updated.Version)
}
}

func TestUpdateSchedule_MissingIfMatch(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Schedule",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

updateBody := map[string]any{"name": "Updated"}
w = doRequest(t, mux, "PUT", "/api/v1/schedules/"+created.ShortID, updateBody)
if w.Code != http.StatusPreconditionRequired {
t.Fatalf("Expected 428, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "if_match_required" {
t.Errorf("Expected error code 'if_match_required', got %q", code)
}
}

func TestUpdateSchedule_VersionConflict(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Schedule",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Use wrong version.
updateBody := map[string]any{"name": "Updated"}
headers := map[string]string{"If-Match": `"99"`}
w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/schedules/"+created.ShortID, updateBody, headers)
if w.Code != http.StatusPreconditionFailed {
t.Fatalf("Expected 412, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "version_mismatch" {
t.Errorf("Expected error code 'version_mismatch', got %q", code)
}
}

func TestUpdateSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

updateBody := map[string]any{"name": "Updated"}
headers := map[string]string{"If-Match": `"1"`}
w := doRequestWithHeaders(t, mux, "PUT", "/api/v1/schedules/SCHED-999", updateBody, headers)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}

func TestUpdateSchedule_InvalidCron(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Schedule",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

updateBody := map[string]any{"cron": "bad cron"}
headers := map[string]string{"If-Match": `"1"`}
w = doRequestWithHeaders(t, mux, "PUT", "/api/v1/schedules/"+created.ShortID, updateBody, headers)
if w.Code != http.StatusBadRequest {
t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "invalid_cron" {
t.Errorf("Expected error code 'invalid_cron', got %q", code)
}
}

// --- Delete Schedule Tests ---

func TestDeleteSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Delete Me",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

w = doRequest(t, mux, "DELETE", "/api/v1/schedules/"+created.ShortID, nil)
if w.Code != http.StatusNoContent {
t.Fatalf("Expected 204, got %d: %s", w.Code, w.Body.String())
}

// Verify gone.
w = doRequest(t, mux, "GET", "/api/v1/schedules/"+created.ShortID, nil)
if w.Code != http.StatusNotFound {
t.Errorf("Expected 404 after delete, got %d", w.Code)
}
}

func TestDeleteSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

w := doRequest(t, mux, "DELETE", "/api/v1/schedules/SCHED-999", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "not_found" {
t.Errorf("Expected error code 'not_found', got %q", code)
}
}

// --- Trigger Schedule Tests ---

func TestTriggerSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Triggerable",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/trigger", nil)
if w.Code != http.StatusAccepted {
t.Fatalf("Expected 202, got %d: %s", w.Code, w.Body.String())
}

// Verify a job was created.
var job core.Job
parseResponseData(t, w, &job)
if job.Type != core.JobWebhook {
t.Errorf("Expected job type 'webhook', got %q", job.Type)
}
if job.ShortID == "" {
t.Error("Expected job short_id to be set")
}
}

func TestTriggerSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

w := doRequest(t, mux, "POST", "/api/v1/schedules/SCHED-999/trigger", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
code, _ := parseErrorResponse(t, w)
if code != "not_found" {
t.Errorf("Expected error code 'not_found', got %q", code)
}
}

// --- Pause Schedule Tests ---

func TestPauseSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Pauseable",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/pause", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var paused core.Schedule
parseResponseData(t, w, &paused)
if paused.Enabled {
t.Error("Expected schedule to be disabled after pause")
}
if paused.NextRun != nil {
t.Error("Expected next_run to be nil after pause")
}
}

func TestPauseSchedule_AlreadyPaused(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Already Paused",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Pause once.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/pause", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

// Pause again — idempotent.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/pause", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200 for idempotent pause, got %d: %s", w.Code, w.Body.String())
}
var paused core.Schedule
parseResponseData(t, w, &paused)
if paused.Enabled {
t.Error("Expected schedule to remain disabled")
}
}

func TestPauseSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

w := doRequest(t, mux, "POST", "/api/v1/schedules/SCHED-999/pause", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}

// --- Resume Schedule Tests ---

func TestResumeSchedule_Success(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Resumable",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Pause first.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/pause", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

// Resume.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/resume", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200, got %d: %s", w.Code, w.Body.String())
}

var resumed core.Schedule
parseResponseData(t, w, &resumed)
if !resumed.Enabled {
t.Error("Expected schedule to be enabled after resume")
}
if resumed.NextRun == nil {
t.Error("Expected next_run to be set after resume")
}
}

func TestResumeSchedule_AlreadyRunning(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

body := map[string]any{
"name":   "Already Running",
"cron":   "0 9 * * *",
"action": validWebhookAction(),
}
w := doRequest(t, mux, "POST", "/api/v1/schedules", body)
if w.Code != http.StatusCreated {
t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
}
var created core.Schedule
parseResponseData(t, w, &created)

// Resume when already enabled — idempotent.
w = doRequest(t, mux, "POST", "/api/v1/schedules/"+created.ShortID+"/resume", nil)
if w.Code != http.StatusOK {
t.Fatalf("Expected 200 for idempotent resume, got %d: %s", w.Code, w.Body.String())
}
var schedule core.Schedule
parseResponseData(t, w, &schedule)
if !schedule.Enabled {
t.Error("Expected schedule to remain enabled")
}
}

func TestResumeSchedule_NotFound(t *testing.T) {
m := setupTestModule(t)
mux := buildMux(m)

w := doRequest(t, mux, "POST", "/api/v1/schedules/SCHED-999/resume", nil)
if w.Code != http.StatusNotFound {
t.Fatalf("Expected 404, got %d: %s", w.Code, w.Body.String())
}
}
