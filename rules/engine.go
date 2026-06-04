package rules

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/filter"
	"github.com/upspeak/upspeak/jobs"
)

// allRepoEventsSubject is the wildcard subject the engine subscribes to. It
// captures every domain event from every repository: repo.{id}.events.{Type}.
const allRepoEventsSubject = "repo.*.events.>"

// Engine evaluates rules against domain events and executes their actions. It
// subscribes to all repository events via core NATS fan-out (best-effort), loads
// the active rules matching each event, evaluates their filter conditions, and
// fires actions for matching rules.
//
// The engine is started from main.go after archive wiring (not via the module's
// MsgHandlers) so its callback never runs before dependencies are set.
type Engine struct {
	archive core.Archive
	pub     app.Publisher
	sub     app.Subscriber
	logger  *slog.Logger
}

// NewEngine constructs a rules engine with its dependencies.
func NewEngine(archive core.Archive, pub app.Publisher, sub app.Subscriber, logger *slog.Logger) *Engine {
	return &Engine{archive: archive, pub: pub, sub: sub, logger: logger}
}

// Start subscribes the engine to all repository events. It returns an error if
// the subscription cannot be established.
func (e *Engine) Start() error {
	if err := e.sub.Subscribe(allRepoEventsSubject, e.handleEvent); err != nil {
		return fmt.Errorf("failed to subscribe rules engine: %w", err)
	}
	e.logger.Info("Rules engine subscribed", "subject", allRepoEventsSubject)
	return nil
}

// handleEvent processes a single domain event: it loads active rules for the
// event type, evaluates their triggers, and fires matching rules. Errors are
// logged rather than propagated, as event delivery is fire-and-forget.
func (e *Engine) handleEvent(_ string, data []byte) {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		e.logger.Error("Failed to unmarshal event", "error", err)
		return
	}

	// Ignore the engine's own administrative and operational meta-events to
	// avoid reacting to rule lifecycle changes and self-triggering loops.
	if isMetaEvent(evt.Type) {
		return
	}

	rules, err := e.archive.GetActiveRulesForEvent(evt.RepoID, evt.Type)
	if err != nil {
		e.logger.Error("Failed to load active rules", "repo_id", evt.RepoID, "event", evt.Type, "error", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	// Decode the event payload once into a generic map for filter evaluation.
	// The payload shape (e.g. {"node": {...}}) matches the dot-paths used by
	// filter conditions, so it is passed to the filter engine unchanged.
	var payload map[string]any
	if len(evt.Payload) > 0 {
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			e.logger.Error("Failed to unmarshal event payload", "event", evt.Type, "error", err)
			return
		}
	}

	for i := range rules {
		e.evaluateAndFire(&rules[i], &evt, payload)
	}
}

// evaluateAndFire evaluates a single rule's trigger filters against the event
// payload and, if they all match, executes the rule's actions and records the
// execution.
func (e *Engine) evaluateAndFire(rule *core.Rule, evt *core.Event, payload map[string]any) {
	if !e.evaluateTrigger(rule, payload) {
		return
	}

	start := time.Now()
	entries := e.executeActions(rule, evt, payload)
	duration := time.Since(start)

	exec := &core.RuleExecution{
		RuleID:          rule.ID,
		EventID:         evt.ID,
		EventType:       evt.Type,
		ActionsExecuted: entries,
		At:              start.UTC(),
		DurationMs:      duration.Milliseconds(),
	}
	if err := e.archive.SaveRuleExecution(exec); err != nil {
		e.logger.Error("Failed to record rule execution", "rule_id", rule.ID, "error", err)
	}

	e.publishTriggered(rule, evt, exec)
}

