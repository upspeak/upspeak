package ingest

import (
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
}

// IngestResult summarises what a batch produced.
type IngestResult struct {
	Created int
	Updated int
	Skipped int // filtered out by the source filter chain
}

// Pipeline turns adapter-emitted IngestBatches into persisted graph entities,
// reusing the canonical write+event path (SaveBatchNodes/SaveNode +
// PublishEvent) so rules, realtime, and search light up for ingested data with
// no extra wiring. A2 implements the Item path; Threads, Annotations,
// Tombstones, reply Edges, and author->User resolution land in Sub-project B.
type Pipeline struct {
	archive core.Archive
	pub     app.Publisher
}

// NewPipeline creates a pipeline. pub may be nil (events are then skipped).
func NewPipeline(archive core.Archive, pub app.Publisher) *Pipeline {
	return &Pipeline{archive: archive, pub: pub}
}

// Ingest persists a batch's Items into ctx.RepoID. Items entering through a
// Source are deduplicated by provenance (re-collection updates the existing
// node) and filtered through the source's chain. On error mid-batch, the
// returned IngestResult reflects operations completed before the failure.
func (p *Pipeline) Ingest(ctx IngestContext, batch *core.IngestBatch) (IngestResult, error) {
	var res IngestResult
	if batch == nil {
		return res, nil
	}

	created := make([]*core.Node, 0, len(batch.Items))
	updated := make([]*core.Node, 0, len(batch.Items))

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
			p.publish(ctx.RepoID, core.EventNodeCreated, core.EventNodeCreatePayload{Node: n})
		}
		res.Created = len(created)
	}

	for _, n := range updated {
		if err := p.archive.SaveNode(n); err != nil {
			return res, fmt.Errorf("ingest: update node: %w", err)
		}
		p.publish(ctx.RepoID, core.EventNodeUpdated, core.EventNodeUpdatePayload{NodeID: n.ID, UpdatedNode: n})
		res.Updated++
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
// node. Empty chain matches. FilterModeAll requires every filter to match;
// FilterModeAny requires at least one.
func (p *Pipeline) applySourceFilters(source *core.Source, node *core.Node) (bool, error) {
	if len(source.FilterIDs) == 0 {
		return true, nil
	}

	// An unset chain mode defaults to "all" (AND), matching filter.Evaluate's
	// handling of an empty filter Mode.
	mode := source.FilterChainMode
	if mode == "" {
		mode = core.FilterModeAll
	}

	payload := nodePayload(node)
	anyMatched := false
	for _, fid := range source.FilterIDs {
		f, err := p.archive.GetFilter(fid)
		if err != nil {
			return false, fmt.Errorf("ingest: load source filter %s: %w", fid, err)
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

// publish emits a domain event, fire-and-forget. A nil publisher is a no-op.
func (p *Pipeline) publish(repoID uuid.UUID, t core.EventType, payload any) {
	if p.pub == nil {
		return
	}
	if err := p.pub.PublishEvent(t, repoID, payload); err != nil {
		slog.Error("ingest: publish event failed", "type", t, "error", err)
	}
}
