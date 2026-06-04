package rules

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveActionRule saves an active rule with the given trigger event and actions.
func saveActionRule(t *testing.T, m *Module, repo *core.Repository, event core.EventType, filterIDs []uuid.UUID, actions ...core.RuleAction) *core.Rule {
	t.Helper()
	rule := &core.Rule{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "action rule",
		Trigger:   core.RuleTrigger{Event: event, FilterIDs: filterIDs},
		Actions:   actions,
		Status:    core.StatusActive,
		CreatedBy: defaultOwnerID,
	}
	if err := m.archive.SaveRule(rule); err != nil {
		t.Fatalf("Failed to save rule: %v", err)
	}
	return rule
}

// saveTestNode saves a node directly via the archive.
func saveTestNode(t *testing.T, m *Module, repo *core.Repository, nodeType, subject string) *core.Node {
	t.Helper()
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        nodeType,
		Subject:     subject,
		ContentType: "text/plain",
		Body:        json.RawMessage(`"body"`),
		CreatedBy:   defaultOwnerID,
	}
	if err := m.archive.SaveNode(node); err != nil {
		t.Fatalf("Failed to save node: %v", err)
	}
	return node
}

// eventFor builds a marshalled core.Event of the given type wrapping payloadObj.
func eventFor(t *testing.T, eventType core.EventType, repoID uuid.UUID, payloadObj any) []byte {
	t.Helper()
	evt, err := core.NewEvent(eventType, repoID, payloadObj)
	if err != nil {
		t.Fatalf("Failed to build event: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	return data
}

func TestEngine_EnrichAction(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	node := saveTestNode(t, m, repo, "article", "subject")
	rule := saveActionRule(t, m, repo, core.EventNodeCreated, nil,
		core.RuleAction{Type: core.ActionEnrich, Params: json.RawMessage(`{"metadata_key":"priority","metadata_value":"high"}`)})

	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node}))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution, got %d", got)
	}
	updated, err := m.archive.GetNode(node.ID)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if !hasMetadata(updated.Metadata, "priority", `"high"`) {
		t.Errorf("Expected metadata priority=high, got %+v", updated.Metadata)
	}
}

func TestEngine_RelateAction_Edge(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	src := saveTestNode(t, m, repo, "article", "source")
	tgt := saveTestNode(t, m, repo, "article", "target")
	rule := saveActionRule(t, m, repo, core.EventNodeCreated, nil,
		core.RuleAction{Type: core.ActionRelate, Params: json.RawMessage(`{"target_node_id":"` + tgt.ID.String() + `","edge_type":"cite"}`)})

	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: src}))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution, got %d", got)
	}
	edges, _, err := m.archive.ListEdges(repo.ID, core.EdgeListOptions{ListOptions: core.DefaultListOptions()})
	if err != nil {
		t.Fatalf("ListEdges failed: %v", err)
	}
	var found bool
	for _, e := range edges {
		if e.Source == src.ID && e.Target == tgt.ID && e.Type == "cite" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected a cite edge %s -> %s, got %+v", src.ID, tgt.ID, edges)
	}
}

func TestEngine_AnnotateAction(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	node := saveTestNode(t, m, repo, "article", "subject")
	rule := saveActionRule(t, m, repo, core.EventNodeCreated, nil,
		core.RuleAction{Type: core.ActionAnnotate, Params: json.RawMessage(`{"motivation":"commenting","body":"a note","content_type":"text/plain"}`)})

	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node}))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected 1 execution, got %d", got)
	}
	annos, total, err := m.archive.ListAnnotations(repo.ID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListAnnotations failed: %v", err)
	}
	if total != 1 || len(annos) != 1 {
		t.Fatalf("Expected 1 annotation, got %d (total %d)", len(annos), total)
	}
	if annos[0].Edge.Target != node.ID {
		t.Errorf("Expected annotation targeting %s, got %s", node.ID, annos[0].Edge.Target)
	}
	if annos[0].Motivation != "commenting" {
		t.Errorf("Expected motivation 'commenting', got %q", annos[0].Motivation)
	}
}

// TestEngine_FilterMatchesOnUpdateEvent is a regression test for the payload
// normalisation fix: a filter using node.* must match an Updated event whose
// payload nests the entity under updated_node.
func TestEngine_FilterMatchesOnUpdateEvent(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	node := saveTestNode(t, m, repo, "article", "subject")
	f := createTestFilterForRepo(t, m, repo, "node.type", "article")
	rule := saveActionRule(t, m, repo, core.EventNodeUpdated, []uuid.UUID{f.ID},
		core.RuleAction{Type: core.ActionWebhook, Params: json.RawMessage(`{"url":"https://example.test"}`)})

	payload := core.EventNodeUpdatePayload{NodeID: node.ID, UpdatedNode: node}
	testEngine(m, pub).dispatch(eventFor(t, core.EventNodeUpdated, repo.ID, payload))

	if got := executionCount(t, m, rule.ID); got != 1 {
		t.Fatalf("Expected rule to fire on update event with node.* filter, got %d executions", got)
	}
}

func TestEngine_EnrichOnNonNodeEvent(t *testing.T) {
	m, pub := setupTestModule(t)
	repo := createTestRepo(t, m)
	rule := saveActionRule(t, m, repo, core.EventEdgeCreated, nil,
		core.RuleAction{Type: core.ActionEnrich, Params: json.RawMessage(`{"metadata_key":"x","metadata_value":"y"}`)})

	// An edge event carries no node, so the enrich action should error (recorded).
	edge := &core.Edge{ID: core.NewID(), RepoID: repo.ID, Type: "cite", Source: core.NewID(), Target: core.NewID()}
	testEngine(m, pub).dispatch(eventFor(t, core.EventEdgeCreated, repo.ID, core.EventEdgeCreatePayload{Edge: edge}))

	execs, total, err := m.archive.ListRuleExecutions(rule.ID, core.DefaultListOptions())
	if err != nil {
		t.Fatalf("ListRuleExecutions failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("Expected 1 execution recorded, got %d", total)
	}
	if execs[0].ActionsExecuted[0].Result != "error" {
		t.Errorf("Expected enrich on non-node event to be recorded as error, got %q", execs[0].ActionsExecuted[0].Result)
	}
}

func hasMetadata(md []core.Metadata, key, value string) bool {
	for _, m := range md {
		if m.Key == key && string(m.Value) == value {
			return true
		}
	}
	return false
}
