package archive

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// dbExecer abstracts *sql.DB and *sql.Tx so sequence functions can be used
// both inside and outside transactions.
type dbExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// nextRepoSequence atomically increments and returns the next sequence number
// for the given entity type within a repository.
func nextRepoSequence(db dbExecer, repoID uuid.UUID, entity string) (int, error) {
	_, err := db.Exec(`
		INSERT INTO repo_sequences (repo_id, entity, next_seq)
		VALUES (?, ?, 1)
		ON CONFLICT (repo_id, entity) DO NOTHING
	`, repoID.String(), entity)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure repo sequence row: %w", err)
	}

	var seq int
	err = db.QueryRow(`
		UPDATE repo_sequences
		SET next_seq = next_seq + 1
		WHERE repo_id = ? AND entity = ?
		RETURNING next_seq - 1
	`, repoID.String(), entity).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("failed to get next repo sequence for %s: %w", entity, err)
	}
	return seq, nil
}

// nextUserSequence atomically increments and returns the next sequence number
// for the given entity type scoped to a user (e.g. REPO-N per owner).
func nextUserSequence(db *sql.DB, ownerID uuid.UUID, entity string) (int, error) {
	_, err := db.Exec(`
		INSERT INTO user_sequences (owner_id, entity, next_seq)
		VALUES (?, ?, 1)
		ON CONFLICT (owner_id, entity) DO NOTHING
	`, ownerID.String(), entity)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure user sequence row: %w", err)
	}

	var seq int
	err = db.QueryRow(`
		UPDATE user_sequences
		SET next_seq = next_seq + 1
		WHERE owner_id = ? AND entity = ?
		RETURNING next_seq - 1
	`, ownerID.String(), entity).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("failed to get next user sequence for %s: %w", entity, err)
	}
	return seq, nil
}

// nextGlobalSequence atomically increments and returns the next sequence number
// for the given globally-scoped entity type (e.g. schedule, job).
func nextGlobalSequence(db *sql.DB, entity string) (int, error) {
	_, err := db.Exec(`
		INSERT INTO global_sequences (entity, next_seq)
		VALUES (?, 1)
		ON CONFLICT (entity) DO NOTHING
	`, entity)
	if err != nil {
		return 0, fmt.Errorf("failed to ensure global sequence row: %w", err)
	}

	var seq int
	err = db.QueryRow(`
		UPDATE global_sequences
		SET next_seq = next_seq + 1
		WHERE entity = ?
		RETURNING next_seq - 1
	`, entity).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("failed to get next global sequence for %s: %w", entity, err)
	}
	return seq, nil
}
