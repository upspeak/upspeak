package nats

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// StreamManager handles JetStream stream lifecycle operations.
type StreamManager struct {
	js nats.JetStreamContext
}

// NewStreamManager creates a StreamManager from a NATS Bus.
func NewStreamManager(bus *Bus) *StreamManager {
	return &StreamManager{js: bus.js}
}

// streamName returns the JetStream stream name for a repository.
func streamName(repoID uuid.UUID) string {
	return fmt.Sprintf("REPO_%s_EVENTS", repoID.String())
}

// subjectFilter returns the subject filter for a repository's events.
func subjectFilter(repoID uuid.UUID) string {
	return fmt.Sprintf("repo.%s.events.>", repoID.String())
}

// CreateRepoStream creates a JetStream stream for a repository's events.
// The stream captures all subjects matching repo.{repo_id}.events.>
// with Limits retention and file storage.
func (sm *StreamManager) CreateRepoStream(repoID uuid.UUID) error {
	_, err := sm.js.AddStream(&nats.StreamConfig{
		Name:      streamName(repoID),
		Subjects:  []string{subjectFilter(repoID)},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("failed to create stream for repo %s: %w", repoID, err)
	}
	return nil
}

// DeleteRepoStream deletes the JetStream stream for a repository.
func (sm *StreamManager) DeleteRepoStream(repoID uuid.UUID) error {
	err := sm.js.DeleteStream(streamName(repoID))
	if err != nil {
		return fmt.Errorf("failed to delete stream for repo %s: %w", repoID, err)
	}
	return nil
}

// JobsStreamName is the name of the global JOBS stream.
const JobsStreamName = "JOBS"

// CreateJobsStream creates the global JOBS stream with WorkQueue retention.
// Messages are deleted once acknowledged by the consumer. Subjects: jobs.>
func (sm *StreamManager) CreateJobsStream() error {
	_, err := sm.js.AddStream(&nats.StreamConfig{
		Name:      JobsStreamName,
		Subjects:  []string{"jobs.>"},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("failed to create JOBS stream: %w", err)
	}
	return nil
}
