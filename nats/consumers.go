package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// ConsumerManager handles JetStream consumer lifecycle operations.
type ConsumerManager struct {
	js nats.JetStreamContext
}

// NewConsumerManager creates a ConsumerManager from a NATS Bus.
func NewConsumerManager(bus *Bus) *ConsumerManager {
	return &ConsumerManager{js: bus.js}
}

// CreateConsumer creates or updates a durable pull consumer on the given stream.
func (cm *ConsumerManager) CreateConsumer(stream string, config *nats.ConsumerConfig) error {
	_, err := cm.js.AddConsumer(stream, config)
	if err != nil {
		return fmt.Errorf("failed to create consumer %s on stream %s: %w", config.Durable, stream, err)
	}
	return nil
}

// DeleteConsumer removes a consumer from a stream.
func (cm *ConsumerManager) DeleteConsumer(stream, consumer string) error {
	err := cm.js.DeleteConsumer(stream, consumer)
	if err != nil {
		return fmt.Errorf("failed to delete consumer %s from stream %s: %w", consumer, stream, err)
	}
	return nil
}

// PullSubscribe creates a pull subscription for a durable consumer.
// The caller uses sub.Fetch() to retrieve messages and must call msg.Ack()
// or msg.Nak() on each received message.
func (cm *ConsumerManager) PullSubscribe(subject, durable string) (*nats.Subscription, error) {
	sub, err := cm.js.PullSubscribe(subject, durable)
	if err != nil {
		return nil, fmt.Errorf("failed to pull subscribe %s/%s: %w", subject, durable, err)
	}
	return sub, nil
}

// Consumer names for use across the application.
const (
	ConsumerJobRunner = "job-runner"
)

// CreateJobRunnerConsumer creates the durable pull consumer for async job
// execution on the JOBS stream. Messages are delivered to a single worker
// at a time with explicit acknowledgement.
func (cm *ConsumerManager) CreateJobRunnerConsumer() error {
	return cm.CreateConsumer(JobsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerJobRunner,
		FilterSubject: "jobs.>",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}
