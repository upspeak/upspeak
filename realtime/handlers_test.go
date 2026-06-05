package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

// newTestModule builds an initialised realtime module wired to a fake resolver
// that knows the given repo ref, with the hub loop running until the test ends.
func newTestModule(t *testing.T, repoRef string, repoID uuid.UUID) *Module {
	t.Helper()
	m := New()
	if err := m.Init(nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	m.resolver = newFakeResolver(repoRef, repoID)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return m
}

// dialWS opens a WebSocket to the module's /ws handler served by srv.
func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func TestHandleWS_SubscribeAndReceiveEvent(t *testing.T) {
	repoID := uuid.New()
	m := newTestModule(t, "research", repoID)
	srv := httptest.NewServer(http.HandlerFunc(m.handleWS))
	t.Cleanup(srv.Close)

	c := dialWS(t, srv)
	defer c.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := wsjson.Write(ctx, c, clientMessage{Action: "subscribe", Channel: "repos.research.events"}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	// The subscribe is processed asynchronously server-side. Ingest until a frame
	// arrives, polling so the test does not depend on processing latency.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("did not receive a dispatched event frame")
		default:
		}
		m.hub.ingest(nodeCreatedEvent(t, repoID, uuid.New(), "article"))

		readCtx, readCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_, data, err := c.Read(readCtx)
		readCancel()
		if err != nil {
			continue // nothing yet; ingest again
		}
		var msg outboundEvent
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		if msg.Channel != "repos.research.events" {
			t.Fatalf("channel: got %q", msg.Channel)
		}
		return
	}
}

func TestHandleWS_InvalidChannelReturnsError(t *testing.T) {
	m := newTestModule(t, "research", uuid.New())
	srv := httptest.NewServer(http.HandlerFunc(m.handleWS))
	t.Cleanup(srv.Close)

	c := dialWS(t, srv)
	defer c.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := wsjson.Write(ctx, c, clientMessage{Action: "subscribe", Channel: "repos.ghost.events"}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	var em errorMessage
	if err := json.Unmarshal(data, &em); err != nil {
		t.Fatalf("decode error frame: %v", err)
	}
	if em.Type != "error" || em.Code != codeInvalidChannel {
		t.Fatalf("expected invalid_channel error, got %+v", em)
	}
}
