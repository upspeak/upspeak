package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
	"github.com/upspeak/upspeak/filter"
)

// IngestContext carries the destination repo and ingestion provenance for a
// batch. Source is nil for ad-hoc ingestion (one-shot webhook): such nodes get
// no provenance, no dedup, and no source filters. When Source is non-nil, items
// are deduplicated by (Source.ID, ExternalID), filtered through the source's
// filter chain, and stamped with provenance.
type IngestContext struct {
	RepoID    uuid.UUID
	Source    *core.Source
	CreatedBy uuid.UUID
	// InboundHops is the Hops count of the event that triggered this ingest (0
	// for direct/ad-hoc ingestion). Events the pipeline emits carry InboundHops+1
	// so repo->repo reaction chains are bounded.
	InboundHops int
}

// IngestResult summarises what a batch produced.
type IngestResult struct {
	Created int
	Updated int
	Skipped int // filtered out by the source filter chain
	// Deferred counts relational references (edge endpoints, annotation targets,
	// thread members) whose target node was absent at ingest time. A non-zero
	// Deferred means the batch should be retried (Nak) so a later-arriving node
	// can resolve them.
	Deferred int
}

// Pipeline turns adapter-emitted IngestBatches into persisted graph entities,
// reusing the canonical write+event path (SaveBatchNodes/SaveNode +
// PublishEvent) so rules, realtime, and search light up for ingested data with
// no extra wiring. The pipeline processes Threads, Items, Edges, and Annotations
// with provenance-based resolution; Tombstones and author->User resolution land
// in later A2b tasks.
type Pipeline struct {
	archive core.Archive
	pub     app.Publisher
}

// NewPipeline creates a pipeline. pub may be nil (events are then skipped).
func NewPipeline(archive core.Archive, pub app.Publisher) *Pipeline {
	return &Pipeline{archive: archive, pub: pub}
}

// Ingest persists a batch's Threads, Items, Edges and Annotations into
// ctx.RepoID, resolving relational references via provenance. Items entering
// through a Source are deduplicated by provenance (re-collection updates the
// existing node) and filtered through the source's chain. On error mid-batch,
// the returned IngestResult reflects operations completed before the failure.
func (p *Pipeline) Ingest(ctx IngestContext, batch *core.IngestBatch) (IngestResult, error) {
	var res IngestResult
	if batch == nil {
		return res, nil
	}

	// Threads first, so items carrying a ThreadExternalID can attach to an
	// already-persisted thread.
	if err := p.ingestThreads(ctx, batch, &res); err != nil {
		return res, err
	}

	created := make([]*core.Node, 0, len(batch.Items))
	updated := make([]*core.Node, 0, len(batch.Items))
	// attachments records (item, persisted node) pairs that must be linked to a
	// thread after the nodes are saved.
	type attachment struct {
		item core.IngestItem
		node *core.Node
	}
	var attachments []attachment

	for _, item := range batch.Items {
		if item.Node == nil {
			continue
		}

		existing, err := p.lookupExisting(ctx, item)
		if err != nil {
			return res, err
		}

		node := mapNode(ctx, item, existing)

		if ctx.Source != nil {
			match, err := p.applySourceFilters(ctx.Source, node)
			if err != nil {
				return res, err
			}
			if !match {
				res.Skipped++
				continue
			}
		}

		if item.ThreadExternalID != "" {
			attachments = append(attachments, attachment{item: item, node: node})
		}

		if existing != nil {
			updated = append(updated, node)
		} else {
			created = append(created, node)
		}
	}

	if len(created) > 0 {
		if err := p.archive.SaveBatchNodes(created); err != nil {
			return res, fmt.Errorf("ingest: save created nodes: %w", err)
		}
		for _, n := range created {
			p.publish(ctx, core.EventNodeCreated, core.EventNodeCreatePayload{Node: n})
		}
		res.Created += len(created)
	}

	for _, n := range updated {
		if err := p.archive.SaveNode(n); err != nil {
			return res, fmt.Errorf("ingest: update node: %w", err)
		}
		p.publish(ctx, core.EventNodeUpdated, core.EventNodeUpdatePayload{NodeID: n.ID, UpdatedNode: n})
		res.Updated++
	}

	// Attach saved nodes to their threads. An unresolved thread is deferred
	// (retryable), not an error.
	for _, at := range attachments {
		applied, err := p.attachItemToThread(ctx, at.item, at.node)
		if err != nil {
			return res, err
		}
		if !applied {
			res.Deferred++
		}
	}

	if err := p.ingestEdges(ctx, batch, &res); err != nil {
		return res, err
	}

	if err := p.ingestAnnotations(ctx, batch, &res); err != nil {
		return res, err
	}

	// Persist the advanced cursor (source-based ingestion only).
	if batch.Cursor != nil && ctx.Source != nil {
		batch.Cursor.SourceID = ctx.Source.ID
		if err := p.archive.SaveIngestCursor(batch.Cursor); err != nil {
			return res, fmt.Errorf("ingest: persist cursor: %w", err)
		}
	}

	return res, nil
}

