package nats

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/upspeak/upspeak/core"
)

// publisher implements app.Publisher using JetStream for delivery confirmation.
// Using js.Publish() instead of nc.Publish() ensures the server acknowledges
// that the message has been persisted to the stream before returning.
type publisher struct {
	js nats.JetStreamContext
}

// Publish publishes data to the given subject via JetStream.
// Returns an error if the server does not confirm storage.
func (p *publisher) Publish(subject string, data []byte) error {
	_, err := p.js.Publish(subject, data)
	return err
}

// PublishEvent builds a core.Event envelope for the given type, repo, and
// payload and publishes it on the event's canonical subject. The envelope shape
// (core.NewEvent) and subject scheme (core.Event.Subject) are owned by core, so
// every producer that publishes through this method stays on the same wire
// format without re-implementing the marshalling.
func (p *publisher) PublishEvent(eventType core.EventType, repoID uuid.UUID, payload any) error {
	evt, err := core.NewEvent(eventType, repoID, payload)
	if err != nil {
		return err
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return p.Publish(evt.Subject(), data)
}
