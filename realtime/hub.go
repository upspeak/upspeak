package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/upspeak/upspeak/core"
)

// hub is the in-process fan-out core. It holds the connection registry and
// dispatches each ingested event to every connection with a matching
// subscription. Ingestion is buffered so events can arrive (via the framework's
// repo.*.events.> subscription) before the dispatch loop starts.
type hub struct {
	logger   *slog.Logger
	ingestCh chan *core.Event

	mu         sync.RWMutex
	conns      map[*connection]struct{}
	byIdentity map[string]int
}

// newHub creates a hub with an empty registry and a buffered ingest channel.
func newHub(logger *slog.Logger) *hub {
	return &hub{
		logger:     logger,
		ingestCh:   make(chan *core.Event, ingestBufferSize),
		conns:      make(map[*connection]struct{}),
		byIdentity: make(map[string]int),
	}
}

// ingest hands an event to the dispatch loop. It never blocks: if the ingest
// backlog is full the event is dropped (logged), trading completeness for
// liveness on a real-time stream.
func (h *hub) ingest(ev *core.Event) {
	select {
	case h.ingestCh <- ev:
	default:
		h.logger.Warn("realtime: ingest buffer full, dropping event", "type", ev.Type)
	}
}

// add registers a connection, enforcing the per-identity cap. It returns false
// when the identity already holds maxConnsPerIdentity connections.
func (h *hub) add(c *connection) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byIdentity[c.identity] >= maxConnsPerIdentity {
		return false
	}
	h.conns[c] = struct{}{}
	h.byIdentity[c.identity]++
	return true
}

// remove unregisters a connection and frees its identity slot.
func (h *hub) remove(c *connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.conns[c]; !ok {
		return
	}
	delete(h.conns, c)
	h.byIdentity[c.identity]--
	if h.byIdentity[c.identity] <= 0 {
		delete(h.byIdentity, c.identity)
	}
}

// dispatch fans one event out to every matching subscription. A connection
// subscribed to several matching channels receives one frame per channel.
func (h *hub) dispatch(ev *core.Event) {
	h.mu.RLock()
	conns := make([]*connection, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	// Encode the event body once: only the channel field differs per recipient,
	// so the body is reused across every matching subscription instead of being
	// re-marshalled per (connection, subscription) pair.
	body, err := json.Marshal(outboundEventBody{
		ID:        ev.ID,
		Type:      ev.Type,
		Data:      ev.Payload,
		Timestamp: ev.Timestamp,
	})
	if err != nil {
		h.logger.Warn("realtime: failed to encode event", "error", err)
		return
	}

	for _, c := range conns {
		for _, sub := range c.snapshotSubs() {
			if !sub.matchEvent(ev) {
				continue
			}
			frame, err := buildFrame(sub.channel, body)
			if err != nil {
				h.logger.Warn("realtime: failed to encode frame", "error", err)
				continue
			}
			c.enqueue(frame)
		}
	}
}

// run drains the ingest channel and dispatches each event until ctx is done.
func (h *hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-h.ingestCh:
			h.dispatch(ev)
		}
	}
}

// buildFrame wraps a pre-encoded event body in the channel envelope. The body is
// inserted verbatim (json.RawMessage), so one encoded body is reused across every
// recipient without re-marshalling the event payload per channel.
func buildFrame(channel string, body json.RawMessage) ([]byte, error) {
	return json.Marshal(outboundFrame{Channel: channel, Event: body})
}