// ingestEdges resolves each IngestEdge's endpoints by node provenance and
// persists the edge. Edges require a Source (provenance) to resolve endpoints;
// without one the step is a no-op. An edge whose source or target node cannot be
// resolved is deferred (res.Deferred++) so the batch can be retried, never an
// error. Re-ingestion updates the existing edge in place.
func (p *Pipeline) ingestEdges(ctx IngestContext, batch *core.IngestBatch, res *IngestResult) error {
	if ctx.Source == nil {
		return nil // edges require provenance to resolve endpoints
	}
	for _, ie := range batch.Edges {
		srcNode, err := p.resolveNode(ctx.Source.ID, ie.SourceExternalID)
		if err != nil {
			return err
		}
		tgtNode, err := p.resolveNode(ctx.Source.ID, ie.TargetExternalID)
		if err != nil {
			return err
		}
		if srcNode == nil || tgtNode == nil {
			res.Deferred++
			slog.Warn("ingest: edge endpoint unresolved, deferring",
				"external_id", ie.ExternalID, "source_ext", ie.SourceExternalID, "target_ext", ie.TargetExternalID)
			continue
		}
		existing, err := p.lookupExistingEdge(ctx.Source.ID, ie.ExternalID)
		if err != nil {
			return err
		}
		edge := mapEdge(ctx, ie, srcNode.ID, tgtNode.ID, existing)
		if err := p.archive.SaveEdge(edge); err != nil {
			return fmt.Errorf("ingest: save edge: %w", err)
		}
		if existing != nil {
			res.Updated++
			p.publish(ctx, core.EventEdgeUpdated, core.EventEdgeUpdatePayload{EdgeID: edge.ID, UpdatedEdge: edge})
		} else {
			res.Created++
			p.publish(ctx, core.EventEdgeCreated, core.EventEdgeCreatePayload{Edge: edge})
		}
	}
	return nil
}

