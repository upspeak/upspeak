package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/ingest"
)

// supFetchBatchSize and supFetchTimeout control the publish supervisor's durable
// consumer fetch loop. supSinkPageLimit bounds the per-repo Sink listing.
const (
	supFetchBatchSize = 10
	supFetchTimeout   = 5 * time.Second
	supSinkPageLimit  = 1000
)

// ackDecision is the disposition of a consumed domain event message.
type ackDecision int

const (
	ackOK    ackDecision = iota // processed (or intentionally skipped): acknowledge
	ackRetry                    // transient failure before side effects: redeliver
	ackTerm                     // permanently undeliverable (poison): drop
)

// PublishSupervisor is the publish side of repo→repo: a durable JetStream pull
// consumer on REPO_EVENTS that enforces each repository's Sink publication
// control. For each domain event it finds all active repo-type Sinks in the
// producing repository, evaluates the event's entity against each Sink's filter
// chain, and republishes the curated subset to SINK_EVENTS. The ingest
// supervisor (ingest.Supervisor) consumes the resulting SINK_EVENTS.
//
// Ack discipline (mirrors rules.Engine):
//   - Malformed envelope → Term (poison; redelivery cannot help).
//   - Archive read failure before any publish → Nak (retry).
//   - Success, including "filtered out, nothing to publish" → Ack.
//
// Publishing itself is fire-and-forget: a publish error is logged and the
// message is still acknowledged, matching the pattern in rules/engine.go and
// ingest/pipeline.go.
type PublishSupervisor struct {
	archive  core.Archive
	pub      app.Publisher
	consumer app.Consumer
}

// NewPublishSupervisor constructs a PublishSupervisor with its dependencies.
// consumer may be nil when calling dispatch directly in tests; Run requires a
// non-nil consumer.
func NewPublishSupervisor(archive core.Archive, pub app.Publisher, consumer app.Consumer) *PublishSupervisor {
	return &PublishSupervisor{archive: archive, pub: pub, consumer: consumer}
}

// Run starts the supervisor's consume loop. It blocks until ctx is cancelled.
func (s *PublishSupervisor) Run(ctx context.Context) {
	slog.Info("Publish supervisor started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Publish supervisor stopping")
			return
		default:
		}

		msgs, err := s.consumer.Fetch(supFetchBatchSize, supFetchTimeout)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue // No messages available; try again.
			}
			slog.Error("Publish supervisor fetch failed", "error", err)
			continue
		}

		for _, msg := range msgs {
			s.processMessage(msg)
		}
	}
}

// processMessage dispatches a single event message and acknowledges it
// according to the outcome: redeliver on transient failure (before any side
// effects), terminate on a poison message, otherwise acknowledge.
func (s *PublishSupervisor) processMessage(msg *app.Msg) {
	switch s.dispatch(msg.Data) {
	case ackRetry:
		_ = msg.Nak()
	case ackTerm:
		_ = msg.Term()
	default:
		_ = msg.Ack()
	}
}

