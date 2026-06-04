package realtime

import (
	"sync"
	"sync/atomic"
)

// connection holds the server-side state for one WebSocket client: a bounded
// outbound buffer and the set of active subscriptions. The WebSocket read and
// write loops (see handlers.go) drive it; this type contains no socket logic so
// it can be tested in isolation.
type connection struct {
	id       string      // unique connection id, for logging
	identity string      // auth identity, for the per-identity connection cap
	out      chan []byte // buffered outbound frames

	mu   sync.Mutex
	subs map[string]*subscription // keyed by raw channel string

	dropped atomic.Bool // set when frames were dropped; cleared by takeDropped
}

// newConnection creates a connection with an empty subscription set and a
// buffer sized to outboundBufferSize.
func newConnection(id, identity string) *connection {
	return &connection{
		id:       id,
		identity: identity,
		out:      make(chan []byte, outboundBufferSize),
		subs:     make(map[string]*subscription),
	}
}

// addSubscription registers a subscription. Re-subscribing to an existing
// channel replaces it without counting against the limit. A new channel beyond
// maxSubscriptions is rejected.
func (c *connection) addSubscription(sub *subscription) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.subs[sub.channel]; !exists && len(c.subs) >= maxSubscriptions {
		return errSubscriptionLimit
	}
	c.subs[sub.channel] = sub
	return nil
}

// removeSubscription removes the subscription for the given channel, if present.
func (c *connection) removeSubscription(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, channel)
}

// snapshotSubs returns a copy of the current subscriptions so the hub can match
// without holding the connection lock during dispatch.
func (c *connection) snapshotSubs() []*subscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*subscription, 0, len(c.subs))
	for _, s := range c.subs {
		out = append(out, s)
	}
	return out
}

// enqueue queues a frame for delivery. When the buffer is full it drops the
// oldest queued frame to make room for the newest and records that a drop
// occurred, so a single messages_dropped notice can be sent.
func (c *connection) enqueue(frame []byte) {
	select {
	case c.out <- frame:
		return
	default:
	}
	// Buffer full: drop one old frame, then enqueue the new one.
	select {
	case <-c.out:
	default:
	}
	select {
	case c.out <- frame:
	default:
	}
	c.dropped.Store(true)
}

// takeDropped reports whether frames were dropped since the last call, clearing
// the flag.
func (c *connection) takeDropped() bool {
	return c.dropped.Swap(false)
}
