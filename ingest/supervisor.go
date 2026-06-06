package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

const (
	supFetchBatchSize = 10
	supFetchTimeout   = 5 * time.Second
)

// ackDecision is the disposition of a consumed Sink event message.
type ackDecision int

const (
	ackOK    ackDecision = iota // processed (or intentionally skipped): acknowledge
	ackRetry                    // transient failure or deferred refs: redeliver
	ackTerm                     // permanently undeliverable (poison): drop
)

// Supervisor is the ingest side of repo→repo: a durable JetStream pull consumer
// on SINK_EVENTS that maps each curated Sink event into the ingest pipeline for
// every repository whose repo-Source subscribes to that Sink. It implements
// app.Runner and is started from main after archive wiring.
//
// Ack discipline (mirrors rules.Engine):
//   - Unparseable subject or envelope → Term (poison; redelivery cannot help).
//   - Archive/transient error before any side effects → Nak (retry).
//   - res.Deferred > 0 or membership not yet applied → Nak (retry so a
//     later-arriving node can resolve relational references).
//   - Success, including filter-rejected (Skipped) → Ack.
type Supervisor struct {
	archive  core.Archive
	pipeline *Pipeline
	consumer app.Consumer
}

// NewSupervisor constructs an ingest Supervisor. consumer may be nil when
// calling handleSinkEvent directly in tests, but Run requires a non-nil
// consumer — a nil one dereferences to a panic at the first Fetch.
func NewSupervisor(archive core.Archive, pub app.Publisher, consumer app.Consumer) *Supervisor {
	return &Supervisor{
		archive:  archive,
		pipeline: NewPipeline(archive, pub),
		consumer: consumer,
	}
}

// Run starts the supervisor's consume loop. It blocks until ctx is cancelled.
func (s *Supervisor) Run(ctx context.Context) {
	slog.Info("Ingest supervisor started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("Ingest supervisor stopping")
			return
		default:
		}

		msgs, err := s.consumer.Fetch(supFetchBatchSize, supFetchTimeout)
		if err != nil {
			if errors.Is(err, app.ErrFetchTimeout) {
				continue
			}
			slog.Error("Ingest supervisor fetch failed", "error", err)
			continue
		}

		for _, msg := range msgs {
			s.processMessage(msg)
		}
	}
}

// processMessage dispatches a single Sink event message and acknowledges it
// according to the outcome.
func (s *Supervisor) processMessage(msg *app.Msg) {
	sinkID, err := sinkIDFromSubject(msg.Subject)
	if err != nil {
		slog.Error("Ingest supervisor: bad sink subject", "subject", msg.Subject, "error", err)
		_ = msg.Term()
		return
	}
	switch s.handleSinkEvent(sinkID, msg.Data) {
	case ackRetry:
		_ = msg.Nak()
	case ackTerm:
		_ = msg.Term()
	default:
		_ = msg.Ack()
	}
}

// handleSinkEvent fans one curated Sink event out to every repo-Source that
// subscribes to that Sink. It returns ackOK on success (including filter
// rejection), ackRetry on transient errors or deferred relational references,
// and ackTerm on a permanently undeliverable envelope.
func (s *Supervisor) handleSinkEvent(sinkID uuid.UUID, data []byte) ackDecision {
	var evt core.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		return ackTerm // malformed envelope; redelivery cannot help
	}

	// Hops drop: discard events that have already cascaded too far, preventing
	// unbounded repo→repo reaction chains.
	if evt.Hops >= core.MaxEventHops {
		slog.Warn("Ingest supervisor: dropping event exceeding max hops",
			"event", evt.Type, "hops", evt.Hops)
		return ackOK
	}

	// Decode the event into an ingestible batch once, before any archive read or
	// side effect. A decode failure is poison (redelivery cannot help) → Term,
	// mirroring rules.Engine. The decoded batch is reused across all subscribers.
	batch, mem, err := batchFromEvent(&evt)
	if err != nil {
		slog.Error("Ingest supervisor: malformed event payload",
			"event", evt.Type, "error", err)
		return ackTerm
	}
	if batch == nil && mem == nil {
		return ackOK // event type not handled; intentional no-op
	}

	sources, err := s.archive.ListRepoSourcesForSink(sinkID)
	if err != nil {
		slog.Error("Ingest supervisor: list subscribers failed",
			"sink", sinkID, "error", err)
		return ackRetry // no side effects yet; redeliver
	}

	// Sources span multiple repositories; cache each repo's owner so a Sink with
	// many subscribers across a few repos does not re-read the same repository.
	ownerCache := make(map[uuid.UUID]uuid.UUID)
	deferred := false
	for i := range sources {
		ownerID, ok := ownerCache[sources[i].RepoID]
		if !ok {
			resolved, err := s.repoOwner(sources[i].RepoID)
			if err != nil {
				slog.Error("Ingest supervisor: load repo owner failed",
					"repo", sources[i].RepoID, "error", err)
				return ackRetry // no side effects for this source yet; redeliver
			}
			ownerID = resolved
			ownerCache[sources[i].RepoID] = ownerID
		}
		d, err := s.ingestForSource(&sources[i], &evt, ownerID, batch, mem)
		if err != nil {
			slog.Error("Ingest supervisor: ingest failed",
				"source", sources[i].ID, "error", err)
			return ackRetry
		}
		if d {
			deferred = true
		}
	}

	if deferred {
		return ackRetry // unresolved relational refs: retry later
	}
	return ackOK
}