// evaluateTrigger reports whether every filter referenced by the rule's trigger
// matches the event payload. Filters combine with AND semantics: a single
// non-matching (or unresolvable) filter blocks the rule. A trigger with no
// filters matches on event type alone.
func (e *Engine) evaluateTrigger(rule *core.Rule, payload map[string]any) bool {
	for _, fid := range rule.Trigger.FilterIDs {
		f, err := e.archive.GetFilter(fid)
		if err != nil {
			e.logger.Error("Failed to load rule filter", "rule_id", rule.ID, "filter_id", fid, "error", err)
			return false
		}
		if !filter.Evaluate(f, payload).Matches {
			return false
		}
	}
	return true
}

// executeActions runs each of the rule's actions in order and returns an entry
// describing the outcome of each. Action failures are recorded but do not stop
// subsequent actions from running.
func (e *Engine) executeActions(rule *core.Rule, evt *core.Event, payload map[string]any) []core.ActionExecutionEntry {
	entries := make([]core.ActionExecutionEntry, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		entry := core.ActionExecutionEntry{Type: action.Type, Result: "success"}
		if err := e.executeAction(rule, action, evt, payload); err != nil {
			entry.Result = "error"
			entry.Error = err.Error()
			e.logger.Error("Rule action failed", "rule_id", rule.ID, "action", action.Type, "error", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// executeAction dispatches a single action. The job-producing actions
// (collect, publish, webhook) are enqueued through the job system, reusing its
// async execution, retry, and history machinery — the action's Params are passed
// straight through as the job params. Graph-mutating actions (enrich, relate,
// annotate) are not yet implemented and record an explicit unsupported error.
func (e *Engine) executeAction(rule *core.Rule, action core.RuleAction, _ *core.Event, _ map[string]any) error {
	switch action.Type {
	case core.ActionCollect:
		return e.enqueueJob(rule, core.JobCollect, action.Params)
	case core.ActionPublish:
		return e.enqueueJob(rule, core.JobPublish, action.Params)
	case core.ActionWebhook:
		return e.enqueueJob(rule, core.JobWebhook, action.Params)
	case core.ActionEnrich, core.ActionRelate, core.ActionAnnotate:
		return fmt.Errorf("action type %q is not yet implemented", action.Type)
	default:
		return fmt.Errorf("unknown action type %q", action.Type)
	}
}

// enqueueJob creates an async job for an action, reusing the rule's repository
// and the engine's publisher.
func (e *Engine) enqueueJob(rule *core.Rule, jobType core.JobType, params json.RawMessage) error {
	if _, err := jobs.CreateJob(e.archive, e.pub, rule.RepoID, rule.CreatedBy, jobType, params); err != nil {
		return fmt.Errorf("failed to enqueue %s job: %w", jobType, err)
	}
	return nil
}

// publishTriggered emits a RuleTriggered operational event recording that a rule
// fired. Fire-and-forget: failures are logged, never block.
func (e *Engine) publishTriggered(rule *core.Rule, evt *core.Event, exec *core.RuleExecution) {
	if e.pub == nil {
		return
	}
	payload := map[string]any{
		"rule_id":    rule.ID,
		"rule_short": rule.ShortID,
		"event_id":   evt.ID,
		"event_type": evt.Type,
		"execution":  exec,
	}
	outEvt, err := core.NewEvent(core.EventRuleTriggered, rule.RepoID, payload)
	if err != nil {
		e.logger.Error("Failed to create RuleTriggered event", "error", err)
		return
	}
	out, err := json.Marshal(outEvt)
	if err != nil {
		e.logger.Error("Failed to marshal RuleTriggered event", "error", err)
		return
	}
	if err := e.pub.Publish(outEvt.Subject(), out); err != nil {
		e.logger.Error("Failed to publish RuleTriggered event", "subject", outEvt.Subject(), "error", err)
	}
}

// isMetaEvent reports whether an event type is produced by rule administration
// or the engine itself, and therefore must not trigger rule evaluation (prevents
// self-referential loops).
func isMetaEvent(t core.EventType) bool {
	switch t {
	case core.EventRuleCreated, core.EventRuleUpdated, core.EventRuleDeleted, core.EventRuleTriggered:
		return true
	default:
		return false
	}
}
