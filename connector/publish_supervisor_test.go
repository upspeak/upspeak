//go:build sqlite_fts5

package connector

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// TestPublishSupervisorRepublishesMatchingNode verifies that a NodeCreated event
// whose node passes a Sink's empty filter chain (publish everything) is
// republished to the correct SINK_EVENTS subject.
func TestPublishSupervisorRepublishesMatchingNode(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)
	sink := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	} // empty FilterIDs → publish everything
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil) // consumer nil: call dispatch directly

	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "s",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	if len(pub.published) != 1 || pub.published[0].Subject != core.SinkSubject(sink.ID, core.EventNodeCreated) {
		t.Fatalf("expected republish on sink subject, got %+v", pub.published)
	}
}

// TestPublishSupervisorDropsFilteredNode verifies that a node failing a Sink's
// filter is not republished, and dispatch still returns ackOK.
func TestPublishSupervisorDropsFilteredNode(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Filter: type == "article" (our test node is type "note", so it will not match).
	cond := core.Condition{
		Field: "type",
		Op:    core.OpEq,
		Value: json.RawMessage(`"article"`),
	}
	f := &core.Filter{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Name:       "articles-only",
		Mode:       core.FilterModeAll,
		Conditions: []core.Condition{cond},
		CreatedBy:  repo.OwnerID,
	}
	if err := m.archive.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	sink := &core.Sink{
		ID:              core.NewID(),
		RepoID:          repo.ID,
		Name:            "filtered-out",
		Connector:       core.ConnectorRepo,
		Status:          core.StatusActive,
		FilterIDs:       []uuid.UUID{f.ID},
		FilterChainMode: core.FilterModeAll,
		CreatedBy:       repo.OwnerID,
	}
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note", // does NOT match the "article" filter
		Subject:     "s",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	if len(pub.published) != 0 {
		t.Fatalf("expected no republish, got %+v", pub.published)
	}
}

// TestPublishSupervisorEdgeRequiresBothEndpoints verifies the two-endpoint AND
// rule: an EdgeCreated is only republished when BOTH source and target nodes pass
// the Sink filter.
func TestPublishSupervisorEdgeRequiresBothEndpoints(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Filter: type == "keep".
	cond := core.Condition{
		Field: "type",
		Op:    core.OpEq,
		Value: json.RawMessage(`"keep"`),
	}
	f := &core.Filter{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Name:       "keep-only",
		Mode:       core.FilterModeAll,
		Conditions: []core.Condition{cond},
		CreatedBy:  repo.OwnerID,
	}
	if err := m.archive.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	sink := &core.Sink{
		ID:              core.NewID(),
		RepoID:          repo.ID,
		Name:            "keep-only-sink",
		Connector:       core.ConnectorRepo,
		Status:          core.StatusActive,
		FilterIDs:       []uuid.UUID{f.ID},
		FilterChainMode: core.FilterModeAll,
		CreatedBy:       repo.OwnerID,
	}
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	// Persist both nodes so GetNode calls in resolvePublish succeed.
	nodeKeep := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "keep",
		Subject:     "keeper",
		ContentType: "text/plain",
		Body:        []byte(`""`),
		CreatedBy:   repo.OwnerID,
	}
	if err := m.archive.SaveNode(nodeKeep); err != nil {
		t.Fatalf("SaveNode keep: %v", err)
	}
	nodeDrop := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "drop",
		Subject:     "dropper",
		ContentType: "text/plain",
		Body:        []byte(`""`),
		CreatedBy:   repo.OwnerID,
	}
	if err := m.archive.SaveNode(nodeDrop); err != nil {
		t.Fatalf("SaveNode drop: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	// Case 1: one endpoint fails → NOT republished.
	edge1 := &core.Edge{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Source: nodeKeep.ID,
		Target: nodeDrop.ID, // drops
		Type:   "relates",
	}
	evt1, err := core.NewEvent(core.EventEdgeCreated, repo.ID, core.EventEdgeCreatePayload{Edge: edge1})
	if err != nil {
		t.Fatalf("NewEvent edge1: %v", err)
	}
	data1, err := json.Marshal(evt1)
	if err != nil {
		t.Fatalf("json.Marshal edge1: %v", err)
	}
	if d := sup.dispatch(data1); d != ackOK {
		t.Fatalf("case1: want ackOK, got %v", d)
	}
	if len(pub.published) != 0 {
		t.Fatalf("case1: expected no republish when one endpoint fails, got %+v", pub.published)
	}

	// Case 2: both endpoints pass → republished.
	nodeKeep2 := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "keep",
		Subject:     "keeper2",
		ContentType: "text/plain",
		Body:        []byte(`""`),
		CreatedBy:   repo.OwnerID,
	}
	if err := m.archive.SaveNode(nodeKeep2); err != nil {
		t.Fatalf("SaveNode keep2: %v", err)
	}
	edge2 := &core.Edge{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Source: nodeKeep.ID,
		Target: nodeKeep2.ID,
		Type:   "relates",
	}
	evt2, err := core.NewEvent(core.EventEdgeCreated, repo.ID, core.EventEdgeCreatePayload{Edge: edge2})
	if err != nil {
		t.Fatalf("NewEvent edge2: %v", err)
	}
	data2, err := json.Marshal(evt2)
	if err != nil {
		t.Fatalf("json.Marshal edge2: %v", err)
	}
	if d := sup.dispatch(data2); d != ackOK {
		t.Fatalf("case2: want ackOK, got %v", d)
	}
	if len(pub.published) != 1 || pub.published[0].Subject != core.SinkSubject(sink.ID, core.EventEdgeCreated) {
		t.Fatalf("case2: expected republish on sink edge subject, got %+v", pub.published)
	}
}

