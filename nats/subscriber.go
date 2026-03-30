package nats

import "github.com/nats-io/nats.go"

// subscriber implements app.Subscriber using a NATS connection.
type subscriber struct {
	nc *nats.Conn
}

// Subscribe creates a subscription on the given subject, adapting the NATS
// message to the app framework's handler signature.
func (s *subscriber) Subscribe(subject string, handler func(subject string, data []byte)) error {
	_, err := s.nc.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Subject, msg.Data)
	})
	return err
}
