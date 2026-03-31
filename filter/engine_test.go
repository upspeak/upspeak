package filter

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func rawJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestEvaluate_AllMode(t *testing.T) {
	f := &core.Filter{
		Mode: core.FilterModeAll,
		Conditions: []core.Condition{
			{Field: "node.type", Op: core.OpEq, Value: rawJSON("article")},
			{Field: "node.subject", Op: core.OpContains, Value: rawJSON("AI")},
		},
	}

	payload := map[string]any{
		"node": map[string]any{
			"type":    "article",
			"subject": "New AI governance framework",
		},
	}

	result := Evaluate(f, payload)
	if !result.Matches {
		t.Error("expected filter to match in 'all' mode")
	}
	if len(result.ConditionResults) != 2 {
		t.Fatalf("expected 2 condition results, got %d", len(result.ConditionResults))
	}
	for _, cr := range result.ConditionResults {
		if !cr.Result {
			t.Errorf("condition %s %s should have matched", cr.Field, cr.Op)
		}
	}
}

func TestEvaluate_AllMode_PartialMatch(t *testing.T) {
	f := &core.Filter{
		Mode: core.FilterModeAll,
		Conditions: []core.Condition{
			{Field: "node.type", Op: core.OpEq, Value: rawJSON("article")},
			{Field: "node.subject", Op: core.OpContains, Value: rawJSON("quantum")},
		},
	}

	payload := map[string]any{
		"node": map[string]any{
			"type":    "article",
			"subject": "New AI governance framework",
		},
	}

	result := Evaluate(f, payload)
	if result.Matches {
		t.Error("expected filter NOT to match in 'all' mode with partial match")
	}
	if !result.ConditionResults[0].Result {
		t.Error("first condition should have matched")
	}
	if result.ConditionResults[1].Result {
		t.Error("second condition should not have matched")
	}
}

func TestEvaluate_AnyMode(t *testing.T) {
	f := &core.Filter{
		Mode: core.FilterModeAny,
		Conditions: []core.Condition{
			{Field: "node.type", Op: core.OpEq, Value: rawJSON("video")},
			{Field: "node.subject", Op: core.OpContains, Value: rawJSON("AI")},
		},
	}

	payload := map[string]any{
		"node": map[string]any{
			"type":    "article",
			"subject": "New AI governance framework",
		},
	}

	result := Evaluate(f, payload)
	if !result.Matches {
		t.Error("expected filter to match in 'any' mode with one matching condition")
	}
}

func TestEvaluate_EmptyConditions(t *testing.T) {
	// "all" mode with no conditions = vacuous truth.
	fAll := &core.Filter{Mode: core.FilterModeAll, Conditions: []core.Condition{}}
	result := Evaluate(fAll, map[string]any{})
	if !result.Matches {
		t.Error("empty conditions in 'all' mode should match (vacuous truth)")
	}

	// "any" mode with no conditions = no match.
	fAny := &core.Filter{Mode: core.FilterModeAny, Conditions: []core.Condition{}}
	result = Evaluate(fAny, map[string]any{})
	if result.Matches {
		t.Error("empty conditions in 'any' mode should not match")
	}
}

func TestEvaluate_MetadataPath(t *testing.T) {
	f := &core.Filter{
		Mode: core.FilterModeAll,
		Conditions: []core.Condition{
			{Field: "node.metadata.priority", Op: core.OpEq, Value: rawJSON("high")},
		},
	}

	payload := map[string]any{
		"node": map[string]any{
			"metadata": []any{
				map[string]any{"key": "priority", "value": "high"},
				map[string]any{"key": "source", "value": "manual"},
			},
		},
	}

	result := Evaluate(f, payload)
	if !result.Matches {
		t.Error("expected metadata path resolution to match")
	}
}

