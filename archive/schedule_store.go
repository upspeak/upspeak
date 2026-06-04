package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveSchedule persists a schedule to the database. Schedules are global entities
// (not repo-scoped) that define cron-based triggers for automated actions.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveSchedule(schedule *core.Schedule) error {
	if schedule == nil {
		return fmt.Errorf("schedule is nil")
	}

	now := time.Now().UTC()

	actionJSON, err := json.Marshal(schedule.Action)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule action: %w", err)
	}

	if schedule.Version == 0 {
		// Create: generate global short ID.
		seq, err := nextGlobalSequence(a.db, "schedule")
		if err != nil {
			return fmt.Errorf("failed to generate schedule short ID: %w", err)
		}
		schedule.ShortID = core.FormatShortID(core.PrefixSchedule, seq)
		schedule.Version = 1
		schedule.CreatedAt = now
		schedule.UpdatedAt = now

		var nextRun, lastRun sql.NullString
		if schedule.NextRun != nil {
			nextRun = sql.NullString{String: schedule.NextRun.Format(time.RFC3339), Valid: true}
		}
		if schedule.LastRun != nil {
			lastRun = sql.NullString{String: schedule.LastRun.Format(time.RFC3339), Valid: true}
		}

		_, err = a.db.Exec(`
			INSERT INTO schedules (id, short_id, name, cron, action, enabled, next_run, last_run, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, schedule.ID.String(), schedule.ShortID, schedule.Name, schedule.Cron,
			string(actionJSON), boolToInt(schedule.Enabled), nextRun, lastRun,
			schedule.Version, schedule.CreatedBy.String(),
			schedule.CreatedAt.Format(time.RFC3339), schedule.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert schedule: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	schedule.UpdatedAt = now

	var nextRun, lastRun sql.NullString
	if schedule.NextRun != nil {
		nextRun = sql.NullString{String: schedule.NextRun.Format(time.RFC3339), Valid: true}
	}
	if schedule.LastRun != nil {
		lastRun = sql.NullString{String: schedule.LastRun.Format(time.RFC3339), Valid: true}
	}

	result, err := a.db.Exec(`
		UPDATE schedules
		SET name = ?, cron = ?, action = ?, enabled = ?, next_run = ?, last_run = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?
	`, schedule.Name, schedule.Cron, string(actionJSON), boolToInt(schedule.Enabled),
		nextRun, lastRun, schedule.UpdatedAt.Format(time.RFC3339),
		schedule.ID.String(), schedule.Version)
	if err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "schedule",
			EntityID:   schedule.ID,
			Expected:   schedule.Version,
		}
	}

	schedule.Version++
	return nil
}

// getSchedule retrieves a schedule by UUID.
func (a *LocalArchive) getSchedule(scheduleID uuid.UUID) (*core.Schedule, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, name, cron, action, enabled, next_run, last_run, version, created_by, created_at, updated_at
		FROM schedules WHERE id = ?
	`, scheduleID.String())

	return scanScheduleFromSingleRow(row)
}

// getScheduleByShortID retrieves a schedule by its short ID (e.g. "SCHED-1").
func (a *LocalArchive) getScheduleByShortID(shortID string) (*core.Schedule, error) {
	row := a.db.QueryRow(`
		SELECT id, short_id, name, cron, action, enabled, next_run, last_run, version, created_by, created_at, updated_at
		FROM schedules WHERE short_id = ?
	`, shortID)

	return scanScheduleFromSingleRow(row)
}

// deleteSchedule removes a schedule from the database.
func (a *LocalArchive) deleteSchedule(scheduleID uuid.UUID) error {
	result, err := a.db.Exec(`
		DELETE FROM schedules WHERE id = ?
	`, scheduleID.String())
	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("schedule", scheduleID.String())
	}

	return nil
}

