package core

// EventType represents output event types published to JetStream.
type EventType string

// Knowledge graph output events.
const (
	EventNodeCreated       EventType = "NodeCreated"
	EventNodeUpdated       EventType = "NodeUpdated"
	EventNodePatched       EventType = "NodePatched"
	EventNodeDeleted       EventType = "NodeDeleted"
	EventEdgeCreated       EventType = "EdgeCreated"
	EventEdgeUpdated       EventType = "EdgeUpdated"
	EventEdgeDeleted       EventType = "EdgeDeleted"
	EventThreadCreated     EventType = "ThreadCreated"
	EventThreadUpdated     EventType = "ThreadUpdated"
	EventThreadDeleted     EventType = "ThreadDeleted"
	EventThreadNodeAdded   EventType = "ThreadNodeAdded"
	EventThreadNodeRemoved EventType = "ThreadNodeRemoved"
	EventAnnotationCreated EventType = "AnnotationCreated"
	EventAnnotationUpdated EventType = "AnnotationUpdated"
	EventAnnotationDeleted EventType = "AnnotationDeleted"
)

// Administrative output events.
const (
	EventSourceCreated   EventType = "SourceCreated"
	EventSourceUpdated   EventType = "SourceUpdated"
	EventSourceDeleted   EventType = "SourceDeleted"
	EventSinkCreated     EventType = "SinkCreated"
	EventSinkUpdated     EventType = "SinkUpdated"
	EventSinkDeleted     EventType = "SinkDeleted"
	EventFilterCreated   EventType = "FilterCreated"
	EventFilterUpdated   EventType = "FilterUpdated"
	EventFilterDeleted   EventType = "FilterDeleted"
	EventRuleCreated     EventType = "RuleCreated"
	EventRuleUpdated     EventType = "RuleUpdated"
	EventRuleDeleted     EventType = "RuleDeleted"
	EventScheduleCreated EventType = "ScheduleCreated"
	EventScheduleUpdated EventType = "ScheduleUpdated"
	EventScheduleDeleted EventType = "ScheduleDeleted"
	EventRepoCreated     EventType = "RepoCreated"
	EventRepoUpdated     EventType = "RepoUpdated"
	EventRepoDeleted     EventType = "RepoDeleted"
)

// Operational output events.
const (
	EventCollectionCompleted EventType = "CollectionCompleted"
	EventPublishCompleted    EventType = "PublishCompleted"
	EventRuleTriggered       EventType = "RuleTriggered"
	EventSyncCompleted       EventType = "SyncCompleted"
	EventConflictDetected    EventType = "ConflictDetected"
)

// InputEventType represents command events processed by HandleInputEvent.
type InputEventType string

// Input event types.
const (
	InputCreateNode       InputEventType = "CreateNode"
	InputUpdateNode       InputEventType = "UpdateNode"
	InputPatchNode        InputEventType = "PatchNode"
	InputDeleteNode       InputEventType = "DeleteNode"
	InputCreateEdge       InputEventType = "CreateEdge"
	InputUpdateEdge       InputEventType = "UpdateEdge"
	InputDeleteEdge       InputEventType = "DeleteEdge"
	InputCreateThread     InputEventType = "CreateThread"
	InputUpdateThread     InputEventType = "UpdateThread"
	InputDeleteThread     InputEventType = "DeleteThread"
	InputAddThreadNode    InputEventType = "AddThreadNode"
	InputRemoveThreadNode InputEventType = "RemoveThreadNode"
	InputCreateAnnotation InputEventType = "CreateAnnotation"
	InputUpdateAnnotation InputEventType = "UpdateAnnotation"
	InputDeleteAnnotation InputEventType = "DeleteAnnotation"
)

// ConnectorType identifies the type of a source or sink connector.
type ConnectorType string

// Connector type constants.
const (
	ConnectorRSS       ConnectorType = "rss"
	ConnectorDiscourse ConnectorType = "discourse"
	ConnectorMatrix    ConnectorType = "matrix"
	ConnectorFediverse ConnectorType = "fediverse"
	ConnectorWebhook   ConnectorType = "webhook"
	ConnectorEmail     ConnectorType = "email"
	ConnectorWebpage   ConnectorType = "webpage"
	ConnectorRepo      ConnectorType = "repo"
	ConnectorUpspeak   ConnectorType = "upspeak"
)

// JobType identifies the type of an async job.
type JobType string

// Job type constants.
const (
	JobCollect JobType = "collect"
	JobPublish JobType = "publish"
	JobSync    JobType = "sync"
	JobWebhook JobType = "webhook"
)

// ActionType identifies the type of a rule action.
type ActionType string

// Action type constants.
const (
	ActionEnrich   ActionType = "enrich"
	ActionRelate   ActionType = "relate"
	ActionAnnotate ActionType = "annotate"
	ActionCollect  ActionType = "collect"
	ActionPublish  ActionType = "publish"
	ActionWebhook  ActionType = "webhook"
)

// FilterMode controls how multiple conditions in a filter are combined.
type FilterMode string

// Filter mode constants.
const (
	FilterModeAll FilterMode = "all" // AND: every condition must match
	FilterModeAny FilterMode = "any" // OR: at least one must match
)

// ConditionOp is an operator used in filter conditions.
type ConditionOp string

// Condition operator constants.
const (
	OpEq          ConditionOp = "eq"
	OpNeq         ConditionOp = "neq"
	OpContains    ConditionOp = "contains"
	OpNotContains ConditionOp = "not_contains"
	OpStartsWith  ConditionOp = "starts_with"
	OpEndsWith    ConditionOp = "ends_with"
	OpIn          ConditionOp = "in"
	OpNotIn       ConditionOp = "not_in"
	OpGt          ConditionOp = "gt"
	OpLt          ConditionOp = "lt"
	OpGte         ConditionOp = "gte"
	OpLte         ConditionOp = "lte"
	OpExists      ConditionOp = "exists"
	OpNotExists   ConditionOp = "not_exists"
	OpMatches     ConditionOp = "matches"
)

// ResourceStatus tracks the operational state of a source or sink.
type ResourceStatus string

// Resource status constants.
const (
	StatusActive      ResourceStatus = "active"
	StatusPaused      ResourceStatus = "paused"
	StatusError       ResourceStatus = "error"
	StatusRateLimited ResourceStatus = "rate_limited"
)

// JobStatus tracks the lifecycle of an async job.
type JobStatus string

// Job status constants.
const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// RateLimit configures per-source/sink rate limiting.
type RateLimit struct {
	MaxRequests       int `json:"max_requests"`
	WindowSeconds     int `json:"window_seconds"`
	RetryAfterSeconds int `json:"retry_after_seconds"`
}