func TestEvaluate_AllOperators(t *testing.T) {
	tests := []struct {
		name    string
		op      core.ConditionOp
		field   any
		value   any
		want    bool
	}{
		{"eq match", core.OpEq, "hello", "hello", true},
		{"eq no match", core.OpEq, "hello", "world", false},
		{"neq match", core.OpNeq, "hello", "world", true},
		{"neq no match", core.OpNeq, "hello", "hello", false},
		{"contains match", core.OpContains, "hello world", "world", true},
		{"contains no match", core.OpContains, "hello world", "foo", false},
		{"not_contains match", core.OpNotContains, "hello world", "foo", true},
		{"not_contains no match", core.OpNotContains, "hello world", "world", false},
		{"starts_with match", core.OpStartsWith, "hello world", "hello", true},
		{"starts_with no match", core.OpStartsWith, "hello world", "world", false},
		{"ends_with match", core.OpEndsWith, "hello world", "world", true},
		{"ends_with no match", core.OpEndsWith, "hello world", "hello", false},
		{"gt match", core.OpGt, float64(10), float64(5), true},
		{"gt no match", core.OpGt, float64(3), float64(5), false},
		{"lt match", core.OpLt, float64(3), float64(5), true},
		{"lt no match", core.OpLt, float64(10), float64(5), false},
		{"gte match equal", core.OpGte, float64(5), float64(5), true},
		{"gte match greater", core.OpGte, float64(6), float64(5), true},
		{"gte no match", core.OpGte, float64(4), float64(5), false},
		{"lte match equal", core.OpLte, float64(5), float64(5), true},
		{"lte match less", core.OpLte, float64(4), float64(5), true},
		{"lte no match", core.OpLte, float64(6), float64(5), false},
		{"matches match", core.OpMatches, "hello-123", "^hello-\\d+$", true},
		{"matches no match", core.OpMatches, "hello-abc", "^hello-\\d+$", false},
		{"eq numeric", core.OpEq, float64(42), float64(42), true},
		{"eq bool", core.OpEq, true, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &core.Filter{
				Mode: core.FilterModeAll,
				Conditions: []core.Condition{
					{Field: "val", Op: tc.op, Value: rawJSON(tc.value)},
				},
			}
			payload := map[string]any{"val": tc.field}
			result := Evaluate(f, payload)
			if result.Matches != tc.want {
				t.Errorf("op %s: got matches=%v, want %v", tc.op, result.Matches, tc.want)
			}
		})
	}
}

func TestEvaluate_InNotIn(t *testing.T) {
	tests := []struct {
		name  string
		op    core.ConditionOp
		field any
		value []string
		want  bool
	}{
		{"in match", core.OpIn, "article", []string{"article", "video"}, true},
		{"in no match", core.OpIn, "podcast", []string{"article", "video"}, false},
		{"not_in match", core.OpNotIn, "podcast", []string{"article", "video"}, true},
		{"not_in no match", core.OpNotIn, "article", []string{"article", "video"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &core.Filter{
				Mode: core.FilterModeAll,
				Conditions: []core.Condition{
					{Field: "val", Op: tc.op, Value: rawJSON(tc.value)},
				},
			}
			payload := map[string]any{"val": tc.field}
			result := Evaluate(f, payload)
			if result.Matches != tc.want {
				t.Errorf("op %s: got matches=%v, want %v", tc.op, result.Matches, tc.want)
			}
		})
	}
}

func TestEvaluate_ExistsNotExists(t *testing.T) {
	f := &core.Filter{
		Mode: core.FilterModeAll,
		Conditions: []core.Condition{
			{Field: "node.type", Op: core.OpExists},
			{Field: "node.missing", Op: core.OpNotExists},
		},
	}

	payload := map[string]any{
		"node": map[string]any{"type": "article"},
	}

	result := Evaluate(f, payload)
	if !result.Matches {
		t.Error("expected exists/not_exists to match")
	}
}

func TestResolvePath_DeepNested(t *testing.T) {
	payload := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "found",
			},
		},
	}

	val, ok := resolvePath("a.b.c", payload)
	if !ok || val != "found" {
		t.Errorf("expected 'found', got %v (ok=%v)", val, ok)
	}

	_, ok = resolvePath("a.b.d", payload)
	if ok {
		t.Error("expected missing path to return false")
	}
}
