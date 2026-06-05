package realtime

import (
	"testing"

	"github.com/google/uuid"
)

func TestApplyClientMessage_SubscribeAddsSubscription(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)
	c := newConnection("c1", "local")

	msg := clientMessage{Action: "subscribe", Channel: "repos.research.events"}
	if errMsg := applyClientMessage(c, msg, r); errMsg != nil {
		t.Fatalf("unexpected error: %+v", errMsg)
	}
	if len(c.snapshotSubs()) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(c.snapshotSubs()))
	}
}

func TestApplyClientMessage_SubscribeInvalidChannel(t *testing.T) {
	r := newFakeResolver("research", uuid.New())
	c := newConnection("c1", "local")

	msg := clientMessage{Action: "subscribe", Channel: "not.a.valid.channel.shape.xyz"}
	errMsg := applyClientMessage(c, msg, r)
	if errMsg == nil || errMsg.Code != codeInvalidChannel {
		t.Fatalf("expected invalid_channel error, got %+v", errMsg)
	}
}

func TestApplyClientMessage_SubscribeUnresolvableRepo(t *testing.T) {
	r := newFakeResolver("research", uuid.New())
	c := newConnection("c1", "local")

	msg := clientMessage{Action: "subscribe", Channel: "repos.ghost.events"}
	errMsg := applyClientMessage(c, msg, r)
	if errMsg == nil || errMsg.Code != codeInvalidChannel {
		t.Fatalf("expected invalid_channel error, got %+v", errMsg)
	}
}

func TestApplyClientMessage_SubscribeLimitReached(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)
	c := newConnection("c1", "local")
	// Fill to the subscription cap with distinct valid channels.
	for i := 0; i < maxSubscriptions; i++ {
		c.subs[uuid.New().String()] = &subscription{channel: uuid.New().String()}
	}

	msg := clientMessage{Action: "subscribe", Channel: "repos.research.events"}
	errMsg := applyClientMessage(c, msg, r)
	if errMsg == nil || errMsg.Code != codeSubscriptionLimit {
		t.Fatalf("expected subscription_limit error, got %+v", errMsg)
	}
}

func TestApplyClientMessage_Unsubscribe(t *testing.T) {
	repoID := uuid.New()
	r := newFakeResolver("research", repoID)
	c := newConnection("c1", "local")
	_ = applyClientMessage(c, clientMessage{Action: "subscribe", Channel: "repos.research.events"}, r)

	errMsg := applyClientMessage(c, clientMessage{Action: "unsubscribe", Channel: "repos.research.events"}, r)
	if errMsg != nil {
		t.Fatalf("unexpected error: %+v", errMsg)
	}
	if len(c.snapshotSubs()) != 0 {
		t.Fatal("expected subscription to be removed")
	}
}

func TestApplyClientMessage_UnknownAction(t *testing.T) {
	r := newFakeResolver("research", uuid.New())
	c := newConnection("c1", "local")

	errMsg := applyClientMessage(c, clientMessage{Action: "bogus", Channel: "repos.research.events"}, r)
	if errMsg == nil || errMsg.Code != codeInvalidChannel {
		t.Fatalf("expected error for unknown action, got %+v", errMsg)
	}
}
