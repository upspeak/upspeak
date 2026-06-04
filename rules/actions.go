package rules

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/core"
)

// enrichParams configures an enrich action: add or update a single metadata
// entry on the triggering node. A null metadata_value removes the key.
type enrichParams struct {
	MetadataKey   string          `json:"metadata_key"`
	MetadataValue json.RawMessage `json:"metadata_value"`
}

// relateParams configures a relate action: either create an edge from the
// triggering node to a target node, or add the triggering node to a thread.
type relateParams struct {
	TargetNodeID   string `json:"target_node_id"`
	TargetThreadID string `json:"target_thread_id"`
	EdgeType       string `json:"edge_type"`
}

// annotateParams configures an annotate action: create an annotation on the
// triggering node.
type annotateParams struct {
	Motivation  string          `json:"motivation"`
	Body        json.RawMessage `json:"body"`
	ContentType string          `json:"content_type"`
}

// enrichNode applies an enrich action: it merges a single metadata entry into
// the triggering node and persists it. The node must belong to the rule's
// repository.
func (e *Engine) enrichNode(rule *core.Rule, payload map[string]any, raw json.RawMessage) error {
	var p enrichParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid enrich params: %w", err)
	}
	if p.MetadataKey == "" {
		return fmt.Errorf("enrich requires metadata_key")
	}

	node, err := e.triggeringNode(rule, payload)
	if err != nil {
		return err
	}

	node.Metadata = mergeOneMetadata(node.Metadata, p.MetadataKey, p.MetadataValue)
	if err := e.archive.SaveNode(node); err != nil {
		return fmt.Errorf("failed to enrich node: %w", err)
	}
	return nil
}

// relateNode applies a relate action: it links the triggering node to a target
// node (via a new edge) or to a thread (via thread membership).
func (e *Engine) relateNode(rule *core.Rule, payload map[string]any, raw json.RawMessage) error {
	var p relateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid relate params: %w", err)
	}

	nodeID, err := triggeringNodeID(payload)
	if err != nil {
		return err
	}

	edgeType := p.EdgeType
	if edgeType == "" {
		edgeType = "related"
	}

	switch {
	case p.TargetNodeID != "":
		targetID, entityType, err := e.archive.ResolveRef(rule.RepoID, p.TargetNodeID)
		if err != nil || entityType != "node" {
			return fmt.Errorf("relate target_node_id %q does not resolve to a node in this repository", p.TargetNodeID)
		}
		edge := &core.Edge{
			ID:        core.NewID(),
			RepoID:    rule.RepoID,
			Type:      edgeType,
			Source:    nodeID,
			Target:    targetID,
			Weight:    1.0,
			CreatedBy: rule.CreatedBy,
		}
		if err := e.archive.SaveEdge(edge); err != nil {
			return fmt.Errorf("failed to create edge: %w", err)
		}
	case p.TargetThreadID != "":
		threadID, entityType, err := e.archive.ResolveRef(rule.RepoID, p.TargetThreadID)
		if err != nil || entityType != "thread" {
			return fmt.Errorf("relate target_thread_id %q does not resolve to a thread in this repository", p.TargetThreadID)
		}
		if err := e.archive.AddNodeToThread(threadID, nodeID, edgeType); err != nil {
			return fmt.Errorf("failed to add node to thread: %w", err)
		}
	default:
		return fmt.Errorf("relate requires target_node_id or target_thread_id")
	}
	return nil
}

// annotateNode applies an annotate action: it creates an annotation (a body node
// plus an "annotates" edge) targeting the triggering node.
func (e *Engine) annotateNode(rule *core.Rule, payload map[string]any, raw json.RawMessage) error {
	var p annotateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("invalid annotate params: %w", err)
	}
	if p.Motivation == "" {
		return fmt.Errorf("annotate requires motivation")
	}

	targetID, err := triggeringNodeID(payload)
	if err != nil {
		return err
	}

	contentType := p.ContentType
	if contentType == "" {
		contentType = "text/plain"
	}

	annoNodeID := core.NewID()
	annotation := &core.Annotation{
		ID:     core.NewID(),
		RepoID: rule.RepoID,
		Node: core.Node{
			ID:          annoNodeID,
			RepoID:      rule.RepoID,
			Type:        "annotation",
			ContentType: contentType,
			Body:        p.Body,
			CreatedBy:   rule.CreatedBy,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    rule.RepoID,
			Type:      "annotates",
			Source:    annoNodeID,
			Target:    targetID,
			Label:     p.Motivation,
			Weight:    1.0,
			CreatedBy: rule.CreatedBy,
		},
		Motivation: p.Motivation,
		CreatedBy:  rule.CreatedBy,
	}
	if err := e.archive.SaveAnnotation(annotation); err != nil {
		return fmt.Errorf("failed to create annotation: %w", err)
	}
	return nil
}

// triggeringNode loads the node that triggered the rule and verifies it belongs
// to the rule's repository.
func (e *Engine) triggeringNode(rule *core.Rule, payload map[string]any) (*core.Node, error) {
	nodeID, err := triggeringNodeID(payload)
	if err != nil {
		return nil, err
	}
	node, err := e.archive.GetNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load triggering node: %w", err)
	}
	if node.RepoID != rule.RepoID {
		return nil, fmt.Errorf("triggering node does not belong to the rule's repository")
	}
	return node, nil
}

// triggeringNodeID extracts the triggering node's UUID from a normalised event
// payload. It prefers an embedded node object (Created/Updated/Patched, the
// latter after normalisation) and falls back to a bare node_id (Deleted).
func triggeringNodeID(payload map[string]any) (uuid.UUID, error) {
	if n, ok := payload["node"].(map[string]any); ok {
		if idStr, ok := n["id"].(string); ok {
			if id, err := uuid.Parse(idStr); err == nil {
				return id, nil
			}
		}
	}
	if idStr, ok := payload["node_id"].(string); ok {
		if id, err := uuid.Parse(idStr); err == nil {
			return id, nil
		}
	}
	return uuid.Nil, fmt.Errorf("event has no triggering node; this action requires a node-bearing event")
}

// mergeOneMetadata returns a copy of existing with the given key set to value,
// preserving order. A nil or JSON-null value removes the key.
func mergeOneMetadata(existing []core.Metadata, key string, value json.RawMessage) []core.Metadata {
	remove := value == nil || string(value) == "null"
	out := make([]core.Metadata, 0, len(existing)+1)
	replaced := false
	for _, md := range existing {
		if md.Key == key {
			replaced = true
			if !remove {
				out = append(out, core.Metadata{Key: key, Value: value})
			}
			continue
		}
		out = append(out, md)
	}
	if !replaced && !remove {
		out = append(out, core.Metadata{Key: key, Value: value})
	}
	return out
}

// normaliseEventPayload aliases per-event-type entity keys to a stable name so
// filter conditions and actions can reference the entity uniformly. Created
// events nest the entity under e.g. "node"; Updated/Patched events nest it under
// "updated_node". This copies the latter to the former when the former is absent.
func normaliseEventPayload(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, entity := range []string{"node", "edge", "thread", "annotation"} {
		if _, ok := payload[entity]; ok {
			continue
		}
		if updated, ok := payload["updated_"+entity]; ok {
			payload[entity] = updated
		}
	}
}
