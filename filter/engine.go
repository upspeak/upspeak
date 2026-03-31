// Package filter provides the condition evaluation engine used by filters,
// rules, and connectors. It evaluates conditions against entity payloads
// represented as map[string]any.
package filter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/upspeak/upspeak/core"
)

// Evaluate runs all conditions in a filter against a payload and returns
// whether the filter matches and per-condition results. The payload is a
// map[string]any representing the entity (e.g. {"node": {...}, "edge": {...}}).
func Evaluate(filter *core.Filter, payload map[string]any) core.FilterTestResult {
	results := make([]core.ConditionResult, len(filter.Conditions))

	matchCount := 0
	for i, cond := range filter.Conditions {
		matched := evaluateCondition(cond, payload)
		results[i] = core.ConditionResult{
			Field:  cond.Field,
			Op:     cond.Op,
			Result: matched,
		}
		if matched {
			matchCount++
		}
	}

	var matches bool
	switch filter.Mode {
	case core.FilterModeAny:
		matches = matchCount > 0
	default: // "all" or unset
		matches = matchCount == len(filter.Conditions)
	}

	// Empty conditions: "all" mode matches (vacuous truth), "any" mode does not.
	if len(filter.Conditions) == 0 {
		matches = filter.Mode != core.FilterModeAny
	}

	return core.FilterTestResult{
		Matches:          matches,
		ConditionResults: results,
	}
}

// evaluateCondition checks a single condition against the payload.
func evaluateCondition(cond core.Condition, payload map[string]any) bool {
	fieldVal, exists := resolvePath(cond.Field, payload)

	switch cond.Op {
	case core.OpExists:
		return exists
	case core.OpNotExists:
		return !exists
	}

	if !exists {
		return false
	}

	return applyOperator(cond.Op, fieldVal, cond.Value)
}

// resolvePath resolves a dot-path (e.g. "node.type", "node.metadata.priority")
// against a nested map. Metadata arrays are handled specially: if a segment
// resolves to a []any or []Metadata-like structure, it searches by key.
func resolvePath(path string, payload map[string]any) (any, bool) {
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return nil, false
	}

	var current any = payload

	for i, seg := range segments {
		switch v := current.(type) {
		case map[string]any:
			val, ok := v[seg]
			if !ok {
				return nil, false
			}
			current = val

		case []any:
			// Handle metadata-style arrays: [{key: "k", value: "v"}, ...]
			found, ok := findInMetadataArray(seg, v)
			if !ok {
				return nil, false
			}
			current = found

		case []map[string]any:
			found, ok := findInMetadataMapArray(seg, v)
			if !ok {
				return nil, false
			}
			current = found

		default:
			// Can't navigate further into a non-map/non-array value.
			if i < len(segments)-1 {
				return nil, false
			}
		}
	}

	return current, true
}

// findInMetadataArray searches a [{key: "k", value: v}, ...] style array
// for an entry whose "key" matches the segment, returning the "value".
func findInMetadataArray(key string, arr []any) (any, bool) {
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if k, ok := m["key"]; ok && fmt.Sprint(k) == key {
			return m["value"], true
		}
	}
	return nil, false
}

// findInMetadataMapArray is a typed variant for []map[string]any.
func findInMetadataMapArray(key string, arr []map[string]any) (any, bool) {
	for _, m := range arr {
		if k, ok := m["key"]; ok && fmt.Sprint(k) == key {
			return m["value"], true
		}
	}
	return nil, false
}

// applyOperator applies a comparison operator between a field value and a
// condition value. The condition value is raw JSON that gets unmarshalled
// as needed.
func applyOperator(op core.ConditionOp, fieldVal any, condValue json.RawMessage) bool {
	switch op {
	case core.OpEq:
		return compareEq(fieldVal, condValue)
	case core.OpNeq:
		return !compareEq(fieldVal, condValue)
	case core.OpContains:
		return stringOp(fieldVal, condValue, strings.Contains)
	case core.OpNotContains:
		return !stringOp(fieldVal, condValue, strings.Contains)
	case core.OpStartsWith:
		return stringOp(fieldVal, condValue, strings.HasPrefix)
	case core.OpEndsWith:
		return stringOp(fieldVal, condValue, strings.HasSuffix)
	case core.OpIn:
		return inOp(fieldVal, condValue, false)
	case core.OpNotIn:
		return inOp(fieldVal, condValue, true)
	case core.OpGt:
		return numericCmp(fieldVal, condValue) > 0
	case core.OpLt:
		return numericCmp(fieldVal, condValue) < 0
	case core.OpGte:
		return numericCmp(fieldVal, condValue) >= 0
	case core.OpLte:
		return numericCmp(fieldVal, condValue) <= 0
	case core.OpMatches:
		return matchesOp(fieldVal, condValue)
	default:
		return false
	}
}

// compareEq checks equality between a field value and a JSON condition value.
func compareEq(fieldVal any, condValue json.RawMessage) bool {
	// Unmarshal condition value to a generic type.
	var cv any
	if err := json.Unmarshal(condValue, &cv); err != nil {
		return false
	}

	return toComparable(fieldVal) == toComparable(cv)
}

// stringOp applies a string operation (contains, starts_with, ends_with).
func stringOp(fieldVal any, condValue json.RawMessage, fn func(string, string) bool) bool {
	fieldStr := fmt.Sprint(fieldVal)
	var cv string
	if err := json.Unmarshal(condValue, &cv); err != nil {
		return false
	}
	return fn(fieldStr, cv)
}

// inOp checks if fieldVal is (or is not) in the condition value array.
func inOp(fieldVal any, condValue json.RawMessage, negate bool) bool {
	var arr []any
	if err := json.Unmarshal(condValue, &arr); err != nil {
		return negate // If not an array, "not_in" is true, "in" is false.
	}

	fieldCmp := toComparable(fieldVal)
	for _, item := range arr {
		if toComparable(item) == fieldCmp {
			return !negate
		}
	}
	return negate
}

// numericCmp compares fieldVal and condValue numerically. Returns -1, 0, or 1.
// Returns 0 if either value is not numeric (no match for gt/lt/gte/lte).
func numericCmp(fieldVal any, condValue json.RawMessage) int {
	fv := toFloat64(fieldVal)
	var cv float64
	if err := json.Unmarshal(condValue, &cv); err != nil {
		return 0
	}
	if fv < cv {
		return -1
	}
	if fv > cv {
		return 1
	}
	return 0
}

// matchesOp applies a regex match.
func matchesOp(fieldVal any, condValue json.RawMessage) bool {
	var pattern string
	if err := json.Unmarshal(condValue, &pattern); err != nil {
		return false
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(fmt.Sprint(fieldVal))
}

// toComparable converts a value to a string for equality comparison.
// This handles the fact that JSON numbers unmarshal as float64 while
// payload values may be strings, floats, or booleans.
func toComparable(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case json.Number:
		return val.String()
	case float64:
		// Format without trailing zeroes for cleaner comparison.
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case nil:
		return "<nil>"
	default:
		return fmt.Sprint(val)
	}
}

// toFloat64 converts a value to float64 for numeric comparisons.
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}
