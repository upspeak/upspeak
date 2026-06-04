package archive

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// makeRule builds a Rule with sensible defaults for the given repo.
func makeRule(repo *core.Repository, name string, event core.EventType) *core.Rule {
	return &core.Rule{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Name:   name,
		Trigger: core.RuleTrigger{
			Event: event,
		},
		Actions: []core.RuleAction{
			{Type: core.ActionWebhook, Params: json.RawMessage(`{"url":"https://example.test"}`)},
		},
		Status:    core.StatusActive,
		CreatedBy: testOwnerID,
	}
}

func TestSaveAndGetRule(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	rule := makeRule(repo, "notify on node create", core.EventType("NodeCreated"))
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}
	if rule.Version != 1 {
		t.Errorf("expected version 1, got %d", rule.Version)
	}
	if rule.ShortID != "RULE-1" {
		t.Errorf("expected short ID RULE-1, got %s", rule.ShortID)
	}

	got, err := a.GetRule(rule.ID)
	if err != nil {
		t.Fatalf("GetRule failed: %v", err)
	}
	if got.Name != "notify on node create" {
		t.Errorf("unexpected name %q", got.Name)
	}
	if got.Trigger.Event != core.EventType("NodeCreated") {
		t.Errorf("unexpected trigger event %q", got.Trigger.Event)
	}
	if len(got.Actions) != 1 || got.Actions[0].Type != core.ActionWebhook {
		t.Errorf("unexpected actions %+v", got.Actions)
	}
}

func TestGetRule_NotFound(t *testing.T) {
	a := setupTestArchive(t)
	if _, err := a.GetRule(core.NewID()); err == nil {
		t.Fatal("expected not-found error for missing rule")
	}
}

func TestSaveRule_VersionConflict(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	rule := makeRule(repo, "rule", core.EventType("NodeCreated"))
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	// Stale update: a writer holding version 1 after it was bumped to 2.
	rule.Name = "first update"
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	stale := *rule
	stale.Version = 1
	stale.Name = "stale update"
	err := a.SaveRule(&stale)
	if err == nil {
		t.Fatal("expected version conflict error")
	}
	if _, ok := err.(*core.VersionConflictError); !ok {
		t.Errorf("expected *core.VersionConflictError, got %T", err)
	}
}

func TestDeleteRule(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	rule := makeRule(repo, "rule", core.EventType("NodeCreated"))
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}
	if err := a.DeleteRule(rule.ID); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}
	if _, err := a.GetRule(rule.ID); err == nil {
		t.Error("expected rule to be gone after delete")
	}
	if err := a.DeleteRule(rule.ID); err == nil {
		t.Error("expected not-found when deleting a missing rule")
	}
}

func TestListRules_StatusFilter(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	active := makeRule(repo, "active rule", core.EventType("NodeCreated"))
	if err := a.SaveRule(active); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}
	paused := makeRule(repo, "paused rule", core.EventType("NodeCreated"))
	paused.Status = core.StatusPaused
	if err := a.SaveRule(paused); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	rules, total, err := a.ListRules(repo.ID, core.RuleListOptions{
		Status:      core.StatusActive,
		ListOptions: core.DefaultListOptions(),
	})
	if err != nil {
		t.Fatalf("ListRules failed: %v", err)
	}
	if total != 1 || len(rules) != 1 {
		t.Fatalf("expected 1 active rule, got %d (total %d)", len(rules), total)
	}
	if rules[0].ID != active.ID {
		t.Errorf("expected active rule %s, got %s", active.ID, rules[0].ID)
	}
}

func TestGetActiveRulesForEvent(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	created := makeRule(repo, "on created", core.EventType("NodeCreated"))
	if err := a.SaveRule(created); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}
	updated := makeRule(repo, "on updated", core.EventType("NodeUpdated"))
	if err := a.SaveRule(updated); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}
	pausedCreated := makeRule(repo, "paused on created", core.EventType("NodeCreated"))
	pausedCreated.Status = core.StatusPaused
	if err := a.SaveRule(pausedCreated); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	rules, err := a.GetActiveRulesForEvent(repo.ID, core.EventType("NodeCreated"))
	if err != nil {
		t.Fatalf("GetActiveRulesForEvent failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 active rule for NodeCreated, got %d", len(rules))
	}
	if rules[0].ID != created.ID {
		t.Errorf("expected rule %s, got %s", created.ID, rules[0].ID)
	}
}

func TestRuleExecutions_SaveAndList(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	rule := makeRule(repo, "rule", core.EventType("NodeCreated"))
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		exec := &core.RuleExecution{
			RuleID:    rule.ID,
			EventID:   core.NewID(),
			EventType: core.EventType("NodeCreated"),
			ActionsExecuted: []core.ActionExecutionEntry{
				{Type: core.ActionWebhook, Result: "success"},
			},
			At:         time.Now().UTC(),
			DurationMs: int64(10 + i),
		}
		if err := a.SaveRuleExecution(exec); err != nil {
			t.Fatalf("SaveRuleExecution failed: %v", err)
		}
		if exec.ID == uuid.Nil {
			t.Error("expected execution ID to be assigned")
		}
	}

	execs, total, err := a.ListRuleExecutions(rule.ID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListRuleExecutions failed: %v", err)
	}
	if total != 3 || len(execs) != 3 {
		t.Fatalf("expected 3 executions, got %d (total %d)", len(execs), total)
	}
}

// TestFilterReferences_IncludesRules verifies that a filter referenced by a
// rule trigger is reported as referenced, preventing its deletion.
func TestFilterReferences_IncludesRules(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	filter := &core.Filter{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Name:   "important filter",
		Conditions: []core.Condition{
			{Field: "type", Op: core.OpEq, Value: json.RawMessage(`"note"`)},
		},
		Mode:      core.FilterModeAll,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveFilter(filter); err != nil {
		t.Fatalf("SaveFilter failed: %v", err)
	}

	rule := makeRule(repo, "rule with filter", core.EventType("NodeCreated"))
	rule.Trigger.FilterIDs = []uuid.UUID{filter.ID}
	if err := a.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule failed: %v", err)
	}

	refs, err := a.GetFilterReferences(filter.ID)
	if err != nil {
		t.Fatalf("GetFilterReferences failed: %v", err)
	}
	var foundRule bool
	for _, r := range refs {
		if r.EntityType == "rule" && r.EntityID == rule.ID.String() {
			foundRule = true
		}
	}
	if !foundRule {
		t.Errorf("expected filter to be referenced by rule, got refs %+v", refs)
	}
}
