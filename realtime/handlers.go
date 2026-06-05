package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// handleWS upgrades an HTTP request to a WebSocket, registers the connection
// with the hub, and runs its read and write loops until either side closes.
func (m *Module) handleWS(w http.ResponseWriter, r *http.Request) {
	identity, err := m.auth.Authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		m.logger.Warn("realtime: websocket upgrade failed", "error", err)
		return
	}
	defer c.CloseNow()

	conn := newConnection(uuid.NewString(), identity)
	if !m.hub.add(conn) {
		_ = c.Close(websocket.StatusPolicyViolation, "connection limit reached")
		return
	}
	defer m.hub.remove(conn)

	// Tie both loops to a single context so either ending stops the other.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go m.writeLoop(ctx, c, conn)
	m.readLoop(ctx, c, conn)
}

// readLoop reads client control messages, applies them to the connection, and
// returns when the socket is closed or errors. Per-message decode failures are
// surfaced to the client without tearing down the connection.
func (m *Module) readLoop(ctx context.Context, c *websocket.Conn, conn *connection) {
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var msg clientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			m.writeJSON(ctx, c, errorMessage{
				Type:    "error",
				Code:    codeInvalidChannel,
				Message: "malformed message",
			})
			continue
		}
		if errMsg := applyClientMessage(conn, msg, m.resolver); errMsg != nil {
			m.writeJSON(ctx, c, *errMsg)
		}
	}
}

// writeLoop drains the connection's outbound buffer to the socket and sends a
// keepalive ping on each interval. It returns when ctx is cancelled or any
// write/ping fails (which the library treats as a closed connection).
func (m *Module) writeLoop(ctx context.Context, c *websocket.Conn, conn *connection) {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-conn.out:
			if err := c.Write(ctx, websocket.MessageText, frame); err != nil {
				return
			}
			if conn.takeDropped() {
				m.writeJSON(ctx, c, noticeMessage{
					Type:    "messages_dropped",
					Message: "outbound buffer overflowed; some events were dropped",
				})
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, pingTimeout)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

// writeJSON marshals v and writes it as a single text frame, ignoring errors
// (a failed write means the connection is closing and the loops will unwind).
func (m *Module) writeJSON(ctx context.Context, c *websocket.Conn, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		m.logger.Warn("realtime: failed to marshal outbound message", "error", err)
		return
	}
	_ = c.Write(ctx, websocket.MessageText, data)
}
