// Package realtime provides the WebSocket module. Clients open a single
// connection at /api/v1/ws, subscribe to channels, and receive a live push of
// matching domain events fanned out from the repo.*.events.> subject.
package realtime

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// Connection and subscription limits (spec 14-api-realtime.md).
const (
	outboundBufferSize  = 1000             // buffered frames per connection
	maxSubscriptions    = 10               // subscriptions per connection
	maxConnsPerIdentity = 5                // connections per authenticated identity
	ingestBufferSize    = 1024             // hub ingest backlog
	pingInterval        = 30 * time.Second // server keepalive interval
	pingTimeout         = 10 * time.Second // grace for a pong before the conn is closed
)

// clientMessage is a control message sent by a client over the socket.
type clientMessage struct {
	Action  string     `json:"action"` // "subscribe" | "unsubscribe"
	Channel string     `json:"channel"`
	Filter  *subFilter `json:"filter,omitempty"`
}

// subFilter narrows which events a subscription delivers. Both fields are
// optional; an absent or empty filter delivers every event on the channel.
type subFilter struct {
	EventType []string `json:"event_type,omitempty"`
	NodeType  []string `json:"node_type,omitempty"`
}

// outboundEvent is the server-to-client envelope for a delivered domain event.
type outboundEvent struct {
	Channel string            `json:"channel"`
	Event   outboundEventBody `json:"event"`
}

// outboundEventBody mirrors the spec's event shape (id/type/data/timestamp).
type outboundEventBody struct {
	ID        uuid.UUID       `json:"id"`
	Type      core.EventType  `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

// errorMessage is the server-to-client error shape.
type errorMessage struct {
	Type    string `json:"type"` // always "error"
	Code    string `json:"code"` // invalid_channel | subscription_limit | authentication_failed
	Message string `json:"message"`
}

// noticeMessage signals a non-fatal condition such as dropped messages.
type noticeMessage struct {
	Type    string `json:"type"` // e.g. "messages_dropped"
	Message string `json:"message"`
}

// Error codes used in errorMessage.
const (
	codeInvalidChannel     = "invalid_channel"
	codeSubscriptionLimit  = "subscription_limit"
	codeAuthenticationFail = "authentication_failed"
)