// ingestForSource applies one decoded Sink event to a single subscribing
// repo-Source, attributing created entities to ownerID. Exactly one of batch/mem
// is non-nil (a membership change is handled separately from batch ingestion). It
// returns (true, nil) when the work should be retried (unresolved relational
// references), (false, nil) on success (including filter rejection), and
// (false, err) on a hard error.
func (s *Supervisor) ingestForSource(src *core.Source, evt *core.Event, ownerID uuid.UUID, batch *core.IngestBatch, mem *membership) (deferred bool, err error) {
	ctx := IngestContext{
		RepoID:      src.RepoID,
		Source:      src,
		CreatedBy:   ownerID,
		InboundHops: evt.Hops, // propagate so emitted events carry Hops+1
	}

	if mem != nil {
		applied, err := s.pipeline.ApplyThreadMembership(ctx, mem.thread, mem.node, mem.add)
		if err != nil {
			return false, err
		}
		if !applied {
			return true, nil // unresolved thread or node: defer
		}
		return false, nil
	}

	res, err := s.pipeline.Ingest(ctx, batch)
	if err != nil {
		return false, err
	}
	// res.Skipped is terminal (filter rejection) — do NOT retry.
	// res.Deferred means relational references are unresolved: ask for retry.
	return res.Deferred > 0, nil
}

// membership carries the thread and node external IDs for a thread membership
// change event, plus whether the node is being added (true) or removed (false).
type membership struct {
	thread string
	node   string
	add    bool
}

// batchFromEvent converts a domain event into an IngestBatch (or a membership
// change) suitable for the pipeline. It returns (nil, nil, nil) for event types
// that the ingest supervisor intentionally ignores (e.g. administrative events).
func batchFromEvent(evt *core.Event) (*core.IngestBatch, *membership, error) {
	switch evt.Type {
	case core.EventNodeCreated:
		var p core.EventNodeCreatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent NodeCreated: %w", err)
		}
		if p.Node == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Items: []core.IngestItem{
			nodeItemFromNode(p.Node.ID.String(), p.Node),
		}}, nil, nil

	case core.EventNodeUpdated:
		var p core.EventNodeUpdatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent NodeUpdated: %w", err)
		}
		if p.UpdatedNode == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Items: []core.IngestItem{
			nodeItemFromNode(p.UpdatedNode.ID.String(), p.UpdatedNode),
		}}, nil, nil

	case core.EventEdgeCreated:
		var p core.EventEdgeCreatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent EdgeCreated: %w", err)
		}
		if p.Edge == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Edges: []core.IngestEdge{
			ingestEdgeFromEdge(p.Edge.ID.String(), p.Edge),
		}}, nil, nil

	case core.EventEdgeUpdated:
		var p core.EventEdgeUpdatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent EdgeUpdated: %w", err)
		}
		if p.UpdatedEdge == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Edges: []core.IngestEdge{
			ingestEdgeFromEdge(p.UpdatedEdge.ID.String(), p.UpdatedEdge),
		}}, nil, nil

	case core.EventThreadCreated:
		var p core.EventThreadCreatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent ThreadCreated: %w", err)
		}
		if p.Thread == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Threads: []core.IngestThread{
			ingestThreadFromThread(p.Thread),
		}}, nil, nil

	case core.EventThreadUpdated:
		var p core.EventThreadUpdatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent ThreadUpdated: %w", err)
		}
		if p.UpdatedThread == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Threads: []core.IngestThread{
			ingestThreadFromThread(p.UpdatedThread),
		}}, nil, nil

	case core.EventAnnotationCreated:
		var p core.EventAnnotationCreatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent AnnotationCreated: %w", err)
		}
		if p.Annotation == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Annotations: []core.IngestAnnotation{
			ingestAnnotationFromAnnotation(p.Annotation),
		}}, nil, nil

	case core.EventAnnotationUpdated:
		var p core.EventAnnotationUpdatePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent AnnotationUpdated: %w", err)
		}
		if p.UpdatedAnnotation == nil {
			return nil, nil, nil
		}
		return &core.IngestBatch{Annotations: []core.IngestAnnotation{
			ingestAnnotationFromAnnotation(p.UpdatedAnnotation),
		}}, nil, nil

	case core.EventThreadNodeAdded:
		var p core.EventThreadNodePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent ThreadNodeAdded: %w", err)
		}
		if p.ThreadID == uuid.Nil || p.NodeID == uuid.Nil {
			return nil, nil, nil // malformed membership payload; intentional no-op
		}
		return nil, &membership{thread: p.ThreadID.String(), node: p.NodeID.String(), add: true}, nil

	case core.EventThreadNodeRemoved:
		var p core.EventThreadNodePayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return nil, nil, fmt.Errorf("batchFromEvent ThreadNodeRemoved: %w", err)
		}
		if p.ThreadID == uuid.Nil || p.NodeID == uuid.Nil {
			return nil, nil, nil // malformed membership payload; intentional no-op
		}
		return nil, &membership{thread: p.ThreadID.String(), node: p.NodeID.String(), add: false}, nil

	default:
		return nil, nil, nil // unhandled event type; intentional no-op
	}
}

