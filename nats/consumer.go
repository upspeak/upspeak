package nats

import (
	"fmt"
	"time"

	natsclient "github.com/nats-io/nats.go"
	"github.com/upspeak/upspeak/app"
)

// consumer implements app.Consumer using a JetStream pull subscription.
type consumer struct {
	sub *natsclient.Subscription
}

// NewConsumer creates an app.Consumer backed by a JetStream pull subscription
// for the given durable consumer on a subject.
func NewConsumer(bus *Bus, subject, durable string) (app.Consumer, error) {
	sub, err := bus.js.PullSubscribe(subject, durable)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull subscription %s/%s: %w", subject, durable, err)
	}
	return &consumer{sub: sub}, nil
}

// Fetch retrieves up to maxMsgs messages, blocking until at least one is
// available or the timeout is reached. Each returned Msg must be acknowledged.
func (c *consumer) Fetch(maxMsgs int, timeout time.Duration) ([]*app.Msg, error) {
	msgs, err := c.sub.Fetch(maxMsgs, natsclient.MaxWait(timeout))
	if err != nil {
		return nil, err
	}

	result := make([]*app.Msg, len(msgs))
	for i, msg := range msgs {
		result[i] = wrapMsg(msg)
	}
	return result, nil
}

// wrapMsg converts a nats.Msg into an app.Msg with acknowledgement functions.
// The nats ack methods accept variadic AckOpt; these closures call them with
// no options, which is the standard usage.
func wrapMsg(msg *natsclient.Msg) *app.Msg {
	return app.NewMsg(
		msg.Subject,
		msg.Data,
		func() error { return msg.Ack() },
		func() error { return msg.Nak() },
		func() error { return msg.InProgress() },
		func() error { return msg.Term() },
	)
}
