package core

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Rule is an event-condition-action trigger: "When X happens, if Y is true, do Z."
// Rules belong to a repository and are evaluated by the rules engine consumer.
type Rule struct {
	ID        uuid.UUID      `json:"id"`
	ShortID   string         `json:"short_id"`
	RepoID    uuid.UUID      `json:"repo_id"`
	Name      string         `json:"name"`
	Trigger   RuleTrigger    `json:"trigger"`
	Actions   []RuleAction   `json:"actions"`
	Status    ResourceStatus `json:"status"`
	Version   int            `json:"version"`
	CreatedBy uuid.UUID      `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// RuleTrigger defines when a rule activates. Event is the event type to match.
// FilterIDs are optional filters evaluated against the event payload — all must
// pass (AND logic) for the rule to fire.
type RuleTrigger struct {
	Event     EventType   `json:"event"`
	FilterIDs []uuid.UUID `json:"filter_ids,omitempty"`
}

// RuleAction defines what happens when a rule fires. Type determines which
// Params fields are relevant.
type RuleAction struct {
	Type   ActionType      `json:"type"`
	Params json.RawMessage `json:"params"`
}

// RuleExecution records the result of a single rule evaluation and action execution.
type RuleExecution struct {
	ID              uuid.UUID              `json:"id"`
	RuleID          uuid.UUID              `json:"rule_id"`
	EventID         uuid.UUID              `json:"event_id"`
	EventType       EventType              `json:"event_type"`
	ActionsExecuted []ActionExecutionEntry `json:"actions_executed"`
	At              time.Time              `json:"at"`
	DurationMs      int64                  `json:"duration_ms"`
}

// ActionExecutionEntry records the outcome of a single action within a rule execution.
type ActionExecutionEntry struct {
	Type   ActionType `json:"type"`
	Result string     `json:"result"` // "success" or "error"
	Error  string     `json:"error,omitempty"`
}

// RuleListOptions filters rules in list operations.
type RuleListOptions struct {
	Status ResourceStatus // filter by status; empty means all
	ListOptions
}
