package realtime

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// matchEvent reports whether the event should be delivered on this subscription.
// It applies channel scoping, then the optional event_type and node_type filters.
func (s *subscription) matchEvent(ev *core.Event) bool {
	if !s.matchChannel(ev) {
		return false
	}
	if !s.filter.matchEventType(ev) {
		return false
	}
	return s.filter.matchNodeType(ev)
}

// matchChannel reports whether the event falls within this subscription's
// channel scope. Stub channels (rule actions, jobs, sync) have no backing
// events yet and never match.
func (s *subscription) matchChannel(ev *core.Event) bool {
	switch s.kind {
	case channelRepoEvents:
		return ev.RepoID == s.repoID
	case channelNode:
		return ev.RepoID == s.repoID && eventReferencesNode(ev, s.entityID)
	case channelThread:
		return ev.RepoID == s.repoID && eventReferencesThread(ev, s.entityID)
	default:
		// TODO(phase-sync / phase-rules): wire channelRuleActions to
		// core.EventRuleTriggered, channelJob to a future JobUpdated event, and
		// channelSync to core.EventSyncCompleted / core.EventConflictDetected
		// once those events are published.
		return false
	}
}

// matchEventType applies the optional event_type filter. A nil or empty filter
// passes everything.
func (f *subFilter) matchEventType(ev *core.Event) bool {
	if f == nil || len(f.EventType) == 0 {
		return true
	}
	for _, t := range f.EventType {
		if core.EventType(t) == ev.Type {
			return true
		}
	}
	return false
}

// matchNodeType applies the optional node_type filter. Create, update, and patch
// events carry the full node, so they are filtered by its type; events without
// an embedded node (e.g. delete) cannot be evaluated and are not excluded.
func (f *subFilter) matchNodeType(ev *core.Event) bool {
	if f == nil || len(f.NodeType) == 0 {
		return true
	}
	node := nodeFromPayload(ev)
	if node == nil {
		return true
	}
	for _, t := range f.NodeType {
		if t == node.Type {
			return true
		}
	}
	return false
}

// nodeFromPayload extracts the embedded node from a node event payload. Create
// events carry it as Node; NodeUpdated and NodePatched both carry it as
// UpdatedNode (the repo module publishes both with EventNodeUpdatePayload).
// Delete events carry no node, so it returns nil there and the node_type filter
// lets them through.
func nodeFromPayload(ev *core.Event) *core.Node {
	switch ev.Type {
	case core.EventNodeCreated:
		var p core.EventNodeCreatePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return nil
		}
		return p.Node
	case core.EventNodeUpdated, core.EventNodePatched:
		var p core.EventNodeUpdatePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return nil
		}
		return p.UpdatedNode
	default:
		return nil
	}
}

// eventReferencesNode reports whether the event's payload references nodeID.
// It covers node-direct events and thread-node membership events.
func eventReferencesNode(ev *core.Event, nodeID uuid.UUID) bool {
	switch ev.Type {
	case core.EventNodeCreated:
		var p core.EventNodeCreatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.Node != nil && p.Node.ID == nodeID
	case core.EventNodeUpdated, core.EventNodePatched:
		// Both are published with EventNodeUpdatePayload by the repo module.
		var p core.EventNodeUpdatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	case core.EventNodeDeleted:
		var p core.EventNodeDeletePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	case core.EventThreadNodeAdded, core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.NodeID == nodeID
	default:
		return false
	}
}

// eventReferencesThread reports whether the event's payload references threadID.
func eventReferencesThread(ev *core.Event, threadID uuid.UUID) bool {
	switch ev.Type {
	case core.EventThreadCreated:
		var p core.EventThreadCreatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.Thread != nil && p.Thread.ID == threadID
	case core.EventThreadUpdated:
		var p core.EventThreadUpdatePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	case core.EventThreadDeleted:
		var p core.EventThreadDeletePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	case core.EventThreadNodeAdded, core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		return json.Unmarshal(ev.Payload, &p) == nil && p.ThreadID == threadID
	default:
		return false
	}
}
