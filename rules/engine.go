package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/filter"
	"github.com/upspeak/upspeak/jobs"
)

// maxRuleHops bounds how many chained rule-engine reactions a single originating
// event may spawn. An event whose Hops count reaches this limit is dropped
// without evaluation, breaking action cascades that would otherwise loop.
const maxRuleHops = 5

// fetchBatchSize and fetchTimeout control the durable consumer fetch loop.
const (
	fetchBatchSize = 10
	fetchTimeout   = 5 * time.Second
)

// ackDecision is the disposition of a consumed event message.
type ackDecision int

const (
	ackOK    ackDecision = iota // processed (or intentionally skipped): acknowledge
	ackRetry                    // transient failure before side effects: redeliver
	ackTerm                     // permanently undeliverable (poison): drop
)

// Engine evaluates rules against domain events and executes their actions. It is
// a durable JetStream pull consumer on the global repository-events stream: it
// fetches events with explicit acknowledgement, loads the active rules matching
// each event, evaluates their filter conditions, and fires actions for matching
// rules. Durability means events published while the engine is down are
// delivered on restart rather than lost.
//
// The engine is run from main.go after archive wiring so its loop never executes
// before dependencies are set.
type Engine struct {
	archive  core.Archive
	pub      app.Publisher
	consumer app.Consumer
	logger   *slog.Logger
}

// NewEngine constructs a rules engine with its dependencies.
func NewEngine(archive core.Archive, pub app.Publisher, consumer app.Consumer, logger *slog.Logger) *Engine {
	return &Engine{archive: archive, pub: pub, consumer: consumer, logger: logger}
}

// Run starts the engine's consume loop. It blocks until the context is cancelled.
func (e *Engine) Run(ctx context.Context) {
	e.logger.Info("Rules engine started")

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("Rules engine stopping")
			return
		default:
		}

		msgs, err := e.consumer.Fetch(fetchBatchSize, fetchTimeout)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue // No messages available, try again.
			}
			e.logger.Error("Rules engine fetch failed", "error", err)
			continue
		}

		for _, msg := range msgs {
			e.processMessage(msg)
		}
	}
}

// processMessage dispatches a single event message and acknowledges it according
// to the outcome: redeliver on transient failure (before any side effects),
// terminate on a poison message, otherwise acknowledge.
func (e *Engine) processMessage(msg *app.Msg) {
	switch e.dispatch(msg.Data) {
	case ackRetry:
		_ = msg.Nak()
	case ackTerm:
		_ = msg.Term()
	default:
		_ = msg.Ack()
	}
}

// dispatch processes a single domain event: it loads active rules for the event
// type, evaluates their triggers, and fires matching rules. It returns the
// acknowledgement disposition for the message.
func (e *Engine) dispatch(data []byte) ackDecision {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		e.logger.Error("Failed to unmarshal event", "error", err)
		return ackTerm // Malformed envelope; redelivery cannot help.
	}

	// Ignore the engine's own meta-events (rule lifecycle/RuleTriggered) and cap
	// reaction cascades to avoid self-triggering loops.
	if isMetaEvent(evt.Type) {
		return ackOK
	}
	if evt.Hops >= maxRuleHops {
		e.logger.Warn("Dropping event exceeding max rule hops", "event", evt.Type, "hops", evt.Hops)
		return ackOK
	}

	rules, err := e.archive.GetActiveRulesForEvent(evt.RepoID, evt.Type)
	if err != nil {
		// Transient: no actions have run yet, so redeliver rather than drop.
		e.logger.Error("Failed to load active rules", "repo_id", evt.RepoID, "event", evt.Type, "error", err)
		return ackRetry
	}
	if len(rules) == 0 {
		return ackOK
	}

	// Decode the event payload once into a generic map for filter evaluation and
	// action targeting. Created events nest the entity under a stable key (e.g.
	// "node") that matches filter dot-paths; Updated/Patched events nest it under
	// "updated_node". normaliseEventPayload aliases the latter to the former so a
	// single rule filter (e.g. node.type) works across create and update triggers.
	var payload map[string]any
	if len(evt.Payload) > 0 {
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			e.logger.Error("Failed to unmarshal event payload", "event", evt.Type, "error", err)
			return ackTerm // Malformed payload; redelivery cannot help.
		}
	}
	normaliseEventPayload(payload)

	for i := range rules {
		e.evaluateAndFire(&rules[i], &evt, payload)
	}
	return ackOK
}

