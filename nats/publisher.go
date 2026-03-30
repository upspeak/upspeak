package nats

import "github.com/nats-io/nats.go"

// publisher implements app.Publisher using a NATS connection.
type publisher struct {
	nc *nats.Conn
}

// Publish publishes data to the given subject.
func (p *publisher) Publish(subject string, data []byte) error {
	return p.nc.Publish(subject, data)
}
