package realtime

import (
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func nodeCreatedEvent(t *testing.T, repoID, nodeID uuid.UUID, nodeType string) *core.Event {
	t.Helper()
	ev, err := core.NewEvent(core.EventNodeCreated, repoID, core.EventNodeCreatePayload{
		Node: &core.Node{ID: nodeID, RepoID: repoID, Type: nodeType},
	})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	return ev
}

func nodeDeletedEvent(t *testing.T, repoID, nodeID uuid.UUID) *core.Event {
	t.Helper()
	ev, err := core.NewEvent(core.EventNodeDeleted, repoID, core.EventNodeDeletePayload{NodeID: nodeID})
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	return ev
}

func TestMatchEvent_RepoChannel(t *testing.T) {
	repo := uuid.New()
	other := uuid.New()
	sub := &subscription{kind: channelRepoEvents, repoID: repo}

	if !sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("expected event in same repo to match")
	}
	if sub.matchEvent(nodeCreatedEvent(t, other, uuid.New(), "article")) {
		t.Error("expected event in other repo to not match")
	}
}

func TestMatchEvent_NodeChannel(t *testing.T) {
	repo := uuid.New()
	node := uuid.New()
	sub := &subscription{kind: channelNode, repoID: repo, entityID: node}

	if !sub.matchEvent(nodeCreatedEvent(t, repo, node, "article")) {
		t.Error("expected create of the subscribed node to match")
	}
	if !sub.matchEvent(nodeDeletedEvent(t, repo, node)) {
		t.Error("expected delete of the subscribed node to match")
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("expected create of a different node to not match")
	}
}

func TestMatchEvent_EventTypeFilter(t *testing.T) {
	repo := uuid.New()
	sub := &subscription{
		kind:   channelRepoEvents,
		repoID: repo,
		filter: &subFilter{EventType: []string{"NodeDeleted"}},
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("NodeCreated should be filtered out")
	}
	if !sub.matchEvent(nodeDeletedEvent(t, repo, uuid.New())) {
		t.Error("NodeDeleted should pass")
	}
}

func TestMatchEvent_NodeTypeFilterBestEffort(t *testing.T) {
	repo := uuid.New()
	sub := &subscription{
		kind:   channelRepoEvents,
		repoID: repo,
		filter: &subFilter{NodeType: []string{"article"}},
	}
	if !sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
		t.Error("article create should pass node_type filter")
	}
	if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "note")) {
		t.Error("note create should be excluded by node_type filter")
	}
	if !sub.matchEvent(nodeDeletedEvent(t, repo, uuid.New())) {
		t.Error("delete should pass since node_type cannot be evaluated")
	}
}

func TestMatchEvent_StubChannelNeverMatches(t *testing.T) {
	repo := uuid.New()
	for _, k := range []channelKind{channelRuleActions, channelJob, channelSync} {
		sub := &subscription{kind: k, repoID: repo}
		if sub.matchEvent(nodeCreatedEvent(t, repo, uuid.New(), "article")) {
			t.Errorf("stub channel kind %v should never match", k)
		}
	}
}
