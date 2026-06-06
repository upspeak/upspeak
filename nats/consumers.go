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
	ConsumerJobRunner      = "job-runner"
	ConsumerScheduleRunner = "schedule-runner"
	ConsumerRulesEngine    = "rules-engine"
	ConsumerSinkPublisher  = "sink-publisher" // on REPO_EVENTS
	ConsumerRepoIngest     = "repo-ingest"    // on SINK_EVENTS
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

// CreateRulesEngineConsumer creates the durable pull consumer for the rules
// engine on the global REPO_EVENTS stream. It receives every repository's domain
// events with explicit acknowledgement, so events published while the engine is
// down are delivered on restart rather than lost.
func (cm *ConsumerManager) CreateRulesEngineConsumer() error {
	return cm.CreateConsumer(RepoEventsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerRulesEngine,
		FilterSubject: RepoEventsSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}

// CreateScheduleRunnerConsumer creates the durable pull consumer for schedule
// trigger processing on the SCHEDULES stream. The schedule runner consumes
// trigger messages and creates jobs for each fired schedule.
func (cm *ConsumerManager) CreateScheduleRunnerConsumer() error {
	return cm.CreateConsumer(SchedulesStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerScheduleRunner,
		FilterSubject: "schedules.trigger.>",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}

// CreateSinkPublisherConsumer creates the durable consumer the publish
// supervisor uses to read every repository's domain events from REPO_EVENTS.
func (cm *ConsumerManager) CreateSinkPublisherConsumer() error {
	return cm.CreateConsumer(RepoEventsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerSinkPublisher,
		FilterSubject: RepoEventsSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}

// CreateRepoIngestConsumer creates the durable consumer the ingest supervisor
// uses to read curated Sink events from SINK_EVENTS.
func (cm *ConsumerManager) CreateRepoIngestConsumer() error {
	return cm.CreateConsumer(SinkEventsStreamName, &nats.ConsumerConfig{
		Durable:       ConsumerRepoIngest,
		FilterSubject: SinkEventsSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
}
