package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Filter is a reusable, named condition set. Sources, sinks, and rules
// reference filters by ID rather than embedding conditions inline.
type Filter struct {
	ID          uuid.UUID   `json:"id"`
	ShortID     string      `json:"short_id"`
	RepoID      uuid.UUID   `json:"repo_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Mode        FilterMode  `json:"mode"` // "all" (AND) or "any" (OR)
	Conditions  []Condition `json:"conditions"`
	CreatedBy   uuid.UUID   `json:"created_by"`
	Version     int         `json:"version"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Condition is a single predicate within a filter. The field is a dot-path
// (e.g. "node.type", "node.metadata.priority"), op is one of the ConditionOp
// constants, and value is a flexible JSON value whose type depends on the
// operator.
type Condition struct {
	Field string          `json:"field"`
	Op    ConditionOp     `json:"op"`
	Value json.RawMessage `json:"value"`
}

// MaxFilterConditions is the maximum number of conditions a single filter may
// declare.
const MaxFilterConditions = 50

// ValidateConditions checks that a filter's conditions are well-formed: the
// count is within MaxFilterConditions and every condition carries a field and a
// recognised operator. The rule lives in core so every entry point that accepts
// filter conditions (the filter module and the repo flat-URL dispatch) validates
// identically.
func ValidateConditions(conditions []Condition) error {
	if len(conditions) > MaxFilterConditions {
		return fmt.Errorf("too many conditions: maximum is %d", MaxFilterConditions)
	}
	for i, c := range conditions {
		if c.Field == "" {
			return fmt.Errorf("condition %d: field is required", i)
		}
		if !c.Op.Valid() {
			return fmt.Errorf("condition %d: invalid operator '%s'", i, c.Op)
		}
	}
	return nil
}

// ConditionResult captures the outcome of evaluating a single condition
// against a sample payload. Used by the filter test endpoint.
type ConditionResult struct {
	Field  string      `json:"field"`
	Op     ConditionOp `json:"op"`
	Result bool        `json:"result"`
}

// FilterTestResult is the response from the filter test endpoint.
type FilterTestResult struct {
	Matches          bool              `json:"matches"`
	ConditionResults []ConditionResult `json:"condition_results"`
}
