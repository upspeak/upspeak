package archive

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveRepository persists a repository to the database.
// If Version == 0, this is a create (inserts with Version 1).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveRepository(repo *core.Repository) error {
	if repo == nil {
		return fmt.Errorf("repository is nil")
	}

	now := time.Now().UTC()

	if repo.Version == 0 {
		// Create: generate short ID.
		seq, err := nextUserSequence(a.db, repo.OwnerID, "repo")
		if err != nil {
			return fmt.Errorf("failed to generate short ID: %w", err)
		}
		repo.ShortID = core.FormatShortID(core.PrefixRepo, seq)
		repo.Version = 1
		repo.CreatedAt = now
		repo.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO repositories (id, short_id, slug, name, description, owner_id, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.ID.String(), repo.ShortID, repo.Slug, repo.Name, repo.Description,
			repo.OwnerID.String(), repo.Version,
			repo.CreatedAt.Format(time.RFC3339), repo.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert repository: %w", err)
		}
		return nil
	}

	// Update: optimistic concurrency check.
	repo.UpdatedAt = now
	result, err := a.db.Exec(`
		UPDATE repositories
		SET slug = ?, name = ?, description = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, repo.Slug, repo.Name, repo.Description,
		repo.UpdatedAt.Format(time.RFC3339),
		repo.ID.String(), repo.Version)
	if err != nil {
		return fmt.Errorf("failed to update repository: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "repository",
			EntityID:   repo.ID,
			Expected:   repo.Version,
		}
	}

	repo.Version++
	return nil
}

// getRepository retrieves a repository by UUID.
func (a *LocalArchive) getRepository(repoID uuid.UUID) (*core.Repository, error) {
	return a.scanRepository(`SELECT id, short_id, slug, name, description, owner_id, version, created_at, updated_at FROM repositories WHERE id = ?`, repoID.String())
}

// getRepositoryBySlug retrieves a repository by slug for a given owner.
func (a *LocalArchive) getRepositoryBySlug(ownerID uuid.UUID, slug string) (*core.Repository, error) {
	return a.scanRepository(`SELECT id, short_id, slug, name, description, owner_id, version, created_at, updated_at FROM repositories WHERE owner_id = ? AND slug = ?`, ownerID.String(), slug)
}

// listRepositories returns paginated repositories for an owner.
func (a *LocalArchive) listRepositories(ownerID uuid.UUID, opts core.ListOptions) ([]core.Repository, int, error) {
	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM repositories WHERE owner_id = ?`, ownerID.String()).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count repositories: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "name", "slug":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, slug, name, description, owner_id, version, created_at, updated_at
		 FROM repositories WHERE owner_id = ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		sortBy, order,
	)

	rows, err := a.db.Query(query, ownerID.String(), opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list repositories: %w", err)
	}
	defer rows.Close()

	var repos []core.Repository
	for rows.Next() {
		repo, err := a.scanRepositoryFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		repos = append(repos, *repo)
	}

	return repos, total, nil
}

