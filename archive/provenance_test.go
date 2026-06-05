//go:build sqlite_fts5

package archive

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// saveBareNode creates a minimal node and returns its ID.
func saveBareNode(t *testing.T, a *LocalArchive, repoID uuid.UUID) uuid.UUID {
	t.Helper()
	n := &core.Node{
		ID:          core.NewID(),
		RepoID:      repoID,
		Type:        "note",
		Subject:     "bare node",
		ContentType: "text/plain",
		Body:        json.RawMessage(`""`),
		CreatedBy:   testOwnerID,
	}
	if err := a.SaveNode(n); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return n.ID
}

// TestEdgeProvenancePersists saves an edge with SourceID and ExternalID set,
// reads it back, and asserts the provenance fields survive the round-trip.
func TestEdgeProvenancePersists(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	src := saveBareNode(t, a, repo.ID)
	tgt := saveBareNode(t, a, repo.ID)

	sid := uuid.New()
	ext := "edge-ext-1"
	e := &core.Edge{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Type:       "rel",
		Source:     src,
		Target:     tgt,
		Weight:     1,
		CreatedBy:  testOwnerID,
		SourceID:   &sid,
		ExternalID: &ext,
	}
	if err := a.SaveEdge(e); err != nil {
		t.Fatalf("save edge: %v", err)
	}
	got, err := a.GetEdge(e.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid {
		t.Fatalf("edge SourceID lost: got %v, want %v", got.SourceID, sid)
	}
	if got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("edge ExternalID lost: got %v, want %q", got.ExternalID, ext)
	}
}

// TestThreadProvenancePersists saves a thread with SourceID and ExternalID set,
// reads it back, and asserts the provenance fields survive the round-trip.
func TestThreadProvenancePersists(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	sid := uuid.New()
	ext := "thread-ext-1"
	th := &core.Thread{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Node: core.Node{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Type:        "thread-root",
			Subject:     "provenance thread",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"root body"`),
			CreatedBy:   testOwnerID,
		},
		CreatedBy:  testOwnerID,
		SourceID:   &sid,
		ExternalID: &ext,
	}
	if err := a.SaveThread(th); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	got, err := a.GetThread(th.ID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid {
		t.Fatalf("thread SourceID lost: got %v, want %v", got.SourceID, sid)
	}
	if got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("thread ExternalID lost: got %v, want %q", got.ExternalID, ext)
	}
}

// TestEdgeProvenanceNilForLocal saves an edge WITHOUT provenance and asserts that
// SourceID and ExternalID are both nil after the round-trip.
func TestEdgeProvenanceNilForLocal(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	src := saveBareNode(t, a, repo.ID)
	tgt := saveBareNode(t, a, repo.ID)

	e := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      "rel",
		Source:    src,
		Target:    tgt,
		Weight:    1,
		CreatedBy: testOwnerID,
	}
	if err := a.SaveEdge(e); err != nil {
		t.Fatalf("save edge: %v", err)
	}
	got, err := a.GetEdge(e.ID)
	if err != nil {
		t.Fatalf("get edge: %v", err)
	}
	if got.SourceID != nil {
		t.Fatalf("edge SourceID: want nil, got %v", got.SourceID)
	}
	if got.ExternalID != nil {
		t.Fatalf("edge ExternalID: want nil, got %v", got.ExternalID)
	}
}

// TestThreadProvenanceNilForLocal saves a thread WITHOUT provenance and asserts
// that SourceID and ExternalID are both nil after the round-trip.
func TestThreadProvenanceNilForLocal(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	th := &core.Thread{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Node: core.Node{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Type:        "thread-root",
			Subject:     "local thread",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"root body"`),
			CreatedBy:   testOwnerID,
		},
		CreatedBy: testOwnerID,
	}
	if err := a.SaveThread(th); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	got, err := a.GetThread(th.ID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got.SourceID != nil {
		t.Fatalf("thread SourceID: want nil, got %v", got.SourceID)
	}
	if got.ExternalID != nil {
		t.Fatalf("thread ExternalID: want nil, got %v", got.ExternalID)
	}
}

// TestAnnotationProvenanceNilForLocal saves an annotation WITHOUT provenance
// and asserts that SourceID and ExternalID are both nil after the round-trip.
func TestAnnotationProvenanceNilForLocal(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	targetID := saveBareNode(t, a, repo.ID)

	annotationNodeID := core.NewID()
	ann := &core.Annotation{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Motivation: "commenting",
		CreatedBy:  testOwnerID,
		Node: core.Node{
			ID:          annotationNodeID,
			RepoID:      repo.ID,
			Type:        "annotation",
			Subject:     "local annotation",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"annotation body"`),
			CreatedBy:   testOwnerID,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Type:      "annotation",
			Source:    annotationNodeID,
			Target:    targetID,
			Label:     "annotates",
			Weight:    1.0,
			CreatedBy: testOwnerID,
		},
	}
	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("save annotation: %v", err)
	}
	got, err := a.GetAnnotation(ann.ID)
	if err != nil {
		t.Fatalf("get annotation: %v", err)
	}
	if got.SourceID != nil {
		t.Fatalf("annotation SourceID: want nil, got %v", got.SourceID)
	}
	if got.ExternalID != nil {
		t.Fatalf("annotation ExternalID: want nil, got %v", got.ExternalID)
	}
}

// TestAnnotationProvenancePersists saves an annotation with SourceID and ExternalID
// set, reads it back, and asserts the provenance fields survive the round-trip.
func TestAnnotationProvenancePersists(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Create a target node for the annotation to point at.
	targetID := saveBareNode(t, a, repo.ID)

	sid := uuid.New()
	ext := "anno-ext-1"
	annotationNodeID := core.NewID()
	ann := &core.Annotation{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Motivation: "commenting",
		CreatedBy:  testOwnerID,
		SourceID:   &sid,
		ExternalID: &ext,
		Node: core.Node{
			ID:          annotationNodeID,
			RepoID:      repo.ID,
			Type:        "annotation",
			Subject:     "provenance annotation",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"annotation body"`),
			CreatedBy:   testOwnerID,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Type:      "annotation",
			Source:    annotationNodeID,
			Target:    targetID,
			Label:     "annotates",
			Weight:    1.0,
			CreatedBy: testOwnerID,
		},
	}
	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("save annotation: %v", err)
	}
	got, err := a.GetAnnotation(ann.ID)
	if err != nil {
		t.Fatalf("get annotation: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid {
		t.Fatalf("annotation SourceID lost: got %v, want %v", got.SourceID, sid)
	}
	if got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("annotation ExternalID lost: got %v, want %q", got.ExternalID, ext)
	}
}
