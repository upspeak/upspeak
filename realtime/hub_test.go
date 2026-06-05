package realtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

func TestHub_DispatchDeliversToMatchingConnection(t *testing.T) {
	h := newHub()
	repo := uuid.New()

	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: repo})
	if !h.add(c) {
		t.Fatal("expected add to succeed")
	}

	ev := nodeCreatedEvent(t, repo, uuid.New(), "article")
	h.dispatch(ev)

	select {
	case frame := <-c.out:
		var msg outboundEvent
		if err := json.Unmarshal(frame, &msg); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		if msg.Channel != "repos.research.events" {
			t.Errorf("channel: got %q", msg.Channel)
		}
		if msg.Event.Type != core.EventNodeCreated {
			t.Errorf("type: got %q", msg.Event.Type)
		}
	default:
		t.Fatal("expected a frame to be enqueued")
	}
}

func TestHub_DispatchSkipsNonMatching(t *testing.T) {
	h := newHub()
	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: uuid.New()})
	h.add(c)

	h.dispatch(nodeCreatedEvent(t, uuid.New(), uuid.New(), "article")) // different repo
	if len(c.out) != 0 {
		t.Fatal("expected no frame for non-matching repo")
	}
}

func TestHub_PerIdentityConnectionCap(t *testing.T) {
	h := newHub()
	for i := 0; i < maxConnsPerIdentity; i++ {
		if !h.add(newConnection("c", "local")) {
			t.Fatalf("add %d should succeed", i)
		}
	}
	if h.add(newConnection("over", "local")) {
		t.Fatal("expected the cap to reject the extra connection")
	}
}

func TestHub_RemoveFreesIdentitySlot(t *testing.T) {
	h := newHub()
	conns := make([]*connection, maxConnsPerIdentity)
	for i := range conns {
		conns[i] = newConnection("c", "local")
		h.add(conns[i])
	}
	h.remove(conns[0])
	if !h.add(newConnection("new", "local")) {
		t.Fatal("expected a freed slot to allow a new connection")
	}
}

func TestHub_RunDispatchesIngestedEvents(t *testing.T) {
	h := newHub()
	repo := uuid.New()
	c := newConnection("c1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events", kind: channelRepoEvents, repoID: repo})
	h.add(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.run(ctx)

	h.ingest(nodeCreatedEvent(t, repo, uuid.New(), "article"))

	select {
	case <-c.out:
	case <-time.After(time.Second):
		t.Fatal("expected ingested event to be dispatched")
	}
}
