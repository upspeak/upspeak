package rules

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// mockSubscriber records the subjects it was asked to subscribe to.
type mockSubscriber struct {
	subjects []string
}

func (s *mockSubscriber) Subscribe(subject string, _ func(subject string, data []byte)) error {
	s.subjects = append(s.subjects, subject)
	return nil
}

// testEngine builds an Engine over the module's archive with a recording publisher.
func testEngine(m *Module, pub *mockPublisher) *Engine {
	return NewEngine(m.archive, pub, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// saveRule persists a rule directly via the archive for engine tests.
func saveRule(t *testing.T, m *Module, repo *core.Repository, status core.ResourceStatus, filterIDs []uuid.UUID) *core.Rule {
	t.Helper()
	rule := &core.Rule{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Name:   "engine rule",
		Trigger: core.RuleTrigger{
			Event:     core.EventNodeCreated,
			FilterIDs: filterIDs,
		},
		Actions: []core.RuleAction{
			{Type: core.ActionWebhook, Params: json.RawMessage(`{"url":"https://example.test"}`)},
		},
		Status:    status,
		CreatedBy: defaultOwnerID,
	}
	if err := m.archive.SaveRule(rule); err != nil {
		t.Fatalf("Failed to save rule: %v", err)
	}
	return rule
}

// nodeCreatedEvent builds a marshalled NodeCreated event with the given node type.
func nodeCreatedEvent(t *testing.T, repoID uuid.UUID, nodeType string) []byte {
	t.Helper()
	evt, err := core.NewEvent(core.EventNodeCreated, repoID, map[string]any{
		"node": map[string]any{"type": nodeType},
	})
	if err != nil {
		t.Fatalf("Failed to build event: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	return data
}

func executionCount(t *testing.T, m *Module, ruleID uuid.UUID) int {
	t.Helper()
	_, total, err := m.archive.ListRuleExecutions(ruleID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListRuleExecutions failed: %v", err)
	}
	return total
}

func TestEngine_Start(t *testing.T) {
	m, pub := setupTestModule(t)
	sub := &mockSubscriber{}
	e := NewEngine(m.archive, pub, sub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := e.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if len(sub.subjects) != 1 || sub.subjects[0] != allRepoEventsSubject {
		t.Errorf("Expected subscription to %q, got %v", allRepoEventsSubject, sub.subjects)
	}
}

func TestEngine_FiresMatchingRule(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	rule := saveRule(t, m, repo, core.StatusActive, nil)
	e := testEngine(m, pub)

	e.handleEvent("", nodeCreatedEvent(t, repo.ID, "article"))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution recorded, got %d", got)
	}
	// A webhook action should have enqueued a job.
	_, total, err := m.archive.ListJobs(core.JobListOptions{ListOptions: core.DefaultListOptions()})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Expected 1 job enqueued, got %d", total)
	}
	// A RuleTriggered event should have been published.
	if !publishedSubjectContains(pub, "events."+string(core.EventRuleTriggered)) {
		t.Error("Expected a RuleTriggered event to be published")
	}
}

func TestEngine_FilterMatchAndBlock(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	f := createTestFilterForRepo(t, m, repo, "node.type", "article")
	rule := saveRule(t, m, repo, core.StatusActive, []uuid.UUID{f.ID})
	e := testEngine(m, pub)

	// Non-matching payload: filter blocks the rule.
	e.handleEvent("", nodeCreatedEvent(t, repo.ID, "note"))
	if got := executionCount(t, m, rule.ID); got != 0 {
		t.Fatalf("Expected 0 executions for blocked rule, got %d", got)
	}

	// Matching payload: filter passes, rule fires.
	e.handleEvent("", nodeCreatedEvent(t, repo.ID, "article"))
	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution after match, got %d", got)
	}
}

func TestEngine_SkipsPausedRule(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	rule := saveRule(t, m, repo, core.StatusPaused, nil)
	e := testEngine(m, pub)

	e.handleEvent("", nodeCreatedEvent(t, repo.ID, "article"))
	if got := executionCount(t, m, rule.ID); got != 0 {
		t.Errorf("Expected paused rule not to fire, got %d executions", got)
	}
}

func TestEngine_IgnoresMetaEvents(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	e := testEngine(m, pub)

	evt, _ := core.NewEvent(core.EventRuleTriggered, repo.ID, map[string]any{"rule_id": core.NewID()})
	data, _ := json.Marshal(evt)
	// Should return early without panicking or publishing anything.
	e.handleEvent("", data)
	if len(pub.published) != 0 {
		t.Errorf("Expected no publishes for meta-event, got %d", len(pub.published))
	}
}

func publishedSubjectContains(pub *mockPublisher, substr string) bool {
	for _, msg := range pub.published {
		if strings.Contains(msg.Subject, substr) {
			return true
		}
	}
	return false
}