// deleteRepository deletes a repository and all its child entities.
func (a *LocalArchive) deleteRepository(repoID uuid.UUID) error {
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete child entities in order (respecting foreign keys).
	tables := []string{"thread_edges", "annotations", "threads", "edges", "nodes"}
	for _, table := range tables {
		if table == "thread_edges" {
			_, err = tx.Exec(`DELETE FROM thread_edges WHERE thread_id IN (SELECT id FROM threads WHERE repo_id = ?)`, repoID.String())
		} else {
			_, err = tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE repo_id = ?`, table), repoID.String())
		}
		if err != nil {
			return fmt.Errorf("failed to delete %s: %w", table, err)
		}
	}

	// Delete repo sequences.
	_, err = tx.Exec(`DELETE FROM repo_sequences WHERE repo_id = ?`, repoID.String())
	if err != nil {
		return fmt.Errorf("failed to delete repo sequences: %w", err)
	}

	// Delete the repository itself.
	_, err = tx.Exec(`DELETE FROM repositories WHERE id = ?`, repoID.String())
	if err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}

	return tx.Commit()
}

// saveSlugRedirect records an old slug as a permanent redirect.
func (a *LocalArchive) saveSlugRedirect(ownerID uuid.UUID, oldSlug string, repoID uuid.UUID) error {
	_, err := a.db.Exec(`
		INSERT INTO repo_slug_redirects (old_slug, owner_id, repo_id, created_at)
		VALUES (?, ?, ?, ?)
	`, oldSlug, ownerID.String(), repoID.String(), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to save slug redirect: %w", err)
	}
	return nil
}

// getSlugRedirect checks if a slug is an old redirect and returns the repo ID
// and current slug.
func (a *LocalArchive) getSlugRedirect(ownerID uuid.UUID, slug string) (uuid.UUID, string, error) {
	var repoIDStr string
	err := a.db.QueryRow(`
		SELECT repo_id FROM repo_slug_redirects WHERE old_slug = ? AND owner_id = ?
	`, slug, ownerID.String()).Scan(&repoIDStr)
	if err == sql.ErrNoRows {
		return uuid.Nil, "", core.NewErrorNotFound("slug redirect", slug)
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to query slug redirect: %w", err)
	}

	repoID, err := uuid.Parse(repoIDStr)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to parse repo ID from redirect: %w", err)
	}

	// Look up current slug.
	repo, err := a.getRepository(repoID)
	if err != nil {
		return uuid.Nil, "", err
	}

	return repoID, repo.Slug, nil
}

// resolveRepoRef resolves a UUID, short ID, slug, or old slug to a Repository.
func (a *LocalArchive) resolveRepoRef(ownerID uuid.UUID, ref string) (*core.Repository, error) {
	// 1. Try as UUID.
	if id, err := uuid.Parse(ref); err == nil {
		return a.getRepository(id)
	}

	// 2. Try as short ID (e.g. "REPO-1").
	if prefix, seq, err := core.ParseShortID(ref); err == nil && prefix == core.PrefixRepo {
		shortID := core.FormatShortID(core.PrefixRepo, seq)
		return a.scanRepository(
			`SELECT id, short_id, slug, name, description, owner_id, version, created_at, updated_at FROM repositories WHERE owner_id = ? AND short_id = ?`,
			ownerID.String(), shortID,
		)
	}

	// 3. Try as slug.
	repo, err := a.getRepositoryBySlug(ownerID, ref)
	if err == nil {
		return repo, nil
	}

	// 4. Check redirect table.
	_, currentSlug, err := a.getSlugRedirect(ownerID, ref)
	if err == nil {
		return nil, &core.ErrorSlugRedirect{
			OldSlug: ref,
			NewSlug: currentSlug,
		}
	}

	return nil, core.NewErrorNotFound("repository", ref)
}

// scanRepository runs a query expected to return a single repository row.
func (a *LocalArchive) scanRepository(query string, args ...any) (*core.Repository, error) {
	row := a.db.QueryRow(query, args...)
	return a.scanRepositoryFromSingleRow(row)
}

func (a *LocalArchive) scanRepositoryFromSingleRow(row *sql.Row) (*core.Repository, error) {
	var repo core.Repository
	var idStr, ownerIDStr, createdAt, updatedAt string

	err := row.Scan(&idStr, &repo.ShortID, &repo.Slug, &repo.Name, &repo.Description,
		&ownerIDStr, &repo.Version, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("repository", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	repo.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository ID: %w", err)
	}
	repo.OwnerID, err = uuid.Parse(ownerIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse owner ID: %w", err)
	}
	repo.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	repo.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}

	return &repo, nil
}

// scanRepositoryFromRow scans a repository from a *sql.Rows iterator.
func (a *LocalArchive) scanRepositoryFromRow(rows *sql.Rows) (*core.Repository, error) {
	var repo core.Repository
	var idStr, ownerIDStr, createdAt, updatedAt string

	err := rows.Scan(&idStr, &repo.ShortID, &repo.Slug, &repo.Name, &repo.Description,
		&ownerIDStr, &repo.Version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository row: %w", err)
	}

	repo.ID, _ = uuid.Parse(idStr)
	repo.OwnerID, _ = uuid.Parse(ownerIDStr)
	repo.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	repo.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &repo, nil
}
