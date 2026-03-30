package core

import (
	"fmt"

	"github.com/google/uuid"
)

// VersionConflictError is returned when an optimistic concurrency check fails.
// This happens when a write operation specifies an expected version that does
// not match the entity's current version in storage.
type VersionConflictError struct {
	EntityType string
	EntityID   uuid.UUID
	Expected   int
	Actual     int
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("version conflict on %s %s: expected %d, got %d",
		e.EntityType, e.EntityID, e.Expected, e.Actual)
}
