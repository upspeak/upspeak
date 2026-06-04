package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveRule persists a rule to the database.
// If Version == 0, this is a create (inserts with Version 1 and generates a short ID).
// If Version > 0, this is an update with optimistic concurrency check.
func (a *LocalArchive) saveRule(rule *core.Rule) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}

	now := time.Now().UTC()

	triggerJSON, err := json.Marshal(rule.Trigger)
	if err != nil {
		return fmt.Errorf("failed to marshal rule trigger: %w", err)
	}

	actionsJSON, err := json.Marshal(rule.Actions)
	if err != nil {
		return fmt.Errorf("failed to marshal rule actions: %w", err)
	}

	if rule.Version == 0 {
		// Create: generate short ID.
		seq, err := nextRepoSequence(a.db, rule.RepoID, "rule")
		if err != nil {
			return fmt.Errorf("failed to generate rule short ID: %w", err)
		}
		rule.ShortID = core.FormatShortID(core.PrefixRule, seq)
		rule.Version = 1
		rule.CreatedAt = now
		rule.UpdatedAt = now

		_, err = a.db.Exec(`
INSERT INTO rules (id, short_id, repo_id, name, trigger, actions, status, version, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, rule.ID.String(), rule.ShortID, rule.RepoID.String(), rule.Name,
			string(triggerJSON), string(actionsJSON), string(rule.Status),
			rule.Version, rule.CreatedBy.String(),
			rule.CreatedAt.Format(time.RFC3339), rule.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert rule: %w", err)
		}

		return nil
	}

	// Update: optimistic concurrency check.
	rule.UpdatedAt = now
	result, err := a.db.Exec(`
UPDATE rules
SET name = ?, trigger = ?, actions = ?, status = ?, version = version + 1, updated_at = ?
WHERE id = ? AND version = ?
`, rule.Name, string(triggerJSON), string(actionsJSON), string(rule.Status),
		rule.UpdatedAt.Format(time.RFC3339),
		rule.ID.String(), rule.Version)
	if err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return &core.VersionConflictError{
			EntityType: "rule",
			EntityID:   rule.ID,
			Expected:   rule.Version,
		}
	}

	rule.Version++
	return nil
}

// getRule retrieves a rule by UUID.
func (a *LocalArchive) getRule(ruleID uuid.UUID) (*core.Rule, error) {
	row := a.db.QueryRow(`
SELECT id, short_id, repo_id, name, trigger, actions, status, version, created_by, created_at, updated_at
FROM rules WHERE id = ?
`, ruleID.String())

	return scanRuleFromSingleRow(row)
}

// deleteRule deletes a rule by UUID.
func (a *LocalArchive) deleteRule(ruleID uuid.UUID) error {
	result, err := a.db.Exec(`DELETE FROM rules WHERE id = ?`, ruleID.String())
	if err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return core.NewErrorNotFound("rule", ruleID.String())
	}

	return nil
}

// listRules returns paginated rules for a repository with optional status filter.
func (a *LocalArchive) listRules(repoID uuid.UUID, opts core.RuleListOptions) ([]core.Rule, int, error) {
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}

	// Filter by status if specified.
	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, string(opts.Status))
	}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM rules `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count rules: %w", err)
	}

	// Validate sort field.
	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "name":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, name, trigger, actions, status, version, created_by, created_at, updated_at
 FROM rules %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list rules: %w", err)
	}
	defer rows.Close()

	var rules []core.Rule
	for rows.Next() {
		r, err := scanRuleFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		rules = append(rules, *r)
	}

	return rules, total, nil
}

// getActiveRulesForEvent returns all active rules for a repository whose trigger
// matches the given event type.
func (a *LocalArchive) getActiveRulesForEvent(repoID uuid.UUID, eventType core.EventType) ([]core.Rule, error) {
	rows, err := a.db.Query(`
SELECT id, short_id, repo_id, name, trigger, actions, status, version, created_by, created_at, updated_at
FROM rules
WHERE repo_id = ? AND status = 'active' AND json_extract(trigger, '$.event') = ?
`, repoID.String(), string(eventType))
	if err != nil {
		return nil, fmt.Errorf("failed to query active rules for event: %w", err)
	}
	defer rows.Close()

	var rules []core.Rule
	for rows.Next() {
		r, err := scanRuleFromRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}

	return rules, nil
}

// scanRuleFromSingleRow scans a rule from a *sql.Row (single-row query).
func scanRuleFromSingleRow(row *sql.Row) (*core.Rule, error) {
	var rule core.Rule
	var idStr, repoIDStr, createdByStr, triggerStr, actionsStr, statusStr, createdAt, updatedAt string

	err := row.Scan(&idStr, &rule.ShortID, &repoIDStr, &rule.Name,
		&triggerStr, &actionsStr, &statusStr,
		&rule.Version, &createdByStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, core.NewErrorNotFound("rule", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan rule: %w", err)
	}

	return parseRuleFields(&rule, idStr, repoIDStr, createdByStr, triggerStr, actionsStr, statusStr, createdAt, updatedAt)
}

// scanRuleFromRow scans a rule from a *sql.Rows iterator.
func scanRuleFromRow(rows *sql.Rows) (*core.Rule, error) {
	var rule core.Rule
	var idStr, repoIDStr, createdByStr, triggerStr, actionsStr, statusStr, createdAt, updatedAt string

	err := rows.Scan(&idStr, &rule.ShortID, &repoIDStr, &rule.Name,
		&triggerStr, &actionsStr, &statusStr,
		&rule.Version, &createdByStr, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to scan rule row: %w", err)
	}

	return parseRuleFields(&rule, idStr, repoIDStr, createdByStr, triggerStr, actionsStr, statusStr, createdAt, updatedAt)
}

// parseRuleFields populates a Rule's parsed fields from raw scanned strings.
func parseRuleFields(rule *core.Rule, idStr, repoIDStr, createdByStr, triggerStr, actionsStr, statusStr, createdAt, updatedAt string) (*core.Rule, error) {
	var err error

	rule.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule ID: %w", err)
	}
	rule.RepoID, err = uuid.Parse(repoIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule repo ID: %w", err)
	}
	rule.CreatedBy, err = uuid.Parse(createdByStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule created_by: %w", err)
	}

	rule.Status = core.ResourceStatus(statusStr)

	if triggerStr != "" {
		if err := json.Unmarshal([]byte(triggerStr), &rule.Trigger); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rule trigger: %w", err)
		}
	}

	if actionsStr != "" {
		if err := json.Unmarshal([]byte(actionsStr), &rule.Actions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rule actions: %w", err)
		}
	}

	rule.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule created_at: %w", err)
	}
	rule.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule updated_at: %w", err)
	}

	return rule, nil
}
