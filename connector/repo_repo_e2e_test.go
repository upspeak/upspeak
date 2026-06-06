//go:build sqlite_fts5

package connector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/ingest"
)

// oneShotConsumer delivers a single pre-captured message to a supervisor's Run
// loop exactly once, then reports ErrFetchTimeout so the loop idles until the
// test cancels its context. It bridges the publish supervisor's output into the
// ingest supervisor without a live NATS connection.
type oneShotConsumer struct {
	msg       *app.Msg
	delivered bool
}

func (c *oneShotConsumer) Fetch(maxMsgs int, timeout time.Duration) ([]*app.Msg, error) {
	if c.delivered {
		return nil, app.ErrFetchTimeout
	}
	c.delivered = true
	return []*app.Msg{c.msg}, nil
}

// mkRepo persists a repository with the given slug for use in e2e wiring.
func mkRepo(t *testing.T, m *Module, slug string) *core.Repository {
	t.Helper()
	repo := &core.Repository{ID: core.NewID(), Slug: slug, Name: slug, OwnerID: defaultOwnerID}
	if err := m.archive.SaveRepository(repo); err != nil {
		t.Fatalf("SaveRepository %s: %v", slug, err)
	}
	return repo
}

// TestRepoToRepoEndToEnd wires the publish supervisor to the ingest supervisor
// in-process: a node created in repoA, with an active repo Sink and a repoB
// repo-Source subscribing to it, is republished to SINK_EVENTS by the publish
// supervisor and lands in repoB with provenance via the ingest supervisor.
func TestRepoToRepoEndToEnd(t *testing.T) {
	m := setupTestModule(t)
	a := m.archive

	repoA := mkRepo(t, m, "repo-a")
	repoB := mkRepo(t, m, "repo-b")

	// repoA's curated publication endpoint (empty filter → publish everything).
	sinkA := &core.Sink{
		ID:        core.NewID(),
		RepoID:    repoA.ID,
		Name:      "A out",
		Connector: core.ConnectorRepo,
		Status:    core.StatusActive,
		CreatedBy: repoA.OwnerID,
	}
	if err := a.SaveSink(sinkA); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	// repoB subscribes to sinkA.
	sourceB := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from A",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkA.ID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(sourceB); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	// Publish side: a NodeCreated event in repoA.
	pub := &mockPublisher{}
	pubSup := NewPublishSupervisor(a, pub, nil) // consumer nil: dispatch is called directly

	aNode := &core.Node{
		ID:          core.NewID(),
		RepoID:      repoA.ID,
		Type:        "note",
		Subject:     "hello",
		ContentType: "text/plain",
		Body:        []byte(`"hi from A"`),
		CreatedBy:   repoA.OwnerID,
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repoA.ID, core.EventNodeCreatePayload{Node: aNode})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := pubSup.dispatch(data); d != ackOK {
		t.Fatalf("publish dispatch: want ackOK, got %v", d)
	}
	wantSubject := core.SinkSubject(sinkA.ID, core.EventNodeCreated)
	if len(pub.published) != 1 || pub.published[0].Subject != wantSubject {
		t.Fatalf("expected republish on %s, got %+v", wantSubject, pub.published)
	}

	// Ingest side: feed the captured SINK_EVENTS message into the ingest
	// supervisor via its real Run loop. The message acks once the node is
	// persisted, so the test proceeds deterministically.
	acked := make(chan struct{})
	noop := func() error { return nil }
	msg := app.NewMsg(pub.published[0].Subject, pub.published[0].Data,
		func() error { close(acked); return nil }, noop, noop, noop)
	ingestSup := ingest.NewSupervisor(a, nil, &oneShotConsumer{msg: msg})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ingestSup.Run(ctx)

	select {
	case <-acked:
	case <-time.After(2 * time.Second):
		t.Fatal("ingest supervisor did not acknowledge within 2s")
	}

	// The node from repoA must now exist in repoB, keyed by its A-side ID as the
	// external id, with provenance pointing at sourceB.
	got, err := a.GetNodeBySourceExternalID(sourceB.ID, aNode.ID.String())
	if err != nil {
		t.Fatalf("node did not reach repo B: %v", err)
	}
	if got.RepoID != repoB.ID {
		t.Fatalf("ingested node in wrong repo: got %s, want %s", got.RepoID, repoB.ID)
	}
	if got.SourceID == nil || *got.SourceID != sourceB.ID {
		t.Fatalf("provenance not set on ingested node")
	}
	if got.Subject != "hello" {
		t.Fatalf("content not carried across: subject %q", got.Subject)
	}
	if string(got.Body) != string(aNode.Body) {
		t.Fatalf("body not carried across: got %q, want %q", got.Body, aNode.Body)
	}
	if got.ContentType != aNode.ContentType {
		t.Fatalf("content type not carried across: got %q, want %q", got.ContentType, aNode.ContentType)
	}
}

