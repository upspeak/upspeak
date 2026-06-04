package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveRuleExecution persists a rule execution record to the database.
// After inserting, it cleans up old entries keeping only the most recent 1000
// per rule to prevent unbounded growth.
func (a *LocalArchive) saveRuleExecution(exec *core.RuleExecution) error {
	if exec == nil {
		return fmt.Errorf("rule execution is nil")
	}

	// Generate ID if not set.
	if exec.ID == uuid.Nil {
		exec.ID = core.NewID()
	}

	actionsJSON, err := json.Marshal(exec.ActionsExecuted)
	if err != nil {
		return fmt.Errorf("failed to marshal rule execution actions: %w", err)
	}

	_, err = a.db.Exec(`
INSERT INTO rule_executions (id, rule_id, event_id, event_type, actions_executed, at, duration_ms)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, exec.ID.String(), exec.RuleID.String(), exec.EventID.String(),
		string(exec.EventType), string(actionsJSON),
		exec.At.Format(time.RFC3339), exec.DurationMs)
	if err != nil {
		return fmt.Errorf("failed to insert rule execution: %w", err)
	}

	// Clean up old entries, keeping only the most recent 1000 per rule.
	_, err = a.db.Exec(`
DELETE FROM rule_executions
WHERE rule_id = ? AND id NOT IN (
SELECT id FROM rule_executions WHERE rule_id = ? ORDER BY at DESC LIMIT 1000
)
`, exec.RuleID.String(), exec.RuleID.String())
	if err != nil {
		return fmt.Errorf("failed to clean up old rule executions: %w", err)
	}

	return nil
}

// listRuleExecutions returns paginated execution records for a rule,
// ordered by execution time descending.
func (a *LocalArchive) listRuleExecutions(ruleID uuid.UUID, opts core.ListOptions) ([]core.RuleExecution, int, error) {
	where := `WHERE rule_id = ?`
	args := []any{ruleID.String()}

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM rule_executions `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count rule executions: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, rule_id, event_id, event_type, actions_executed, at, duration_ms
 FROM rule_executions %s ORDER BY at DESC LIMIT ? OFFSET ?`,
		where,
	)

	queryArgs := append(args, opts.Limit, opts.Offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list rule executions: %w", err)
	}
	defer rows.Close()

	var executions []core.RuleExecution
	for rows.Next() {
		exec, err := scanRuleExecutionFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		executions = append(executions, *exec)
	}

	return executions, total, nil
}

// scanRuleExecutionFromRow scans a rule execution from a *sql.Rows iterator.
func scanRuleExecutionFromRow(rows *sql.Rows) (*core.RuleExecution, error) {
	var exec core.RuleExecution
	var idStr, ruleIDStr, eventIDStr, eventTypeStr, actionsStr, atStr string

	err := rows.Scan(&idStr, &ruleIDStr, &eventIDStr, &eventTypeStr, &actionsStr, &atStr, &exec.DurationMs)
	if err != nil {
		return nil, fmt.Errorf("failed to scan rule execution row: %w", err)
	}

	exec.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule execution ID: %w", err)
	}
	exec.RuleID, err = uuid.Parse(ruleIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule execution rule_id: %w", err)
	}
	exec.EventID, err = uuid.Parse(eventIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule execution event_id: %w", err)
	}

	exec.EventType = core.EventType(eventTypeStr)

	if actionsStr != "" {
		if err := json.Unmarshal([]byte(actionsStr), &exec.ActionsExecuted); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rule execution actions: %w", err)
		}
	}

	exec.At, err = time.Parse(time.RFC3339, atStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule execution at: %w", err)
	}

	return &exec, nil
}
