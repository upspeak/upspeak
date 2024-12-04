package core

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/rs/xid"
)

type Repository struct {
	ID         xid.ID     // Unique identifier for the Repository
	Name       string     // Name of the Repository
	archive    Archive    // Backend storage for Nodes, Edges, etc.
	natsConn   *nats.Conn // Connection to NATS
	inSubject  string     // Subject for incoming operations
	outSubject string     // Subject for outgoing events
}

// NewRepository initializes a Repository with dedicated input/output subjects.
func NewRepository(id xid.ID, name string, archive Archive, natsConn *nats.Conn) *Repository {
	return &Repository{
		ID:         id,
		Name:       name,
		archive:    archive,
		natsConn:   natsConn,
		inSubject:  fmt.Sprintf("repos.%s.in", id.String()),
		outSubject: fmt.Sprintf("repos.%s.out", id.String()),
	}
}

// SubscribeToInputEvents sets up a NATS subscription for input events.
func (r *Repository) SubscribeToInputEvents() (*nats.Subscription, error) {
	return r.natsConn.Subscribe(r.inSubject, func(msg *nats.Msg) {
		var inputEvent Event
		if err := json.Unmarshal(msg.Data, &inputEvent); err != nil {
			fmt.Printf("Invalid input event received: %v\n", err)
			return
		}
		r.handleInputEvent(inputEvent)
	})
}

// handleInputEvent processes an incoming InputEvent and generates standard Events.
func (r *Repository) handleInputEvent(inputEvent Event) error {
	switch inputEvent.Type {
	case EventCreateNode:
		var payload EventNodeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventNodeCreatePayload"}
		}
		if err := r.archive.SaveNode(payload.Node); err != nil {
			return &ErrorSave{msg: "node"}
		}
		event, err := NewEvent(EventNodeCreated, EventNodeCreatePayload{Node: payload.Node})
		if err != nil {
			return &ErrorEventCreation{msg: "EventNodeCreated"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventNodeCreated"}
		}
	case EventUpdateNode:
		var payload EventNodeUpdatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventNodeUpdatePayload"}
		}
		if err := r.archive.SaveNode(payload.UpdatedNode); err != nil {
			return &ErrorSave{msg: "updated node"}
		}
		event, err := NewEvent(EventNodeUpdated, EventNodeUpdatePayload{NodeId: payload.NodeId, UpdatedNode: payload.UpdatedNode})
		if err != nil {
			return &ErrorEventCreation{msg: "EventNodeUpdated"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventNodeUpdated"}
		}
	case EventDeleteNode:
		var payload EventNodeDeletePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventNodeDeletePayload"}
		}
		if err := r.archive.DeleteNode(payload.NodeId); err != nil {
			return &ErrorDelete{msg: "node"}
		}
		event, err := NewEvent(EventNodeDeleted, EventNodeDeletePayload{NodeId: payload.NodeId})
		if err != nil {
			return &ErrorEventCreation{msg: "EventNodeDeleted"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventNodeDeleted"}
		}
	case EventCreateEdge:
		var payload EventEdgeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventEdgeCreatePayload"}
		}
		if err := r.archive.SaveEdge(payload.Edge); err != nil {
			return &ErrorSave{msg: "edge"}
		}
		event, err := NewEvent(EventEdgeCreated, EventEdgeCreatePayload{Edge: payload.Edge})
		if err != nil {
			return &ErrorEventCreation{msg: "EventEdgeCreated"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventEdgeCreated"}
		}
	case EventUpdateEdge:
		var payload EventEdgeCreatePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventEdgeCreatePayload"}
		}
		if err := r.archive.SaveEdge(payload.Edge); err != nil {
			return &ErrorSave{msg: "edge"}
		}

		event, err := NewEvent(EventEdgeUpdated, EventEdgeUpdatePayload{EdgeId: payload.Edge.ID, UpdatedEdge: payload.Edge})
		if err != nil {
			return &ErrorEventCreation{msg: "EventEdgeUpdated"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventEdgeUpdated"}
		}
	case EventDeleteEdge:
		var payload EventEdgeDeletePayload
		if err := json.Unmarshal(inputEvent.Payload, &payload); err != nil {
			return &ErrorUnmarshal{msg: "EventEdgeDeletePayload"}
		}
		if err := r.archive.DeleteEdge(payload.EdgeId); err != nil {
			return &ErrorDelete{msg: "edge"}
		}
		event, err := NewEvent(EventEdgeDeleted, EventEdgeDeletePayload{EdgeId: payload.EdgeId})
		if err != nil {
			return &ErrorEventCreation{msg: "EventEdgeDeleted"}
		}
		if err := r.publishEvent(event); err != nil {
			return &ErrorPublish{msg: "EventEdgeDeleted"}
		}
	default:
		return fmt.Errorf("unhandled input event type: %s", inputEvent.Type)
	}
	return nil
}

// publishEvent sends a standard Event to the outgoing NATS subject.
func (r *Repository) publishEvent(event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	return r.natsConn.Publish(r.outSubject, data)
}
