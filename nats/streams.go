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

// NewStreamManager creates a StreamManager from a NATS connection.
func NewStreamManager(nc *nats.Conn) (*StreamManager, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}
	return &StreamManager{js: js}, nil
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
