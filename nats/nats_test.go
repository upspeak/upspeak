package nats

import (
	"testing"
	"time"

	"github.com/google/uuid"
	natsclient "github.com/nats-io/nats.go"
)

// setupTestBus creates an embedded NATS Bus for testing with a private
// in-process server. The bus is stopped when the test completes.
func setupTestBus(t *testing.T) *Bus {
	t.Helper()
	bus, err := Start("test", Config{
		Embedded: true,
		Private:  true,
		Logging:  false,
	})
	if err != nil {
		t.Fatalf("failed to start test bus: %v", err)
	}
	t.Cleanup(bus.Stop)
	return bus
}

func TestStart_EmbeddedPrivate(t *testing.T) {
	bus := setupTestBus(t)

	if bus.nc == nil {
		t.Fatal("expected NATS connection")
	}
	if bus.js == nil {
		t.Fatal("expected JetStream context")
	}
	if bus.ns == nil {
		t.Fatal("expected embedded server")
	}
	if !bus.nc.IsConnected() {
		t.Fatal("expected connection to be connected")
	}
}

func TestPublisher_JetStream(t *testing.T) {
	bus := setupTestBus(t)

	// Create a stream to capture the test subject.
	repoID := uuid.New()
	sm := NewStreamManager(bus)
	if err := sm.CreateRepoStream(repoID); err != nil {
		t.Fatalf("failed to create repo stream: %v", err)
	}

	// Publish via the app.Publisher interface.
	pub := bus.Publisher()
	subject := "repo." + repoID.String() + ".events.NodeCreated"
	data := []byte(`{"test": true}`)

	if err := pub.Publish(subject, data); err != nil {
		t.Fatalf("JetStream publish failed: %v", err)
	}

	// Verify the message is in the stream by subscribing and fetching.
	sub, err := bus.js.PullSubscribe(subject, "test-verify",
		natsclient.BindStream(streamName(repoID)))
	if err != nil {
		t.Fatalf("failed to pull subscribe: %v", err)
	}

	msgs, err := sub.Fetch(1, natsclient.MaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("failed to fetch message: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Data) != `{"test": true}` {
		t.Errorf("expected data %q, got %q", `{"test": true}`, string(msgs[0].Data))
	}
}

func TestStreamManager_RepoStream(t *testing.T) {
	bus := setupTestBus(t)
	sm := NewStreamManager(bus)
	repoID := uuid.New()

	// Create.
	if err := sm.CreateRepoStream(repoID); err != nil {
		t.Fatalf("CreateRepoStream failed: %v", err)
	}

	// Verify stream exists.
	info, err := bus.js.StreamInfo(streamName(repoID))
	if err != nil {
		t.Fatalf("failed to get stream info: %v", err)
	}
	if info.Config.Name != streamName(repoID) {
		t.Errorf("expected stream name %s, got %s", streamName(repoID), info.Config.Name)
	}
	if info.Config.Retention != natsclient.LimitsPolicy {
		t.Errorf("expected LimitsPolicy retention, got %v", info.Config.Retention)
	}

	// Delete.
	if err := sm.DeleteRepoStream(repoID); err != nil {
		t.Fatalf("DeleteRepoStream failed: %v", err)
	}

	// Verify deleted.
	_, err = bus.js.StreamInfo(streamName(repoID))
	if err == nil {
		t.Error("expected error after deleting stream, got nil")
	}
}

func TestStreamManager_JobsStream(t *testing.T) {
	bus := setupTestBus(t)
	sm := NewStreamManager(bus)

	if err := sm.CreateJobsStream(); err != nil {
		t.Fatalf("CreateJobsStream failed: %v", err)
	}

	info, err := bus.js.StreamInfo(JobsStreamName)
	if err != nil {
		t.Fatalf("failed to get JOBS stream info: %v", err)
	}
	if info.Config.Name != "JOBS" {
		t.Errorf("expected stream name JOBS, got %s", info.Config.Name)
	}
	if info.Config.Retention != natsclient.WorkQueuePolicy {
		t.Errorf("expected WorkQueuePolicy retention, got %v", info.Config.Retention)
	}
	if len(info.Config.Subjects) != 1 || info.Config.Subjects[0] != "jobs.>" {
		t.Errorf("expected subjects [jobs.>], got %v", info.Config.Subjects)
	}
}

func TestConsumerManager_JobRunnerConsumer(t *testing.T) {
	bus := setupTestBus(t)
	sm := NewStreamManager(bus)
	cm := NewConsumerManager(bus)

	// JOBS stream must exist first.
	if err := sm.CreateJobsStream(); err != nil {
		t.Fatalf("CreateJobsStream failed: %v", err)
	}

	if err := cm.CreateJobRunnerConsumer(); err != nil {
		t.Fatalf("CreateJobRunnerConsumer failed: %v", err)
	}

	// Verify consumer exists.
	info, err := bus.js.ConsumerInfo(JobsStreamName, ConsumerJobRunner)
	if err != nil {
		t.Fatalf("failed to get consumer info: %v", err)
	}
	if info.Config.Durable != ConsumerJobRunner {
		t.Errorf("expected durable %s, got %s", ConsumerJobRunner, info.Config.Durable)
	}
	if info.Config.AckPolicy != natsclient.AckExplicitPolicy {
		t.Errorf("expected AckExplicit, got %v", info.Config.AckPolicy)
	}
	if info.Config.MaxDeliver != 5 {
		t.Errorf("expected MaxDeliver 5, got %d", info.Config.MaxDeliver)
	}
}

func TestConsumer_FetchAndAck(t *testing.T) {
	bus := setupTestBus(t)
	sm := NewStreamManager(bus)
	cm := NewConsumerManager(bus)

	if err := sm.CreateJobsStream(); err != nil {
		t.Fatalf("CreateJobsStream failed: %v", err)
	}
	if err := cm.CreateJobRunnerConsumer(); err != nil {
		t.Fatalf("CreateJobRunnerConsumer failed: %v", err)
	}

	// Publish a job message.
	pub := bus.Publisher()
	jobData := []byte(`{"type":"collect","repo_id":"test"}`)
	if err := pub.Publish("jobs.collect.test", jobData); err != nil {
		t.Fatalf("failed to publish job: %v", err)
	}

	// Create an app.Consumer and fetch.
	c, err := NewConsumer(bus, "jobs.>", ConsumerJobRunner)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	msgs, err := c.Fetch(1, 2*time.Second)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Data) != string(jobData) {
		t.Errorf("expected data %q, got %q", string(jobData), string(msgs[0].Data))
	}
	if msgs[0].Subject != "jobs.collect.test" {
		t.Errorf("expected subject jobs.collect.test, got %s", msgs[0].Subject)
	}

	// Ack the message.
	if err := msgs[0].Ack(); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// After ack on a WorkQueue stream, the message should be gone.
	// Fetching again should timeout with no messages.
	_, err = c.Fetch(1, 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error on empty queue, got nil")
	}
}

func TestSubscriber(t *testing.T) {
	bus := setupTestBus(t)

	received := make(chan string, 1)
	sub := bus.Subscriber()
	if err := sub.Subscribe("test.subject", func(subject string, data []byte) {
		received <- string(data)
	}); err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Use core NATS publish for the subscriber (it's a core NATS subscription).
	if err := bus.nc.Publish("test.subject", []byte("hello")); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	bus.nc.Flush()

	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("expected hello, got %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}