// nodeItemFromNode projects a Node from an upstream event into an IngestItem,
// stripping identity, provenance, and version fields so the destination repo
// assigns its own. Only content-bearing fields are carried over.
func nodeItemFromNode(externalID string, n *core.Node) core.IngestItem {
	return core.IngestItem{
		ExternalID: externalID,
		Node: &core.Node{
			Type:        n.Type,
			Subject:     n.Subject,
			ContentType: n.ContentType,
			Body:        n.Body,
			Metadata:    n.Metadata,
		},
	}
}

// ingestEdgeFromEdge projects an Edge from an upstream event into an IngestEdge,
// using the upstream Edge's Source and Target UUIDs as external IDs so the
// destination pipeline can resolve them by provenance.
func ingestEdgeFromEdge(externalID string, e *core.Edge) core.IngestEdge {
	return core.IngestEdge{
		ExternalID:       externalID,
		SourceExternalID: e.Source.String(),
		TargetExternalID: e.Target.String(),
		Type:             e.Type,
		Label:            e.Label,
		Weight:           e.Weight,
	}
}

// ingestThreadFromThread projects a Thread from an upstream event into an
// IngestThread. The root node's Subject becomes the IngestThread's Subject.
func ingestThreadFromThread(t *core.Thread) core.IngestThread {
	return core.IngestThread{
		ExternalThreadID: t.ID.String(),
		Subject:          t.Node.Subject,
		Metadata:         t.Metadata,
	}
}

// ingestAnnotationFromAnnotation projects an Annotation from an upstream event
// into an IngestAnnotation. The annotation's Edge.Target is the annotated node's
// UUID, which becomes the TargetExternalID for provenance resolution.
func ingestAnnotationFromAnnotation(a *core.Annotation) core.IngestAnnotation {
	return core.IngestAnnotation{
		ExternalID:       a.ID.String(),
		TargetExternalID: a.Edge.Target.String(),
		Motivation:       a.Motivation,
		Body:             a.Node.Body,
	}
}

// repoOwner loads the repository and returns its OwnerID.
func (s *Supervisor) repoOwner(repoID uuid.UUID) (uuid.UUID, error) {
	repo, err := s.archive.GetRepository(repoID)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("get repository: %w", err)
	}
	return repo.OwnerID, nil
}

// sinkIDFromSubject parses the Sink UUID from a subject of the form
// sink.{sink_id}.events.{EventType}. It returns an error for malformed subjects.
func sinkIDFromSubject(subject string) (uuid.UUID, error) {
	parts := strings.Split(subject, ".")
	// Expected: ["sink", "<uuid>", "events", "<EventType>"]
	if len(parts) < 4 || parts[0] != "sink" || parts[2] != "events" {
		return uuid.UUID{}, fmt.Errorf("sinkIDFromSubject: unexpected subject %q", subject)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("sinkIDFromSubject: parse sink id %q: %w", parts[1], err)
	}
	return id, nil
}
