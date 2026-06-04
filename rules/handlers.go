package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// maxActions is the maximum number of actions allowed per rule.
const maxActions = 20

// createRuleRequest is the JSON body for creating a rule.
type createRuleRequest struct {
	Name    string            `json:"name"`
	Trigger core.RuleTrigger  `json:"trigger"`
	Actions []core.RuleAction `json:"actions"`
	Status  string            `json:"status,omitempty"`
}

// updateRuleRequest is the JSON body for updating a rule. Fields are pointers so
// only those present in the request body are applied.
type updateRuleRequest struct {
	Name    *string            `json:"name,omitempty"`
	Trigger *core.RuleTrigger  `json:"trigger,omitempty"`
	Actions *[]core.RuleAction `json:"actions,omitempty"`
	Status  *string            `json:"status,omitempty"`
}

// createRuleHandler handles POST /repos/{repo_ref}/rules.
func (m *Module) createRuleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req createRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		status := core.StatusActive
		if req.Status != "" {
			if !isUserSettableStatus(req.Status) {
				api.WriteError(w, http.StatusBadRequest, "invalid_field", "status must be 'active' or 'paused'")
				return
			}
			status = core.ResourceStatus(req.Status)
		}

		if err := validateRule(req.Name, req.Trigger, req.Actions); err != nil {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}

		// Verify referenced filters exist and belong to this repository.
		if err := m.validateFilterRefs(repo.ID, req.Trigger.FilterIDs); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_filter_ref", err.Error())
			return
		}

		rule := &core.Rule{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Name:      req.Name,
			Trigger:   req.Trigger,
			Actions:   req.Actions,
			Status:    status,
			Version:   0,
			CreatedBy: defaultOwnerID,
		}

		if err := m.archive.SaveRule(rule); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save rule")
			return
		}

		m.publishEvent(repo.ID, core.EventRuleCreated, rule)
		api.SetETag(w, rule.Version)
		api.WriteJSON(w, http.StatusCreated, rule)
	}
}

// listRulesHandler handles GET /repos/{repo_ref}/rules.
func (m *Module) listRulesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.RuleListOptions{ListOptions: api.ParsePagination(r)}
		if st := r.URL.Query().Get("status"); st != "" {
			opts.Status = core.ResourceStatus(st)
		}

		ruleList, total, err := m.archive.ListRules(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list rules")
			return
		}

		api.WriteList(w, ruleList, total, opts.ListOptions)
	}
}

// getRuleHandler handles GET /repos/{repo_ref}/rules/{rule_ref}.
func (m *Module) getRuleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		rule, err := m.resolveRule(w, repo.ID, r.PathValue("rule_ref"))
		if err != nil {
			return
		}

		api.SetETag(w, rule.Version)
		api.WriteJSON(w, http.StatusOK, rule)
	}
}

// updateRuleHandler handles PUT /repos/{repo_ref}/rules/{rule_ref}.
func (m *Module) updateRuleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		ifMatch := api.ParseIfMatch(r)
		if ifMatch == 0 {
			api.WriteError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match header is required for updates")
			return
		}
		if ifMatch == -1 {
			api.WriteError(w, http.StatusPreconditionFailed, "invalid_if_match", "If-Match header is malformed")
			return
		}

		rule, err := m.resolveRule(w, repo.ID, r.PathValue("rule_ref"))
		if err != nil {
			return
		}

		if rule.Version != ifMatch {
			api.WriteError(w, http.StatusPreconditionFailed, "version_mismatch", "If-Match version does not match current version")
			return
		}

		r = api.LimitedBody(w, r)
		var req updateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name != nil {
			rule.Name = *req.Name
		}
		if req.Trigger != nil {
			rule.Trigger = *req.Trigger
		}
		if req.Actions != nil {
			rule.Actions = *req.Actions
		}
		if req.Status != nil {
			if !isUserSettableStatus(*req.Status) {
				api.WriteError(w, http.StatusBadRequest, "invalid_field", "status must be 'active' or 'paused'")
				return
			}
			rule.Status = core.ResourceStatus(*req.Status)
		}

		// Validate the merged rule before persisting.
		if err := validateRule(rule.Name, rule.Trigger, rule.Actions); err != nil {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if err := m.validateFilterRefs(repo.ID, rule.Trigger.FilterIDs); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_filter_ref", err.Error())
			return
		}

		if err := m.archive.SaveRule(rule); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Rule was modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save rule")
			return
		}

		m.publishEvent(repo.ID, core.EventRuleUpdated, rule)
		api.SetETag(w, rule.Version)
		api.WriteJSON(w, http.StatusOK, rule)
	}
}

// deleteRuleHandler handles DELETE /repos/{repo_ref}/rules/{rule_ref}.
func (m *Module) deleteRuleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		rule, err := m.resolveRule(w, repo.ID, r.PathValue("rule_ref"))
		if err != nil {
			return
		}

		if err := m.archive.DeleteRule(rule.ID); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete rule")
			return
		}

		m.publishEvent(repo.ID, core.EventRuleDeleted, rule)
		w.WriteHeader(http.StatusNoContent)
	}
}

