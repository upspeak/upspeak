package realtime

import (
	"testing"

	"github.com/google/uuid"
)

func TestConnection_AddSubscriptionLimit(t *testing.T) {
	c := newConnection("conn-1", "local")
	for i := 0; i < maxSubscriptions; i++ {
		ch := "repos.research.nodes.NODE-" + uuid.New().String()
		if err := c.addSubscription(&subscription{channel: ch}); err != nil {
			t.Fatalf("unexpected error adding subscription %d: %v", i, err)
		}
	}
	// The 11th distinct subscription must be rejected.
	err := c.addSubscription(&subscription{channel: "repos.research.events"})
	if err == nil {
		t.Fatal("expected subscription limit error, got nil")
	}
}

func TestConnection_AddSubscriptionReplaceSameChannel(t *testing.T) {
	c := newConnection("conn-1", "local")
	for i := 0; i < maxSubscriptions; i++ {
		ch := "repos.research.nodes.NODE-" + uuid.New().String()
		if err := c.addSubscription(&subscription{channel: ch}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	// Re-subscribing to an already-subscribed channel must not exceed the limit.
	existing := c.snapshotSubs()[0].channel
	if err := c.addSubscription(&subscription{channel: existing}); err != nil {
		t.Fatalf("re-subscribe to existing channel should succeed, got %v", err)
	}
}

func TestConnection_EnqueueOverflowDropsAndFlags(t *testing.T) {
	c := newConnection("conn-1", "local")
	// Fill the buffer beyond capacity.
	for i := 0; i < outboundBufferSize+10; i++ {
		c.enqueue([]byte("x"))
	}
	if len(c.out) != outboundBufferSize {
		t.Fatalf("expected buffer to be at capacity %d, got %d", outboundBufferSize, len(c.out))
	}
	if !c.takeDropped() {
		t.Fatal("expected dropped flag to be set after overflow")
	}
	// takeDropped clears the flag.
	if c.takeDropped() {
		t.Fatal("expected dropped flag to be cleared after takeDropped")
	}
}

func TestConnection_RemoveSubscription(t *testing.T) {
	c := newConnection("conn-1", "local")
	_ = c.addSubscription(&subscription{channel: "repos.research.events"})
	c.removeSubscription("repos.research.events")
	if len(c.snapshotSubs()) != 0 {
		t.Fatal("expected subscription to be removed")
	}
}