// TestPublishSupervisorDropsHighHopsEvent verifies that an event whose Hops
// count is at or above core.MaxEventHops is silently dropped (ackOK, no publish).
func TestPublishSupervisorDropsHighHopsEvent(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)
	sink := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	}
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "high-hops",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	evt.Hops = core.MaxEventHops // at the limit: must be dropped

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK for high-hops, got %v", d)
	}
	if len(pub.published) != 0 {
		t.Fatalf("expected no publish for high-hops event, got %+v", pub.published)
	}
}

// TestPublishSupervisorPropagatesHops verifies that the republished event carries
// Hops = inbound.Hops + 1, bounding downstream reaction chains.
func TestPublishSupervisorPropagatesHops(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)
	sink := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	}
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "s",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	evt.Hops = 2 // inbound hops; should become 3 in the outbound event

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub.published))
	}

	var outEvt core.Event
	if err := json.Unmarshal(pub.published[0].Data, &outEvt); err != nil {
		t.Fatalf("unmarshal outbound event: %v", err)
	}
	if outEvt.Hops != 3 {
		t.Fatalf("expected Hops=3 in outbound event, got %d", outEvt.Hops)
	}
}

// TestPublishSupervisorNormalisesNodePatched verifies that a NodePatched event is
// resolved against the stored node and republished as a NodeUpdated event (the
// patch payload alone does not carry the full node a subscriber needs).
func TestPublishSupervisorNormalisesNodePatched(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)
	sink := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	} // empty filter → publish everything
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	// Persist the node so resolvePublish's GetNode succeeds.
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "patched",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
		CreatedBy:   repo.OwnerID,
	}
	if err := m.archive.SaveNode(node); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	evt, err := core.NewEvent(core.EventNodePatched, repo.ID,
		core.EventNodePatchPayload{NodeID: node.ID, Fields: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	// Republished under the normalised NodeUpdated type, not NodePatched.
	if len(pub.published) != 1 || pub.published[0].Subject != core.SinkSubject(sink.ID, core.EventNodeUpdated) {
		t.Fatalf("expected republish as NodeUpdated, got %+v", pub.published)
	}
	var out core.Event
	if err := json.Unmarshal(pub.published[0].Data, &out); err != nil {
		t.Fatalf("unmarshal outbound: %v", err)
	}
	var p core.EventNodeUpdatePayload
	if err := json.Unmarshal(out.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.UpdatedNode == nil || p.UpdatedNode.ID != node.ID {
		t.Fatalf("normalised payload missing full node: %+v", p)
	}
}

// TestPublishSupervisorSkipsUnresolvableTarget verifies that an event whose
// referenced entity is absent (e.g. deleted before publish) is acked with no
// republish, even when a matching Sink exists.
func TestPublishSupervisorSkipsUnresolvableTarget(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)
	sink := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	}
	if err := m.archive.SaveSink(sink); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	// ThreadNodeAdded referencing a node that was never persisted.
	evt, err := core.NewEvent(core.EventThreadNodeAdded, repo.ID,
		core.EventThreadNodePayload{ThreadID: core.NewID(), NodeID: core.NewID()})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK for unresolvable target, got %v", d)
	}
	if len(pub.published) != 0 {
		t.Fatalf("expected no republish for unresolvable target, got %+v", pub.published)
	}
}

// TestPublishSupervisorRepublishesPerMatchingSink verifies that, with multiple
// Sinks, only those whose filter chain the entity passes receive a republish.
func TestPublishSupervisorRepublishesPerMatchingSink(t *testing.T) {
	m := setupTestModule(t)
	repo := createTestRepo(t, m)

	// Filter that the test node (type "note") will NOT match.
	f := &core.Filter{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Name:       "articles-only",
		Mode:       core.FilterModeAll,
		Conditions: []core.Condition{{Field: "type", Op: core.OpEq, Value: json.RawMessage(`"article"`)}},
		CreatedBy:  repo.OwnerID,
	}
	if err := m.archive.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	matching := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Name:      "matching",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repo.OwnerID,
	} // empty filter → matches
	if err := m.archive.SaveSink(matching); err != nil {
		t.Fatalf("SaveSink matching: %v", err)
	}
	nonMatching := &core.Sink{
		ID:              core.NewID(),
		RepoID:          repo.ID,
		Name:            "non-matching",
		Connector:       core.ConnectorRepo,
		Status:          core.StatusActive,
		FilterIDs:       []uuid.UUID{f.ID},
		FilterChainMode: core.FilterModeAll,
		CreatedBy:       repo.OwnerID,
	}
	if err := m.archive.SaveSink(nonMatching); err != nil {
		t.Fatalf("SaveSink non-matching: %v", err)
	}

	pub := &mockPublisher{}
	sup := NewPublishSupervisor(m.archive, pub, nil)

	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "s",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repo.ID, core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.dispatch(data); d != ackOK {
		t.Fatalf("want ackOK, got %v", d)
	}
	if len(pub.published) != 1 || pub.published[0].Subject != core.SinkSubject(matching.ID, core.EventNodeCreated) {
		t.Fatalf("expected exactly one republish on the matching sink, got %+v", pub.published)
	}
}
