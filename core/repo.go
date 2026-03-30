package core

import (
	"time"

	"github.com/google/uuid"
)

// Repository is a top-level organising unit — a self-contained knowledge graph.
// A user might have "research", "work", "personal" repositories.
type Repository struct {
	ID          uuid.UUID `json:"id"`
	ShortID     string    `json:"short_id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     uuid.UUID `json:"owner_id"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