// dispatch evaluates one domain event against the producing repo's Sinks and
// republishes the curated subset to SINK_EVENTS. It returns the acknowledgement
// disposition for the message.
func (s *PublishSupervisor) dispatch(data []byte) ackDecision {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		slog.Error("Publish supervisor: failed to unmarshal event", "error", err)
		return ackTerm // Malformed envelope; redelivery cannot help.
	}

	// Hops bound: do not re-publish events that have already cascaded too far,
	// bounding unbounded repo→repo reaction chains.
	if evt.Hops >= core.MaxEventHops {
		slog.Warn("Publish supervisor: dropping event exceeding max hops",
			"event", evt.Type, "hops", evt.Hops)
		return ackOK
	}

	if !isPublishableEntityEvent(evt.Type) {
		return ackOK // Lifecycle/meta/delete events never leave the repo.
	}

	nodes, normType, payload, ok, err := s.resolvePublish(&evt)
	if err != nil {
		if errors.Is(err, errBadPayload) {
			slog.Error("Publish supervisor: malformed event payload",
				"event", evt.Type, "error", err)
			return ackTerm // Poison payload; redelivery cannot help.
		}
		slog.Error("Publish supervisor: failed to resolve event entity",
			"event", evt.Type, "repo", evt.RepoID, "error", err)
		return ackRetry // Archive read failed before any publish.
	}
	if !ok {
		return ackOK // Unresolvable target (e.g. deleted) — nothing to publish.
	}

	sinks, _, err := s.archive.ListSinks(evt.RepoID, core.SinkListOptions{
		ListOptions: core.ListOptions{Limit: supSinkPageLimit, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		slog.Error("Publish supervisor: failed to list sinks",
			"repo", evt.RepoID, "error", err)
		return ackRetry
	}

	// Evaluate every candidate Sink before publishing to any of them. This keeps
	// the ackRetry path side-effect-free: a filter-evaluation error must not leave
	// some Sinks already republished, which a redelivery would then duplicate
	// (SINK_EVENTS has no dedup window, and each republish mints a fresh event ID).
	var matched []uuid.UUID
	for i := range sinks {
		sk := &sinks[i]
		if sk.Connector != core.ConnectorRepo || sk.Status != core.StatusActive {
			continue
		}
		pass, err := allNodesPass(s.archive, sk, nodes)
		if err != nil {
			slog.Error("Publish supervisor: filter evaluation failed",
				"sink", sk.ID, "error", err)
			return ackRetry // no publishes yet — safe to redeliver
		}
		if pass {
			matched = append(matched, sk.ID)
		}
	}

	for _, sinkID := range matched {
		s.republish(sinkID, normType, payload, &evt)
	}
	return ackOK
}

// errBadPayload marks an event whose payload cannot be decoded into the expected
// shape. Such an event is poison — redelivery cannot help — so dispatch
// terminates it rather than retrying, mirroring rules.Engine.
var errBadPayload = errors.New("malformed event payload")

// decodePayload unmarshals an event payload, tagging a decode failure as poison
// (errBadPayload) so the caller can distinguish it from a transient archive error.
func decodePayload(raw json.RawMessage, dst any, t core.EventType) error {
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("resolvePublish %s: %w: %v", t, errBadPayload, err)
	}
	return nil
}