// TestRepoToRepoEndToEnd_FilteredNodeBlocked verifies that a node failing the
// Sink's filter chain is never republished and therefore never reaches the
// subscriber, enforcing publication control at the repo boundary.
func TestRepoToRepoEndToEnd_FilteredNodeBlocked(t *testing.T) {
	m := setupTestModule(t)
	a := m.archive

	repoA := mkRepo(t, m, "repo-a")
	repoB := mkRepo(t, m, "repo-b")

	// A filter that only passes type == "article".
	f := &core.Filter{
		ID:         core.NewID(),
		RepoID:     repoA.ID,
		Name:       "articles-only",
		Mode:       core.FilterModeAll,
		Conditions: []core.Condition{{Field: "type", Op: core.OpEq, Value: json.RawMessage(`"article"`)}},
		CreatedBy:  repoA.OwnerID,
	}
	if err := a.SaveFilter(f); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	sinkA := &core.Sink{
		ID:              core.NewID(),
		RepoID:          repoA.ID,
		Name:            "A out (filtered)",
		Connector:       core.ConnectorRepo,
		Status:          core.StatusActive,
		FilterIDs:       []uuid.UUID{f.ID},
		FilterChainMode: core.FilterModeAll,
		CreatedBy:       repoA.OwnerID,
	}
	if err := a.SaveSink(sinkA); err != nil {
		t.Fatalf("SaveSink: %v", err)
	}

	sourceB := &core.Source{
		ID:        core.NewID(),
		RepoID:    repoB.ID,
		Name:      "from A",
		Connector: core.ConnectorRepo,
		Config:    map[string]any{"sink_id": sinkA.ID.String()},
		Status:    core.StatusActive,
		CreatedBy: repoB.OwnerID,
	}
	if err := a.SaveSource(sourceB); err != nil {
		t.Fatalf("SaveSource: %v", err)
	}

	pub := &mockPublisher{}
	pubSup := NewPublishSupervisor(a, pub, nil)

	// A "note" node fails the "article" filter.
	aNode := &core.Node{
		ID:          core.NewID(),
		RepoID:      repoA.ID,
		Type:        "note",
		Subject:     "blocked",
		ContentType: "text/plain",
		Body:        []byte(`"secret"`),
		CreatedBy:   repoA.OwnerID,
	}
	evt, err := core.NewEvent(core.EventNodeCreated, repoA.ID, core.EventNodeCreatePayload{Node: aNode})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if d := pubSup.dispatch(data); d != ackOK {
		t.Fatalf("publish dispatch: want ackOK, got %v", d)
	}
	if len(pub.published) != 0 {
		t.Fatalf("filtered node must not be republished, got %+v", pub.published)
	}

	// Nothing was published, so nothing can reach repo B.
	if _, err := a.GetNodeBySourceExternalID(sourceB.ID, aNode.ID.String()); err == nil {
		t.Fatalf("filtered node must not reach repo B")
	}
}