// evaluateAndFire evaluates a single rule's trigger filters against the event
// payload and, if they all match, executes the rule's actions and records the
// execution.
func (e *Engine) evaluateAndFire(rule *core.Rule, evt *core.Event, payload map[string]any) {
	if !e.evaluateTrigger(rule, payload) {
		return
	}

	start := time.Now()
	entries := e.executeActions(rule, payload)
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
func (e *Engine) executeActions(rule *core.Rule, payload map[string]any) []core.ActionExecutionEntry {
	entries := make([]core.ActionExecutionEntry, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		entry := core.ActionExecutionEntry{Type: action.Type, Result: "success"}
		if err := e.executeAction(rule, action, payload); err != nil {
			entry.Result = "error"
			entry.Error = err.Error()
			e.logger.Error("Rule action failed", "rule_id", rule.ID, "action", action.Type, "error", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// executeAction dispatches a single action. Job-producing actions (collect,
// publish, webhook) are enqueued through the job system, reusing its async
// execution, retry, and history machinery — the action's Params pass straight
// through as the job params. Graph-mutating actions (enrich, relate, annotate)
// operate on the triggering node and write directly to the archive.
//
// Graph-mutating actions deliberately do NOT publish downstream domain events:
// without a loop/depth guard, re-publishing (e.g. a NodeUpdated from enrich)
// could re-trigger the same rule. The RuleTriggered event still records that the
// rule fired. Surfacing these writes to other consumers is a follow-up that
// depends on loop prevention.
func (e *Engine) executeAction(rule *core.Rule, action core.RuleAction, payload map[string]any) error {
	switch action.Type {
	case core.ActionCollect:
		return e.enqueueConnectorJob(rule, core.JobCollect, action.Params, "source_id", "source")
	case core.ActionPublish:
		return e.enqueueConnectorJob(rule, core.JobPublish, action.Params, "sink_id", "sink")
	case core.ActionWebhook:
		return e.enqueueJob(rule, core.JobWebhook, action.Params)
	case core.ActionEnrich:
		return e.enrichNode(rule, payload, action.Params)
	case core.ActionRelate:
		return e.relateNode(rule, payload, action.Params)
	case core.ActionAnnotate:
		return e.annotateNode(rule, payload, action.Params)
	default:
		return fmt.Errorf("unknown action type %q", action.Type)
	}
}

// enqueueConnectorJob enqueues a collect/publish job, enforcing that the
// referenced connector (source or sink) belongs to the rule's repository. It
// resolves refField (a UUID or short ID) within the rule's repo, rejects any
// reference that does not resolve to the expected entity type in that repo, and
// rewrites the param to the canonical UUID so the job runner cannot be steered
// at another repository's connector.
func (e *Engine) enqueueConnectorJob(rule *core.Rule, jobType core.JobType, raw json.RawMessage, refField, expectedType string) error {
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return fmt.Errorf("invalid %s action params: %w", jobType, err)
	}
	ref, _ := params[refField].(string)
	if ref == "" {
		return fmt.Errorf("%s action requires %s", jobType, refField)
	}

	id, entityType, err := e.archive.ResolveRef(rule.RepoID, ref)
	if err != nil || entityType != expectedType {
		return fmt.Errorf("%s %q does not resolve to a %s in this repository", refField, ref, expectedType)
	}
	params[refField] = id.String()

	resolved, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal %s job params: %w", jobType, err)
	}
	return e.enqueueJob(rule, jobType, resolved)
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
	// Propagate the hop count so any reaction chain is bounded by maxRuleHops.
	outEvt.Hops = evt.Hops + 1
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