// resolvePublish resolves the node(s) to evaluate the Sink filter chain against,
// the normalised event type to republish, and the payload to republish. It
// returns ok=false when the entity is absent (e.g. deleted) and there is nothing
// to publish. An error return signals a real archive failure and causes a Nak.
//
// The returned nodes slice is used by allNodesPass with AND semantics: every node
// must pass. For edges both endpoint nodes are returned. For annotations the
// target node is returned. For threads the root node is returned (see below). For
// membership events the single node is returned.
//
// Thread root-node rationale: the ingest side consumes only a thread's
// ExternalThreadID/Subject/Metadata and ignores Thread.Edges (membership);
// members propagate as separate ThreadNodeAdded events filtered individually.
// Filtering on the root node is therefore sufficient — per-member pruning of the
// republished payload would have no effect on ingestion. This is a deliberate
// divergence from any "≥1 member passes" wording in earlier design notes, which
// predated the pipeline detail.
func (s *PublishSupervisor) resolvePublish(evt *core.Event) (nodes []*core.Node, normType core.EventType, payload any, ok bool, err error) {
	switch evt.Type {
	case core.EventNodeCreated:
		var p core.EventNodeCreatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.Node == nil {
			return nil, "", nil, false, nil
		}
		return []*core.Node{p.Node}, core.EventNodeCreated, p, true, nil

	case core.EventNodeUpdated:
		var p core.EventNodeUpdatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.UpdatedNode == nil {
			return nil, "", nil, false, nil
		}
		return []*core.Node{p.UpdatedNode}, core.EventNodeUpdated, p, true, nil

	case core.EventNodePatched:
		// The patch payload does not carry the full node; load it from the archive
		// and normalise to an EventNodeUpdated shape so Sink filters can evaluate
		// the current node state.
		var p core.EventNodePatchPayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		n, err := s.archive.GetNode(p.NodeID)
		if err != nil {
			if errors.As(err, new(*core.ErrorNotFound)) {
				return nil, "", nil, false, nil // Deleted between patch and publish.
			}
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		normPayload := core.EventNodeUpdatePayload{NodeID: n.ID, UpdatedNode: n}
		return []*core.Node{n}, core.EventNodeUpdated, normPayload, true, nil

	case core.EventEdgeCreated:
		var p core.EventEdgeCreatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.Edge == nil {
			return nil, "", nil, false, nil
		}
		srcNode, tgtNode, ok, err := s.loadEdgeEndpoints(p.Edge.Source, p.Edge.Target)
		if err != nil {
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		if !ok {
			return nil, "", nil, false, nil
		}
		return []*core.Node{srcNode, tgtNode}, core.EventEdgeCreated, p, true, nil

	case core.EventEdgeUpdated:
		var p core.EventEdgeUpdatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.UpdatedEdge == nil {
			return nil, "", nil, false, nil
		}
		srcNode, tgtNode, ok, err := s.loadEdgeEndpoints(p.UpdatedEdge.Source, p.UpdatedEdge.Target)
		if err != nil {
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		if !ok {
			return nil, "", nil, false, nil
		}
		return []*core.Node{srcNode, tgtNode}, core.EventEdgeUpdated, p, true, nil

	case core.EventAnnotationCreated:
		var p core.EventAnnotationCreatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.Annotation == nil {
			return nil, "", nil, false, nil
		}
		// Filter on the annotated target node (the Edge.Target of the "annotates"
		// edge), not the annotation's own content node: the latter always has
		// type "annotation", which would make type-based Sink filters degenerate.
		n, err := s.archive.GetNode(p.Annotation.Edge.Target)
		if err != nil {
			if errors.As(err, new(*core.ErrorNotFound)) {
				return nil, "", nil, false, nil
			}
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		return []*core.Node{n}, core.EventAnnotationCreated, p, true, nil

	case core.EventAnnotationUpdated:
		var p core.EventAnnotationUpdatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.UpdatedAnnotation == nil {
			return nil, "", nil, false, nil
		}
		n, err := s.archive.GetNode(p.UpdatedAnnotation.Edge.Target)
		if err != nil {
			if errors.As(err, new(*core.ErrorNotFound)) {
				return nil, "", nil, false, nil
			}
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		return []*core.Node{n}, core.EventAnnotationUpdated, p, true, nil

	case core.EventThreadCreated:
		var p core.EventThreadCreatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.Thread == nil {
			return nil, "", nil, false, nil
		}
		rootNode := &p.Thread.Node
		return []*core.Node{rootNode}, core.EventThreadCreated, p, true, nil

	case core.EventThreadUpdated:
		var p core.EventThreadUpdatePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.UpdatedThread == nil {
			return nil, "", nil, false, nil
		}
		rootNode := &p.UpdatedThread.Node
		return []*core.Node{rootNode}, core.EventThreadUpdated, p, true, nil

	case core.EventThreadNodeAdded:
		var p core.EventThreadNodePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.NodeID == uuid.Nil {
			return nil, "", nil, false, nil
		}
		n, err := s.archive.GetNode(p.NodeID)
		if err != nil {
			if errors.As(err, new(*core.ErrorNotFound)) {
				return nil, "", nil, false, nil
			}
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		return []*core.Node{n}, core.EventThreadNodeAdded, p, true, nil

	case core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		if err := decodePayload(evt.Payload, &p, evt.Type); err != nil {
			return nil, "", nil, false, err
		}
		if p.NodeID == uuid.Nil {
			return nil, "", nil, false, nil
		}
		n, err := s.archive.GetNode(p.NodeID)
		if err != nil {
			if errors.As(err, new(*core.ErrorNotFound)) {
				return nil, "", nil, false, nil
			}
			return nil, "", nil, false, fmt.Errorf("resolvePublish %s: %w", evt.Type, err)
		}
		return []*core.Node{n}, core.EventThreadNodeRemoved, p, true, nil

	default:
		return nil, "", nil, false, nil // Gated by isPublishableEntityEvent; should not reach here.
	}
}

// loadEdgeEndpoints loads both endpoint nodes of an edge. It returns ok=false
// (and no error) when either endpoint is absent (NotFound). A real archive error
// propagates as err.
func (s *PublishSupervisor) loadEdgeEndpoints(srcID, tgtID uuid.UUID) (src *core.Node, tgt *core.Node, ok bool, err error) {
	src, err = s.archive.GetNode(srcID)
	if err != nil {
		if errors.As(err, new(*core.ErrorNotFound)) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	tgt, err = s.archive.GetNode(tgtID)
	if err != nil {
		if errors.As(err, new(*core.ErrorNotFound)) {
			return nil, nil, false, nil
		}
		return nil, nil, false, err
	}
	return src, tgt, true, nil
}

// republish constructs the outbound event with propagated hops and publishes it
// to SINK_EVENTS via the canonical SinkSubject. Fire-and-forget: marshal/publish
// errors are logged and do not cause a retry (by the time this is called the
// message has effectively been accepted). A nil publisher is a no-op.
func (s *PublishSupervisor) republish(sinkID uuid.UUID, eventType core.EventType, payload any, inbound *core.Event) {
	if s.pub == nil {
		return
	}
	out, err := core.NewEvent(eventType, inbound.RepoID, payload)
	if err != nil {
		slog.Error("Publish supervisor: failed to build outbound event",
			"type", eventType, "error", err)
		return
	}
	// Propagate hops so downstream consumers can bound reaction chains.
	// Do NOT use PublishEvent, which resets Hops to 0 — build the event and
	// marshal directly, matching the pattern in rules/engine.go and
	// ingest/pipeline.go.
	out.Hops = inbound.Hops + 1
	data, err := json.Marshal(out)
	if err != nil {
		slog.Error("Publish supervisor: failed to marshal outbound event",
			"type", eventType, "error", err)
		return
	}
	subject := core.SinkSubject(sinkID, eventType)
	if err := s.pub.Publish(subject, data); err != nil {
		slog.Error("Publish supervisor: failed to publish to sink",
			"sink", sinkID, "subject", subject, "error", err)
	}
}

// allNodesPass reports whether every node in the slice satisfies the Sink's
// filter chain. AND semantics: a single failing node returns false. An empty
// nodes slice is treated as a pass. This realises the edge two-endpoint AND rule
// cleanly: edges supply both src and tgt nodes; single-entity events supply one.
func allNodesPass(archive core.Archive, sk *core.Sink, nodes []*core.Node) (bool, error) {
	for _, n := range nodes {
		match, err := ingest.MatchesFilterChain(archive, sk.FilterIDs, sk.FilterChainMode, n)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

// isPublishableEntityEvent reports whether an event type carries a knowledge
// graph entity that Sinks are eligible to republish. Delete events and all
// administrative/operational events are excluded — they are scoped to the
// originating repository and must not cross the repo boundary.
func isPublishableEntityEvent(t core.EventType) bool {
	switch t {
	case core.EventNodeCreated, core.EventNodeUpdated, core.EventNodePatched,
		core.EventEdgeCreated, core.EventEdgeUpdated,
		core.EventThreadCreated, core.EventThreadUpdated,
		core.EventAnnotationCreated, core.EventAnnotationUpdated,
		core.EventThreadNodeAdded, core.EventThreadNodeRemoved:
		return true
	default:
		return false
	}
}
