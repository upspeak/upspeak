package nats

import "github.com/nats-io/nats.go"

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
