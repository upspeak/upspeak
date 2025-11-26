package core

import (
	"encoding/json"

	"github.com/rs/xid"
)

// Repository is a domain aggregate that manages Nodes and Edges within a context.
// It uses an Archive for persistent storage and processes events to maintain state.
type Repository struct {
	ID      xid.ID  // Unique identifier for the Repository
	Name    string  // Name of the Repository
	archive Archive // Backend storage for Nodes, Edges, etc.
}

// NewRepository initialises a Repository with the given archive backend.
func NewRepository(id xid.ID, name string, archive Archive) *Repository {
	return &Repository{
		ID:      id,
		Name:    name,
		archive: archive,
	}
}

// HandleInputEvent processes an incoming Event and returns the corresponding output Event.
// Returns the output event that should be published, or an error if processing fails.
func (r *Repository) HandleInputEvent(inputEvent Event) (*Event, error) {
	switch inputEvent.Type {
	case EventCreateNode:
		var payload EventNodeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventNodeCreatePayload"}
		}
		if err := r.archive.SaveNode(payload.Node); err != nil {
			return nil, &ErrorSave{msg: "node"}
		}
		event, err := NewEvent(EventNodeCreated, EventNodeCreatePayload{Node: payload.Node})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventNodeCreated"}
		}
		return event, nil
	case EventUpdateNode:
		var payload EventNodeUpdatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventNodeUpdatePayload"}
		}
		if err := r.archive.SaveNode(payload.UpdatedNode); err != nil {
			return nil, &ErrorSave{msg: "updated node"}
		}
		event, err := NewEvent(EventNodeUpdated, EventNodeUpdatePayload{NodeId: payload.NodeId, UpdatedNode: payload.UpdatedNode})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventNodeUpdated"}
		}
		return event, nil
	case EventDeleteNode:
		var payload EventNodeDeletePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventNodeDeletePayload"}
		}
		if err := r.archive.DeleteNode(payload.NodeId); err != nil {
			return nil, &ErrorDelete{msg: "node"}
		}
		event, err := NewEvent(EventNodeDeleted, EventNodeDeletePayload{NodeId: payload.NodeId})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventNodeDeleted"}
		}
		return event, nil
	case EventCreateEdge:
		var payload EventEdgeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventEdgeCreatePayload"}
		}
		if err := r.archive.SaveEdge(payload.Edge); err != nil {
			return nil, &ErrorSave{msg: "edge"}
		}
		event, err := NewEvent(EventEdgeCreated, EventEdgeCreatePayload{Edge: payload.Edge})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventEdgeCreated"}
		}
		return event, nil
	case EventUpdateEdge:
		var payload EventEdgeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventEdgeCreatePayload"}
		}
		if err := r.archive.SaveEdge(payload.Edge); err != nil {
			return nil, &ErrorSave{msg: "edge"}
		}

		event, err := NewEvent(EventEdgeUpdated, EventEdgeUpdatePayload{EdgeId: payload.Edge.ID, UpdatedEdge: payload.Edge})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventEdgeUpdated"}
		}
		return event, nil
	case EventDeleteEdge:
		var payload EventEdgeDeletePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventEdgeDeletePayload"}
		}
		if err := r.archive.DeleteEdge(payload.EdgeId); err != nil {
			return nil, &ErrorDelete{msg: "edge"}
		}
		event, err := NewEvent(EventEdgeDeleted, EventEdgeDeletePayload{EdgeId: payload.EdgeId})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventEdgeDeleted"}
		}
		return event, nil
	case EventCreateThread:
		var payload EventThreadCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventThreadCreatePayload"}
		}
		if err := r.archive.SaveThread(payload.Thread); err != nil {
			return nil, &ErrorSave{msg: "thread"}
		}
		event, err := NewEvent(EventThreadCreated, EventThreadCreatePayload{Thread: payload.Thread})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventThreadCreated"}
		}
		return event, nil
	case EventUpdateThread:
		var payload EventThreadUpdatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventThreadUpdatePayload"}
		}
		if err := r.archive.SaveThread(payload.UpdatedThread); err != nil {
			return nil, &ErrorSave{msg: "updated thread"}
		}
		event, err := NewEvent(EventThreadUpdated, EventThreadUpdatePayload{ThreadId: payload.ThreadId, UpdatedThread: payload.UpdatedThread})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventThreadUpdated"}
		}
		return event, nil
	case EventDeleteThread:
		var payload EventThreadDeletePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return nil, &ErrorUnmarshal{msg: "EventThreadDeletePayload"}
		}
		if err := r.archive.DeleteThread(payload.ThreadId); err != nil {
			return nil, &ErrorDelete{msg: "thread"}
		}
		event, err := NewEvent(EventThreadDeleted, EventThreadDeletePayload{ThreadId: payload.ThreadId})
		if err != nil {
			return nil, &ErrorEventCreation{msg: "EventThreadDeleted"}
		}
		return event, nil
	default:
		return nil, &ErrorUnknownEventType{eventType: string(inputEvent.Type)}
	}
}

// GetNode retrieves a Node by its ID from the archive.
func (r *Repository) GetNode(id xid.ID) (*Node, error) {
	return r.archive.GetNode(id)
}

// GetEdge retrieves an Edge by its ID from the archive.
func (r *Repository) GetEdge(id xid.ID) (*Edge, error) {
	return r.archive.GetEdge(id)
}

// GetThread retrieves a Thread by its root node ID from the archive.
func (r *Repository) GetThread(nodeID xid.ID) (*Thread, error) {
	return r.archive.GetThread(nodeID)
}

// GetAnnotation retrieves an Annotation by its node ID from the archive.
func (r *Repository) GetAnnotation(nodeID xid.ID) (*Annotation, error) {
	return r.archive.GetAnnotation(nodeID)
}