// listSchedules returns paginated schedules, optionally filtered by repo, enabled state,
// and action type. Filtering is done by examining the JSON action field.
func (a *LocalArchive) listSchedules(opts core.ScheduleListOptions) ([]core.Schedule, int, error) {
	where := "WHERE 1=1"
	var args []any

	// Filter by repo_id in JSON action field
	if opts.RepoID != "" {
		where += ` AND json_unquote(json_extract(action, '$.repo_id')) = ?`
		args = append(args, opts.RepoID)
	}

	// Filter by enabled state
	if opts.Enabled != nil {
		where += ` AND enabled = ?`
		args = append(args, boolToInt(*opts.Enabled))
	}

	// Filter by action type
	if opts.ActionType != "" {
		where += ` AND json_unquote(json_extract(action, '$.type')) = ?`
		args = append(args, opts.ActionType)
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM schedules %s", where)
	var total int
	countArgs := args
	err := a.db.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count schedules: %w", err)
	}

	// Handle sort order
	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	order := opts.Order
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, name, cron, action, enabled, next_run, last_run, version, created_by, created_at, updated_at
		 FROM schedules %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []core.Schedule
	for rows.Next() {
		s, err := scanScheduleFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		schedules = append(schedules, *s)
	}

	return schedules, total, nil
}

// getEnabledSchedules returns all enabled schedules (for the scheduler runner).
func (a *LocalArchive) getEnabledSchedules() ([]core.Schedule, error) {
	rows, err := a.db.Query(`
		SELECT id, short_id, name, cron, action, enabled, next_run, last_run, version, created_by, created_at, updated_at
		FROM schedules WHERE enabled = 1
		ORDER BY next_run ASC NULLS LAST
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled schedules: %w", err)
	}
	defer rows.Close()

	var schedules []core.Schedule
	for rows.Next() {
		s, err := scanScheduleFromRow(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, *s)
	}

	return schedules, nil
}

// scanScheduleFromSingleRow scans a schedule from a *sql.Row (single-row query).
func scanScheduleFromSingleRow(row *sql.Row) (*core.Schedule, error) {
	var schedule core.Schedule
	var idStr, createdByStr, actionStr, createdAt, updatedAt string
	var enabled int
	var nextRun, lastRun sql.NullString

	err := row.Scan(&idStr, &schedule.ShortID, &schedule.Name, &schedule.Cron, &actionStr,
		&enabled, &nextRun, &lastRun, &schedule.Version, &createdByStr,
		&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("schedule", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan schedule: %w", err)
	}

	return parseScheduleFields(&schedule, idStr, createdByStr, actionStr, enabled,
		nextRun, lastRun, createdAt, updatedAt)
}

// scanScheduleFromRow scans a schedule from a *sql.Rows iterator.
func scanScheduleFromRow(rows *sql.Rows) (*core.Schedule, error) {
	var schedule core.Schedule
	var idStr, createdByStr, actionStr, createdAt, updatedAt string
	var enabled int
	var nextRun, lastRun sql.NullString

	err := rows.Scan(&idStr, &schedule.ShortID, &schedule.Name, &schedule.Cron, &actionStr,
		&enabled, &nextRun, &lastRun, &schedule.Version, &createdByStr,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan schedule row: %w", err)
	}

	return parseScheduleFields(&schedule, idStr, createdByStr, actionStr, enabled,
		nextRun, lastRun, createdAt, updatedAt)
}

// parseScheduleFields populates a Schedule's parsed fields from raw scanned strings.
func parseScheduleFields(schedule *core.Schedule, idStr, createdByStr, actionStr string,
	enabled int, nextRun, lastRun sql.NullString, createdAt, updatedAt string) (*core.Schedule, error) {
	var err error

	schedule.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schedule ID: %w", err)
	}

	schedule.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schedule created_by: %w", err)
	}

	schedule.Enabled = intToBool(enabled)

	// Unmarshal action JSON
	err = json.Unmarshal([]byte(actionStr), &schedule.Action)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule action: %w", err)
	}

	// Parse optional timestamps
	if nextRun.Valid {
		t, err := time.Parse(time.RFC3339, nextRun.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse schedule next_run: %w", err)
		}
		schedule.NextRun = &t
	}

	if lastRun.Valid {
		t, err := time.Parse(time.RFC3339, lastRun.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse schedule last_run: %w", err)
		}
		schedule.LastRun = &t
	}

	schedule.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schedule created_at: %w", err)
	}

	schedule.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schedule updated_at: %w", err)
	}

	return schedule, nil
}

// boolToInt converts a boolean to an integer for storage in SQLite.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// intToBool converts an integer from SQLite to a boolean.
func intToBool(i int) bool {
	return i != 0
}
