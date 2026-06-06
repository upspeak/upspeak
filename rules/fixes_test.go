package rules

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// saveRepoSlug creates a repository with a specific slug.
func saveRepoSlug(t *testing.T, m *Module, slug string) *core.Repository {
	t.Helper()
	repo := &core.Repository{
		ID:      core.NewID(),
		Slug:    slug,
		Name:    slug,
		OwnerID: defaultOwnerID,
	}
	if err := m.archive.SaveRepository(repo); err != nil {
		t.Fatalf("Failed to create repo %q: %v", slug, err)
	}
	return repo
}

// saveTestSource creates a webhook source in the given repository.
func saveTestSource(t *testing.T, m *Module, repo *core.Repository, name string) *core.Source {
	t.Helper()
	src := &core.Source{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      name,
		Connector: core.ConnectorWebhook,
		Status:    core.StatusActive,
		CreatedBy: defaultOwnerID,
	}
	if err := m.archive.SaveSource(src); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	return src
}

func jobCount(t *testing.T, m *Module) int {
	t.Helper()
	_, total, err := m.archive.ListJobs(core.JobListOptions{ListOptions: core.DefaultListOptions()})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	return total
}

// TestCreateRule_InvalidActionParams verifies create-time validation rejects an
// action missing its required params (#5).
func TestCreateRule_InvalidActionParams(t *testing.T) {
	m, _ := setupTestModule(t)
	repo := createTestRepo(t, m)
	mux := buildMux(m)

	body := validRuleBody()
	body["actions"] = []map[string]any{{"type": "collect", "params": map[string]any{}}}
	w := doRequest(t, mux, "POST", "/api/v1/repos/"+repo.Slug+"/rules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if code := parseErrorCode(t, w); code != "validation_failed" {
		t.Errorf("Expected validation_failed, got %q", code)
	}
}

// TestEngine_CollectAction_ResolvesSource verifies a collect action whose
// source_id belongs to the rule's repository enqueues a job (#5).
func TestEngine_CollectAction_ResolvesSource(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	src := saveTestSource(t, m, repo, "src")
	node := saveTestNode(t, m, repo, "article", "subject")
	rule := saveActionRule(t, m, repo, core.EventNodeCreated, nil,
		core.RuleAction{Type: core.ActionCollect, Params: json.RawMessage(`{"source_id":"` + src.ID.String() + `"}`)})

	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node}))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution, got %d", got)
	}
	if got := jobCount(t, m); got != 1 {
		t.Errorf("Expected 1 collect job enqueued, got %d", got)
	}
}

// TestEngine_CollectAction_CrossRepoRejected verifies a collect action cannot be
// steered at another repository's source: the action errors and no job runs (#5).
func TestEngine_CollectAction_CrossRepoRejected(t *testing.T) {
	m, pub := setupTestModule(t)
	repoA := saveRepoSlug(t, m, "repo-a")
	repoB := saveRepoSlug(t, m, "repo-b")
	foreignSrc := saveTestSource(t, m, repoB, "foreign")
	node := saveTestNode(t, m, repoA, "article", "subject")
	rule := saveActionRule(t, m, repoA, core.EventNodeCreated, nil,
		core.RuleAction{Type: core.ActionCollect, Params: json.RawMessage(`{"source_id":"` + foreignSrc.ID.String() + `"}`)})

	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeCreated, repoA.ID, core.EventNodeCreatePayload{Node: node}))

	execs, total, err := m.archive.ListRuleExecutions(rule.ID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListRuleExecutions failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("Expected 1 execution recorded, got %d", total)
	}
	if execs[0].ActionsExecuted[0].Result != "error" {
		t.Errorf("Expected cross-repo collect to be recorded as error, got %q", execs[0].ActionsExecuted[0].Result)
	}
	if got := jobCount(t, m); got != 0 {
		t.Errorf("Expected no job enqueued for cross-repo source, got %d", got)
	}
}

// TestEngine_HopCapDropsDeepEvents verifies the hop counter bounds reaction
// cascades: an event at the hop limit is dropped without evaluation (#3).
func TestEngine_HopCapDropsDeepEvents(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	rule := saveRule(t, m, repo, core.StatusActive, nil)

	evt := &core.Event{
		ID:      core.NewID(),
		Type:    core.EventNodeCreated,
		RepoID:  repo.ID,
		Payload: json.RawMessage(`{"node":{"type":"article"}}`),
		Hops:    core.MaxEventHops,
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	testEngine(m, pub).dispatch(data)

	if got := executionCount(t, m, rule.ID); got != 0 {
		t.Errorf("Expected event at hop cap to be dropped, got %d executions", got)
	}
}