// listHistoryHandler handles GET /repos/{repo_ref}/rules/{rule_ref}/history,
// returning the rule's execution records.
func (m *Module) listHistoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		rule, err := m.resolveRule(w, repo.ID, r.PathValue("rule_ref"))
		if err != nil {
			return
		}

		opts := api.ParsePagination(r)
		execs, total, err := m.archive.ListRuleExecutions(rule.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list rule executions")
			return
		}

		api.WriteList(w, execs, total, opts)
	}
}

// pauseRuleHandler handles POST /repos/{repo_ref}/rules/{rule_ref}/pause,
// transitioning the rule to the paused state so the engine stops evaluating it.
func (m *Module) pauseRuleHandler() http.HandlerFunc {
	return m.setStatusHandler(core.StatusPaused, core.EventRuleUpdated)
}

// resumeRuleHandler handles POST /repos/{repo_ref}/rules/{rule_ref}/resume,
// transitioning the rule back to the active state.
func (m *Module) resumeRuleHandler() http.HandlerFunc {
	return m.setStatusHandler(core.StatusActive, core.EventRuleUpdated)
}

// setStatusHandler builds a handler that sets a rule to the given status. It is
// idempotent: setting a rule to its current status succeeds without a write.
func (m *Module) setStatusHandler(status core.ResourceStatus, event core.EventType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		rule, err := m.resolveRule(w, repo.ID, r.PathValue("rule_ref"))
		if err != nil {
			return
		}

		if rule.Status == status {
			api.SetETag(w, rule.Version)
			api.WriteJSON(w, http.StatusOK, rule)
			return
		}

		rule.Status = status
		if err := m.archive.SaveRule(rule); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Rule was modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update rule status")
			return
		}

		m.publishEvent(repo.ID, event, rule)
		api.SetETag(w, rule.Version)
		api.WriteJSON(w, http.StatusOK, rule)
	}
}

// validActionTypes is the set of action types a rule may declare.
var validActionTypes = map[core.ActionType]bool{
	core.ActionEnrich:   true,
	core.ActionRelate:   true,
	core.ActionAnnotate: true,
	core.ActionCollect:  true,
	core.ActionPublish:  true,
	core.ActionWebhook:  true,
}

// isUserSettableStatus reports whether a status may be set directly by a client.
// Operational states (error, rate_limited) are reserved for the engine.
func isUserSettableStatus(s string) bool {
	return s == string(core.StatusActive) || s == string(core.StatusPaused)
}

// validateRule checks the structural validity of a rule's user-supplied fields.
func validateRule(name string, trigger core.RuleTrigger, actions []core.RuleAction) error {
	if name == "" {
		return errors.New("name is required")
	}
	if trigger.Event == "" {
		return errors.New("trigger.event is required")
	}
	if len(actions) == 0 {
		return errors.New("at least one action is required")
	}
	if len(actions) > maxActions {
		return fmt.Errorf("too many actions: maximum is %d", maxActions)
	}
	for i, action := range actions {
		if !validActionTypes[action.Type] {
			return fmt.Errorf("action %d: invalid action type '%s'", i, action.Type)
		}
		if err := validateActionParams(action); err != nil {
			return fmt.Errorf("action %d: %w", i, err)
		}
	}
	return nil
}

// validateActionParams checks that an action's params carry the fields its type
// requires. This is structural validation only; repository ownership of
// referenced connectors is enforced by the engine at execution time.
func validateActionParams(action core.RuleAction) error {
	switch action.Type {
	case core.ActionCollect:
		return requireStringField(action.Params, "source_id")
	case core.ActionPublish:
		return requireStringField(action.Params, "sink_id")
	case core.ActionWebhook:
		var p struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(action.Params, &p); err != nil {
			return fmt.Errorf("invalid webhook params: %w", err)
		}
		if p.URL == "" {
			return errors.New("webhook action requires url")
		}
		if u, err := url.Parse(p.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("webhook url must be a valid http(s) URL")
		}
	case core.ActionEnrich:
		return requireStringField(action.Params, "metadata_key")
	case core.ActionRelate:
		var p struct {
			TargetNodeID   string `json:"target_node_id"`
			TargetThreadID string `json:"target_thread_id"`
		}
		if err := json.Unmarshal(action.Params, &p); err != nil {
			return fmt.Errorf("invalid relate params: %w", err)
		}
		if p.TargetNodeID == "" && p.TargetThreadID == "" {
			return errors.New("relate action requires target_node_id or target_thread_id")
		}
	case core.ActionAnnotate:
		return requireStringField(action.Params, "motivation")
	}
	return nil
}

// requireStringField verifies that raw is a JSON object with a non-empty string
// value at the given field.
func requireStringField(raw json.RawMessage, field string) error {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	if s, ok := m[field].(string); !ok || s == "" {
		return fmt.Errorf("action requires %s", field)
	}
	return nil
}

// validateFilterRefs verifies that every referenced filter exists and belongs to
// the given repository, preventing rules with dangling or cross-repo filter refs.
func (m *Module) validateFilterRefs(repoID uuid.UUID, filterIDs []uuid.UUID) error {
	for _, fid := range filterIDs {
		filter, err := m.archive.GetFilter(fid)
		if err != nil || filter.RepoID != repoID {
			return fmt.Errorf("filter %s not found in this repository", fid)
		}
	}
	return nil
}
