package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveJob persists a job to the database. Jobs use a simpler persistence model
// than versioned entities — they have no optimistic concurrency (status transitions
// are managed by the job runner). A new job (Version == 0) is inserted; an
// existing job is updated by ID.
func (a *LocalArchive) saveJob(job *core.Job) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}

	now := time.Now().UTC()

	var resultJSON sql.NullString
	if len(job.Result) > 0 && string(job.Result) != "null" {
		resultJSON = sql.NullString{String: string(job.Result), Valid: true}
	}

	var errorStr sql.NullString
	if job.Error != nil {
		errorStr = sql.NullString{String: *job.Error, Valid: true}
	}

	var startedAt sql.NullString
	if job.StartedAt != nil {
		startedAt = sql.NullString{String: job.StartedAt.Format(time.RFC3339), Valid: true}
	}

	var completedAt sql.NullString
	if job.CompletedAt != nil {
		completedAt = sql.NullString{String: job.CompletedAt.Format(time.RFC3339), Valid: true}
	}

	if job.ShortID == "" {
		// New job: generate global short ID.
		seq, err := nextGlobalSequence(a.db, "job")
		if err != nil {
			return fmt.Errorf("failed to generate job short ID: %w", err)
		}
		job.ShortID = core.FormatShortID(core.PrefixJob, seq)
		job.CreatedAt = now
		job.UpdatedAt = now

		_, err = a.db.Exec(`
			INSERT INTO jobs (id, short_id, repo_id, type, status, started_at, completed_at, result, error, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, job.ID.String(), job.ShortID, job.RepoID.String(), string(job.Type),
			string(job.Status), startedAt, completedAt, resultJSON, errorStr,
			job.CreatedBy.String(),
			job.CreatedAt.Format(time.RFC3339), job.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert job: %w", err)
		}

		return nil
	}

	// Update existing job.
	job.UpdatedAt = now
	_, err := a.db.Exec(`
		UPDATE jobs
		SET status = ?, started_at = ?, completed_at = ?, result = ?, error = ?, updated_at = ?
		WHERE id = ?
	`, string(job.Status), startedAt, completedAt, resultJSON, errorStr,
		job.UpdatedAt.Format(time.RFC3339), job.ID.String())
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

// getJob retrieves a job by UUID.
func (a *LocalArchive) getJob(jobID uuid.UUID) (*core.Job, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, repo_id, type, status, started_at, completed_at, result, error, created_by, created_at, updated_at
		FROM jobs WHERE id = ?
	`, jobID.String())

	return scanJobFromSingleRow(row)
}

// listJobs returns paginated jobs, optionally filtered by status, type, and repo.
func (a *LocalArchive) listJobs(opts core.JobListOptions) ([]core.Job, int, error) {
	where := "WHERE 1=1"
	var args []any

	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, opts.Status)
	}
	if opts.Type != "" {
		where += ` AND type = ?`
		args = append(args, opts.Type)
	}
	if opts.RepoID != "" {
		where += ` AND repo_id = ?`
		args = append(args, opts.RepoID)
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM jobs `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count jobs: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "status", "type":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, type, status, started_at, completed_at, result, error, created_by, created_at, updated_at
		 FROM jobs %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []core.Job
	for rows.Next() {
		j, err := scanJobFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *j)
	}

	return jobs, total, nil
}

// scanJobFromSingleRow scans a job from a *sql.Row (single-row query).
func scanJobFromSingleRow(row *sql.Row) (*core.Job, error) {
	var job core.Job
	var idStr, repoIDStr, createdByStr, typeStr, statusStr, createdAt, updatedAt string
	var startedAt, completedAt, resultStr, errorStr sql.NullString

	err := row.Scan(&idStr, &job.ShortID, &repoIDStr, &typeStr, &statusStr,
		&startedAt, &completedAt, &resultStr, &errorStr, &createdByStr,
		&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("job", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	return parseJobFields(&job, idStr, repoIDStr, createdByStr, typeStr, statusStr,
		startedAt, completedAt, resultStr, errorStr, createdAt, updatedAt)
}

// scanJobFromRow scans a job from a *sql.Rows iterator.
func scanJobFromRow(rows *sql.Rows) (*core.Job, error) {
	var job core.Job
	var idStr, repoIDStr, createdByStr, typeStr, statusStr, createdAt, updatedAt string
	var startedAt, completedAt, resultStr, errorStr sql.NullString

	err := rows.Scan(&idStr, &job.ShortID, &repoIDStr, &typeStr, &statusStr,
		&startedAt, &completedAt, &resultStr, &errorStr, &createdByStr,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan job row: %w", err)
	}

	return parseJobFields(&job, idStr, repoIDStr, createdByStr, typeStr, statusStr,
		startedAt, completedAt, resultStr, errorStr, createdAt, updatedAt)
}

// parseJobFields populates a Job's parsed fields from raw scanned strings.
func parseJobFields(job *core.Job, idStr, repoIDStr, createdByStr, typeStr, statusStr string,
	startedAt, completedAt, resultStr, errorStr sql.NullString,
	createdAt, updatedAt string) (*core.Job, error) {
	var err error

	job.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job ID: %w", err)
	}
	job.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job repo ID: %w", err)
	}
	job.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job created_by: %w", err)
	}

	job.Type = core.JobType(typeStr)
	job.Status = core.JobStatus(statusStr)

	if startedAt.Valid {
		t, err := time.Parse(time.RFC3339, startedAt.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse job started_at: %w", err)
		}
		job.StartedAt = &t
	}

	if completedAt.Valid {
		t, err := time.Parse(time.RFC3339, completedAt.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse job completed_at: %w", err)
		}
		job.CompletedAt = &t
	}

	if resultStr.Valid {
		job.Result = json.RawMessage(resultStr.String)
	}

	if errorStr.Valid {
		s := errorStr.String
		job.Error = &s
	}

	job.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job created_at: %w", err)
	}
	job.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job updated_at: %w", err)
	}

	return job, nil
}