// resolveNode looks up a node by source provenance. An empty external ID yields
// (nil, nil), as does a not-found node — both signal "unresolved", which callers
// treat as a skip rather than an error.
func (p *Pipeline) resolveNode(sourceID uuid.UUID, externalID string) (*core.Node, error) {
	if externalID == "" {
		return nil, nil
	}
	n, err := p.archive.GetNodeBySourceExternalID(sourceID, externalID)
	if err == nil {
		return n, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: resolve node %q: %w", externalID, err)
}

// lookupExistingEdge finds a prior edge for dedup by source provenance. A
// not-found edge yields (nil, nil) so the caller creates a new one.
func (p *Pipeline) lookupExistingEdge(sourceID uuid.UUID, externalID string) (*core.Edge, error) {
	if externalID == "" {
		return nil, nil
	}
	e, err := p.archive.GetEdgeBySourceExternalID(sourceID, externalID)
	if err == nil {
		return e, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: edge dedup lookup: %w", err)
}

// mapEdge builds the edge to persist. On update, the existing edge's mutable
// fields are overwritten and it is returned (preserving ID/provenance/version).
// On create, a fresh edge is built with identity and provenance stamped from the
// ingest context.
func mapEdge(ctx IngestContext, ie core.IngestEdge, src, tgt uuid.UUID, existing *core.Edge) *core.Edge {
	if existing != nil {
		existing.Type = ie.Type
		existing.Label = ie.Label
		existing.Weight = ie.Weight
		existing.Source = src
		existing.Target = tgt
		return existing
	}
	sid := ctx.Source.ID
	ext := ie.ExternalID
	edge := &core.Edge{
		ID:        core.NewID(),
		RepoID:    ctx.RepoID,
		Type:      ie.Type,
		Source:    src,
		Target:    tgt,
		Label:     ie.Label,
		Weight:    ie.Weight,
		CreatedBy: ctx.CreatedBy,
		SourceID:  &sid,
	}
	if ext != "" {
		edge.ExternalID = &ext
	}
	return edge
}

// ingestThreads resolves each IngestThread to a Thread by (Source.ID,
// ExternalThreadID). A new thread is created with a root node of type "thread";
// an existing thread has its metadata (and root subject) refreshed. Threads
// require a Source for provenance; without one the step is a no-op.
func (p *Pipeline) ingestThreads(ctx IngestContext, batch *core.IngestBatch, res *IngestResult) error {
	if ctx.Source == nil {
		return nil // threads require provenance to dedup
	}
	for _, it := range batch.Threads {
		existing, err := p.lookupExistingThread(ctx.Source.ID, it.ExternalThreadID)
		if err != nil {
			return err
		}
		if existing == nil {
			if err := p.createThread(ctx, it, res); err != nil {
				return err
			}
			continue
		}
		if err := p.updateThread(ctx, it, existing, res); err != nil {
			return err
		}
	}
	return nil
}

// createThread builds the root node and thread for a new IngestThread, persists
// it, and emits EventThreadCreated. The root node holds an empty JSON-string
// body — threads are containers, so the meaningful content lives in member nodes.
func (p *Pipeline) createThread(ctx IngestContext, it core.IngestThread, res *IngestResult) error {
	sid := ctx.Source.ID
	ext := it.ExternalThreadID
	rootNode := core.Node{
		ID:          core.NewID(),
		RepoID:      ctx.RepoID,
		Type:        "thread",
		Subject:     it.Subject,
		ContentType: "text/plain",
		Body:        []byte(`""`),
		Metadata:    it.Metadata,
		CreatedBy:   ctx.CreatedBy,
	}
	thread := &core.Thread{
		ID:         core.NewID(),
		RepoID:     ctx.RepoID,
		Node:       rootNode,
		Metadata:   it.Metadata,
		CreatedBy:  ctx.CreatedBy,
		SourceID:   &sid,
		ExternalID: &ext,
	}
	if err := p.archive.SaveThread(thread); err != nil {
		return fmt.Errorf("ingest: save thread: %w", err)
	}
	p.publish(ctx, core.EventThreadCreated, core.EventThreadCreatePayload{Thread: thread})
	res.Created++
	return nil
}

// updateThread refreshes an existing thread's metadata and root-node subject,
// then emits EventThreadUpdated. The root-node subject is updated via SaveNode;
// SaveThread persists the metadata change.
func (p *Pipeline) updateThread(ctx IngestContext, it core.IngestThread, existing *core.Thread, res *IngestResult) error {
	existing.Metadata = it.Metadata
	if it.Subject != "" && existing.Node.Subject != it.Subject {
		existing.Node.Subject = it.Subject
		if err := p.archive.SaveNode(&existing.Node); err != nil {
			return fmt.Errorf("ingest: update thread root node: %w", err)
		}
	}
	if err := p.archive.SaveThread(existing); err != nil {
		return fmt.Errorf("ingest: update thread: %w", err)
	}
	p.publish(ctx, core.EventThreadUpdated, core.EventThreadUpdatePayload{ThreadID: existing.ID, UpdatedThread: existing})
	res.Updated++
	return nil
}

// lookupExistingThread finds a prior thread by source provenance. A not-found
// thread yields (nil, nil) so the caller creates a new one.
func (p *Pipeline) lookupExistingThread(sourceID uuid.UUID, externalID string) (*core.Thread, error) {
	if externalID == "" {
		return nil, nil
	}
	t, err := p.archive.GetThreadBySourceExternalID(sourceID, externalID)
	if err == nil {
		return t, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: thread dedup lookup: %w", err)
}

// attachItemToThread links a persisted item node to the thread named by the
// item's ThreadExternalID. It returns (true, nil) when the attachment is
// applied, and (false, nil) when there is nothing to attach (no source/thread
// reference) or the thread is unresolved (deferred — the caller increments
// Deferred so the batch is retried). A real archive error returns (false, err).
func (p *Pipeline) attachItemToThread(ctx IngestContext, item core.IngestItem, node *core.Node) (bool, error) {
	if ctx.Source == nil || item.ThreadExternalID == "" {
		return false, nil
	}
	thread, err := p.lookupExistingThread(ctx.Source.ID, item.ThreadExternalID)
	if err != nil {
		return false, err
	}
	if thread == nil {
		// Unresolved thread: defer so a later-arriving thread can resolve it.
		slog.Warn("ingest: thread unresolved for item attachment, deferring",
			"thread_ext", item.ThreadExternalID, "item_ext", item.ExternalID)
		return false, nil
	}
	if err := p.archive.AddNodeToThread(thread.ID, node.ID, ""); err != nil {
		return false, fmt.Errorf("ingest: add node to thread: %w", err)
	}
	p.publish(ctx, core.EventThreadNodeAdded, core.EventThreadNodePayload{ThreadID: thread.ID, NodeID: node.ID})
	return true, nil
}

// ingestAnnotations resolves each IngestAnnotation's target by node provenance
// and persists the annotation (content node + "annotates" edge). Annotations
// require a Source for provenance; without one the step is a no-op. An annotation
// whose target node cannot be resolved is deferred (res.Deferred++) so the batch
// can be retried, never an error. Re-ingestion updates the existing annotation in
// place.
func (p *Pipeline) ingestAnnotations(ctx IngestContext, batch *core.IngestBatch, res *IngestResult) error {
	if ctx.Source == nil {
		return nil // annotations require provenance to resolve targets
	}
	for _, ia := range batch.Annotations {
		tgt, err := p.resolveNode(ctx.Source.ID, ia.TargetExternalID)
		if err != nil {
			return err
		}
		if tgt == nil {
			res.Deferred++
			slog.Warn("ingest: annotation target unresolved, deferring",
				"external_id", ia.ExternalID, "target_ext", ia.TargetExternalID)
			continue
		}
		existing, err := p.lookupExistingAnnotation(ctx.Source.ID, ia.ExternalID)
		if err != nil {
			return err
		}
		if existing != nil {
			existing.Motivation = ia.Motivation
			if len(ia.Body) > 0 {
				existing.Node.Body = ia.Body
			}
			if err := p.archive.SaveAnnotation(existing); err != nil {
				return fmt.Errorf("ingest: update annotation: %w", err)
			}
			p.publish(ctx, core.EventAnnotationUpdated, core.EventAnnotationUpdatePayload{AnnotationID: existing.ID, UpdatedAnnotation: existing})
			res.Updated++
			continue
		}
		anno := mapAnnotation(ctx, ia, tgt.ID)
		if err := p.archive.SaveAnnotation(anno); err != nil {
			return fmt.Errorf("ingest: save annotation: %w", err)
		}
		p.publish(ctx, core.EventAnnotationCreated, core.EventAnnotationCreatePayload{Annotation: anno})
		res.Created++
	}
	return nil
}

// lookupExistingAnnotation finds a prior annotation by source provenance. A
// not-found annotation yields (nil, nil) so the caller creates a new one.
func (p *Pipeline) lookupExistingAnnotation(sourceID uuid.UUID, externalID string) (*core.Annotation, error) {
	if externalID == "" {
		return nil, nil
	}
	an, err := p.archive.GetAnnotationBySourceExternalID(sourceID, externalID)
	if err == nil {
		return an, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: annotation dedup lookup: %w", err)
}

// mapAnnotation builds a new annotation mirroring the canonical repo handler
// construction: a JSON content node and an "annotates" edge from that node to the
// resolved target, stamped with ingestion provenance.
func mapAnnotation(ctx IngestContext, ia core.IngestAnnotation, target uuid.UUID) *core.Annotation {
	sid := ctx.Source.ID
	ext := ia.ExternalID
	body := []byte(ia.Body)
	if len(body) == 0 {
		body = []byte(`""`)
	}
	annoNodeID := core.NewID()
	anno := &core.Annotation{
		ID:     core.NewID(),
		RepoID: ctx.RepoID,
		Node: core.Node{
			ID:          annoNodeID,
			RepoID:      ctx.RepoID,
			Type:        "annotation",
			ContentType: "application/json",
			Body:        body,
			CreatedBy:   ctx.CreatedBy,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    ctx.RepoID,
			Type:      "annotates",
			Source:    annoNodeID,
			Target:    target,
			Label:     ia.Motivation,
			Weight:    1.0,
			CreatedBy: ctx.CreatedBy,
		},
		Motivation: ia.Motivation,
		CreatedBy:  ctx.CreatedBy,
		SourceID:   &sid,
	}
	if ext != "" {
		anno.ExternalID = &ext
	}
	return anno
}

// ApplyThreadMembership resolves a thread and node by source provenance and adds
// or removes the membership. It returns (true, nil) when the change was applied
// (and the event published), and (false, nil) when there is no source or the
// thread or node is unresolved (deferred — the caller should Nak so a
// later-arriving entity can resolve it). A real archive error returns
// (false, err). It is used by the connector supervisor to apply membership
// changes that arrive separately from item ingestion.
func (p *Pipeline) ApplyThreadMembership(ctx IngestContext, threadExternalID, nodeExternalID string, add bool) (bool, error) {
	if ctx.Source == nil {
		return false, nil
	}
	thread, err := p.lookupExistingThread(ctx.Source.ID, threadExternalID)
	if err != nil {
		return false, err
	}
	node, err := p.resolveNode(ctx.Source.ID, nodeExternalID)
	if err != nil {
		return false, err
	}
	if thread == nil || node == nil {
		return false, nil // unresolved: deferred no-op
	}
	if add {
		if err := p.archive.AddNodeToThread(thread.ID, node.ID, ""); err != nil {
			return false, fmt.Errorf("ingest: add node to thread: %w", err)
		}
		p.publish(ctx, core.EventThreadNodeAdded, core.EventThreadNodePayload{ThreadID: thread.ID, NodeID: node.ID})
		return true, nil
	}
	if err := p.archive.RemoveNodeFromThread(thread.ID, node.ID); err != nil {
		return false, fmt.Errorf("ingest: remove node from thread: %w", err)
	}
	p.publish(ctx, core.EventThreadNodeRemoved, core.EventThreadNodePayload{ThreadID: thread.ID, NodeID: node.ID})
	return true, nil
}

// lookupExisting finds a prior node for dedup. Only source-based items with a
// non-empty ExternalID can dedup; ad-hoc items always create.
func (p *Pipeline) lookupExisting(ctx IngestContext, item core.IngestItem) (*core.Node, error) {
	if ctx.Source == nil || item.ExternalID == "" {
		return nil, nil
	}
	n, err := p.archive.GetNodeBySourceExternalID(ctx.Source.ID, item.ExternalID)
	if err == nil {
		return n, nil
	}
	if errors.As(err, new(*core.ErrorNotFound)) {
		return nil, nil
	}
	return nil, fmt.Errorf("ingest: dedup lookup: %w", err)
}

// mapNode builds the node to persist. A new node (Version 0) is assigned
// identity + provenance by the archive on save; an existing node is updated in
// place, preserving ID/provenance/version (the archive bumps the version and
// leaves provenance columns untouched on update).
func mapNode(ctx IngestContext, item core.IngestItem, existing *core.Node) *core.Node {
	src := item.Node
	if existing != nil {
		existing.Type = src.Type
		existing.Subject = src.Subject
		existing.ContentType = src.ContentType
		existing.Body = src.Body
		existing.Metadata = src.Metadata
		return existing
	}
	node := &core.Node{
		ID:          core.NewID(),
		RepoID:      ctx.RepoID,
		Type:        src.Type,
		Subject:     src.Subject,
		ContentType: src.ContentType,
		Body:        src.Body,
		Metadata:    src.Metadata,
		CreatedBy:   ctx.CreatedBy,
	}
	if ctx.Source != nil {
		sid := ctx.Source.ID
		node.SourceID = &sid
		if item.ExternalID != "" {
			ext := item.ExternalID
			node.ExternalID = &ext
		}
	}
	return node
}

// applySourceFilters evaluates the source's filter chain against the normalised
// node, delegating to the shared MatchesFilterChain helper.
func (p *Pipeline) applySourceFilters(source *core.Source, node *core.Node) (bool, error) {
	return MatchesFilterChain(p.archive, source.FilterIDs, source.FilterChainMode, node)
}

// MatchesFilterChain reports whether a node satisfies a filter chain (by id)
// under the given mode. An empty chain matches. An unset mode defaults to "all"
// (AND): every filter must match; FilterModeAny requires at least one. Shared by
// the pipeline's source filtering and the publish supervisor's Sink filtering.
func MatchesFilterChain(archive core.Archive, filterIDs []uuid.UUID, mode core.FilterMode, node *core.Node) (bool, error) {
	if len(filterIDs) == 0 {
		return true, nil
	}

	// An unset chain mode defaults to "all" (AND), matching filter.Evaluate's
	// handling of an empty filter Mode.
	if mode == "" {
		mode = core.FilterModeAll
	}

	payload := nodePayload(node)
	anyMatched := false
	for _, fid := range filterIDs {
		f, err := archive.GetFilter(fid)
		if err != nil {
			return false, fmt.Errorf("ingest: load filter %s: %w", fid, err)
		}
		matched := filter.Evaluate(f, payload).Matches
		if mode == core.FilterModeAll && !matched {
			return false, nil
		}
		if matched {
			anyMatched = true
		}
	}
	if mode == core.FilterModeAny {
		return anyMatched, nil
	}
	return true, nil
}

// nodePayload projects a node into the map the filter engine evaluates. Metadata
// is flattened to key->raw-value; body is exposed as a string for text filters.
func nodePayload(n *core.Node) map[string]any {
	meta := make(map[string]any, len(n.Metadata))
	for _, m := range n.Metadata {
		meta[m.Key] = m.Value
	}
	return map[string]any{
		"type":         n.Type,
		"subject":      n.Subject,
		"content_type": n.ContentType,
		"body":         string(n.Body),
		"metadata":     meta,
	}
}

// publish emits a domain event for an ingested entity, fire-and-forget,
// propagating the inbound hop count (+1) so repo->repo cascades stay bounded.
// A nil publisher is a no-op.
func (p *Pipeline) publish(ctx IngestContext, t core.EventType, payload any) {
	if p.pub == nil {
		return
	}
	evt, err := core.NewEvent(t, ctx.RepoID, payload)
	if err != nil {
		slog.Error("ingest: build event failed", "type", t, "error", err)
		return
	}
	evt.Hops = ctx.InboundHops + 1
	data, err := json.Marshal(evt)
	if err != nil {
		slog.Error("ingest: marshal event failed", "type", t, "error", err)
		return
	}
	if err := p.pub.Publish(evt.Subject(), data); err != nil {
		slog.Error("ingest: publish event failed", "type", t, "subject", evt.Subject(), "error", err)
	}
}
