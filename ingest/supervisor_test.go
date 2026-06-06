//go:build sqlite_fts5

package ingest

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// TestSupervisorIngestsNodeEventToSubscribers verifies that a NodeCreated event
// arriving on a Sink's subject is fanned out to every repo-Source subscribed to
// that Sink, that the node lands in the subscriber's repo with correct
// provenance, and that handleSinkEvent returns ackOK.
func TestSupervisorIngestsNodeEventToSubscribers(t *testing.T) {
	a := newTestArchive(t)
	repoB := newTestRepo(t, a)
	sinkID := uuid.New()

	src := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from-a",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	sup := NewSupervisor(a, nil, nil) // consumer nil: we call handleSinkEvent directly

	node := &core.Node{
		ID:          uuid.New(),
		Type:        "note",
		Subject:     "hi",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, uuid.New(), core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.handleSinkEvent(sinkID, data); d != ackOK {
		t.Fatalf("expected ackOK, got %v", d)
	}

	got, err := a.GetNodeBySourceExternalID(src.ID, node.ID.String())
	if err != nil {
		t.Fatalf("node not ingested into subscriber: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != src.ID {
		t.Fatalf("provenance not set: SourceID=%v", got.SourceID)
	}
	if got.ExternalID == nil || *got.ExternalID != node.ID.String() {
		t.Fatalf("external ID not set: ExternalID=%v", got.ExternalID)
	}
	if got.Subject != "hi" {
		t.Fatalf("subject not copied: %q", got.Subject)
	}
	if got.Type != "note" {
		t.Fatalf("type not copied: %q", got.Type)
	}
}

// TestSupervisorDropsHighHopsEvent verifies that an event with Hops >=
// core.MaxEventHops is dropped (ackOK) and never ingested into any subscriber.
func TestSupervisorDropsHighHopsEvent(t *testing.T) {
	a := newTestArchive(t)
	repoB := newTestRepo(t, a)
	sinkID := uuid.New()

	src := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from-a",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	sup := NewSupervisor(a, nil, nil)

	node := &core.Node{
		ID:          uuid.New(),
		Type:        "note",
		Subject:     "hi",
		ContentType: "text/plain",
		Body:        []byte(`"x"`),
	}
	evt, err := core.NewEvent(core.EventNodeCreated, uuid.New(), core.EventNodeCreatePayload{Node: node})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	evt.Hops = core.MaxEventHops // at the limit: must be dropped
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := sup.handleSinkEvent(sinkID, data); d != ackOK {
		t.Fatalf("high-hops event should ackOK (drop), got %v", d)
	}

	// The node must NOT have been ingested.
	_, err = a.GetNodeBySourceExternalID(src.ID, node.ID.String())
	if err == nil {
		t.Fatal("node must not be ingested when event exceeds max hops")
	}
}

// TestSupervisorIngestsEdgeEvent verifies that an EdgeCreated event whose
// Source and Target UUIDs match nodes already ingested (by prior NodeCreated
// events) causes the edge to be resolved and persisted in the subscriber's repo.
func TestSupervisorIngestsEdgeEvent(t *testing.T) {
	a := newTestArchive(t)
	repoB := newTestRepo(t, a)
	sinkID := uuid.New()

	src := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from-a",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	sup := NewSupervisor(a, nil, nil)

	// Ingest two nodes first so edge endpoints can be resolved.
	nodeA := &core.Node{
		ID:          uuid.New(),
		Type:        "note",
		Subject:     "node-a",
		ContentType: "text/plain",
		Body:        []byte(`"a"`),
	}
	nodeB := &core.Node{
		ID:          uuid.New(),
		Type:        "note",
		Subject:     "node-b",
		ContentType: "text/plain",
		Body:        []byte(`"b"`),
	}

	ingestNode := func(n *core.Node) {
		t.Helper()
		evt, err := core.NewEvent(core.EventNodeCreated, uuid.New(), core.EventNodeCreatePayload{Node: n})
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		data, _ := json.Marshal(evt)
		if d := sup.handleSinkEvent(sinkID, data); d != ackOK {
			t.Fatalf("expected ackOK ingesting node, got %v", d)
		}
	}
	ingestNode(nodeA)
	ingestNode(nodeB)

	// Now ingest an edge referencing the two nodes' A-side UUIDs.
	edgeID := uuid.New()
	edge := &core.Edge{
		ID:     edgeID,
		Type:   "reply",
		Source: nodeA.ID,
		Target: nodeB.ID,
		Label:  "in-reply-to",
		Weight: 1.5,
	}
	edgeEvt, err := core.NewEvent(core.EventEdgeCreated, uuid.New(), core.EventEdgeCreatePayload{Edge: edge})
	if err != nil {
		t.Fatalf("NewEvent edge: %v", err)
	}
	edgeData, _ := json.Marshal(edgeEvt)

	if d := sup.handleSinkEvent(sinkID, edgeData); d != ackOK {
		t.Fatalf("expected ackOK ingesting edge, got %v", d)
	}

	// Verify the edge was persisted with resolved endpoints.
	got, err := a.GetEdgeBySourceExternalID(src.ID, edgeID.String())
	if err != nil {
		t.Fatalf("edge not ingested: %v", err)
	}
	if got.Type != "reply" || got.Label != "in-reply-to" || got.Weight != 1.5 {
		t.Fatalf("edge fields not mapped: %+v", got)
	}

	// Confirm the edge endpoints resolved to the ingested nodes' local IDs.
	ingestedA, err := a.GetNodeBySourceExternalID(src.ID, nodeA.ID.String())
	if err != nil {
		t.Fatalf("resolve nodeA: %v", err)
	}
	ingestedB, err := a.GetNodeBySourceExternalID(src.ID, nodeB.ID.String())
	if err != nil {
		t.Fatalf("resolve nodeB: %v", err)
	}
	if got.Source != ingestedA.ID || got.Target != ingestedB.ID {
		t.Fatalf("edge endpoints not resolved: src=%s want %s, tgt=%s want %s",
			got.Source, ingestedA.ID, got.Target, ingestedB.ID)
	}
}

// TestSupervisorDefersUnresolvedEdge verifies that an EdgeCreated event whose
// endpoint nodes have not been ingested yet returns ackRetry (the pipeline
// reports Deferred), so the message is redelivered until a later-arriving node
// resolves the reference. The edge must not be persisted in the meantime.
func TestSupervisorDefersUnresolvedEdge(t *testing.T) {
	a := newTestArchive(t)
	repoB := newTestRepo(t, a)
	sinkID := uuid.New()

	src := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from-a",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(src); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	sup := NewSupervisor(a, nil, nil)

	edgeID := uuid.New()
	edge := &core.Edge{
		ID:     edgeID,
		Type:   "reply",
		Source: uuid.New(), // never ingested
		Target: uuid.New(), // never ingested
	}
	evt, err := core.NewEvent(core.EventEdgeCreated, uuid.New(), core.EventEdgeCreatePayload{Edge: edge})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, _ := json.Marshal(evt)

	if d := sup.handleSinkEvent(sinkID, data); d != ackRetry {
		t.Fatalf("unresolved edge should ackRetry (deferred), got %v", d)
	}

	if _, err := a.GetEdgeBySourceExternalID(src.ID, edgeID.String()); err == nil {
		t.Fatalf("deferred edge must not be persisted")
	}
}

// TestSupervisorMalformedMessageTerminates verifies that an unparseable message
// body returns ackTerm (poison — redelivery cannot help).
func TestSupervisorMalformedMessageTerminates(t *testing.T) {
	a := newTestArchive(t)
	sup := NewSupervisor(a, nil, nil)
	sinkID := uuid.New()

	if d := sup.handleSinkEvent(sinkID, []byte("not-json")); d != ackTerm {
		t.Fatalf("malformed message should ackTerm, got %v", d)
	}
}
