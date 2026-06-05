package realtime

import (
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// fakeResolver is a hand-rolled refResolver for resolveChannel tests. It maps a
// repo ref to a repository and an entity ref to a (uuid, entityType) pair, and
// returns ErrorNotFound for anything it does not know.
type fakeResolver struct {
	repos    map[string]*core.Repository
	entities map[string]struct {
		id  uuid.UUID
		typ string
	}
}

func (f fakeResolver) ResolveRepoRef(_ uuid.UUID, ref string) (*core.Repository, error) {
	if r, ok := f.repos[ref]; ok {
		return r, nil
	}
	return nil, &core.ErrorNotFound{}
}

func (f fakeResolver) ResolveRef(_ uuid.UUID, ref string) (uuid.UUID, string, error) {
	if e, ok := f.entities[ref]; ok {
		return e.id, e.typ, nil
	}
	return uuid.Nil, "", &core.ErrorNotFound{}
}

func newFakeResolver(repoRef string, repoID uuid.UUID) fakeResolver {
	return fakeResolver{
		repos: map[string]*core.Repository{
			repoRef: {ID: repoID, Slug: repoRef},
		},
		entities: map[string]struct {
			id  uuid.UUID
			typ string
		}{},
	}
}

func TestResolveChannel_RepoEvents(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)

	pc, err := parseChannel("repos.research.events")
	if err != nil {
		t.Fatalf("parseChannel: %v", err)
	}
	sub, err := resolveChannel(pc, nil, r)
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if sub.channel != "repos.research.events" {
		t.Errorf("channel: got %q", sub.channel)
	}
	if sub.kind != channelRepoEvents {
		t.Errorf("kind: got %v", sub.kind)
	}
	if sub.repoID != repoID {
		t.Errorf("repoID: got %v, want %v", sub.repoID, repoID)
	}
}

func TestResolveChannel_NodeResolvesEntity(t *testing.T) {
	repoID, nodeID := uuid.New(), uuid.New()
	r := newFakeResolver("research", repoID)
	r.entities["NODE-1"] = struct {
		id  uuid.UUID
		typ string
	}{nodeID, "node"}

	pc, _ := parseChannel("repos.research.nodes.NODE-1")
	sub, err := resolveChannel(pc, nil, r)
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if sub.repoID != repoID || sub.entityID != nodeID {
		t.Errorf("ids: repo=%v entity=%v", sub.repoID, sub.entityID)
	}
}

func TestResolveChannel_NodeRejectsWrongEntityType(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)
	r.entities["THREAD-1"] = struct {
		id  uuid.UUID
		typ string
	}{uuid.New(), "thread"}

	pc, _ := parseChannel("repos.research.nodes.THREAD-1")
	if _, err := resolveChannel(pc, nil, r); err == nil {
		t.Fatal("expected error when ref resolves to a non-node entity")
	}
}

func TestResolveChannel_UnknownRepoFails(t *testing.T) {
	r := newFakeResolver("research", uuid.New())
	pc, _ := parseChannel("repos.unknown.events")
	if _, err := resolveChannel(pc, nil, r); err == nil {
		t.Fatal("expected error for unknown repo ref")
	}
}

func TestResolveChannel_SyncNeedsNoResolution(t *testing.T) {
	r := newFakeResolver("research", uuid.New())
	pc, _ := parseChannel("sync")
	sub, err := resolveChannel(pc, nil, r)
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if sub.kind != channelSync {
		t.Errorf("kind: got %v", sub.kind)
	}
}

func TestResolveChannel_AttachesFilter(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)
	f := &subFilter{EventType: []string{"NodeCreated"}}

	pc, _ := parseChannel("repos.research.events")
	sub, err := resolveChannel(pc, f, r)
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if sub.filter != f {
		t.Error("expected filter to be attached to the subscription")
	}
}
