package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// makeAnnotation builds an Annotation targeting the given node. The embedded
// Node (annotation content) and Edge (linking content to target) are populated
// with fresh IDs.
func makeAnnotation(repo *core.Repository, target *core.Node, motivation string) *core.Annotation {
	annotationNodeID := core.NewID()
	return &core.Annotation{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Motivation: motivation,
		CreatedBy:  testOwnerID,
		Node: core.Node{
			ID:          annotationNodeID,
			RepoID:      repo.ID,
			Type:        "annotation",
			Subject:     "Annotation on target",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"annotation body"`),
			Metadata:    []core.Metadata{{Key: "colour", Value: json.RawMessage(`"yellow"`)}},
			CreatedBy:   testOwnerID,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Type:      "annotation",
			Source:    annotationNodeID,
			Target:    target.ID,
			Label:     "annotates",
			Weight:    1.0,
			CreatedBy: testOwnerID,
		},
	}
}

func TestSaveAndGetAnnotation(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Create a target node.
	target := makeNode(repo, "note", "Target for Annotation")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	ann := makeAnnotation(repo, target, "commenting")

	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	if ann.Version != 1 {
		t.Errorf("Expected version 1 after create, got %d", ann.Version)
	}
	if ann.ShortID != "ANNO-1" {
		t.Errorf("Expected short ID ANNO-1, got %s", ann.ShortID)
	}
	if ann.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	// Embedded node and edge should have been saved.
	if ann.Node.ShortID == "" {
		t.Error("Expected annotation node to have a short ID")
	}
	if ann.Edge.ShortID == "" {
		t.Error("Expected annotation edge to have a short ID")
	}

	got, err := a.GetAnnotation(ann.ID)
	if err != nil {
		t.Fatalf("GetAnnotation failed: %v", err)
	}

	if got.ID != ann.ID {
		t.Errorf("Expected ID %s, got %s", ann.ID, got.ID)
	}
	if got.RepoID != repo.ID {
		t.Errorf("Expected RepoID %s, got %s", repo.ID, got.RepoID)
	}
	if got.ShortID != "ANNO-1" {
		t.Errorf("Expected short ID ANNO-1, got %s", got.ShortID)
	}
	if got.Motivation != "commenting" {
		t.Errorf("Expected motivation 'commenting', got %q", got.Motivation)
	}
	if got.CreatedBy != testOwnerID {
		t.Errorf("Expected created_by %s, got %s", testOwnerID, got.CreatedBy)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}

	// Verify embedded node.
	if got.Node.ID != ann.Node.ID {
		t.Errorf("Expected annotation node ID %s, got %s", ann.Node.ID, got.Node.ID)
	}
	if got.Node.Subject != "Annotation on target" {
		t.Errorf("Expected node subject 'Annotation on target', got %q", got.Node.Subject)
	}
	if string(got.Node.Body) != `"annotation body"` {
		t.Errorf("Expected node body '\"annotation body\"', got %q", string(got.Node.Body))
	}

	// Verify embedded edge.
	if got.Edge.ID != ann.Edge.ID {
		t.Errorf("Expected annotation edge ID %s, got %s", ann.Edge.ID, got.Edge.ID)
	}
	if got.Edge.Source != ann.Node.ID {
		t.Errorf("Expected edge source %s, got %s", ann.Node.ID, got.Edge.Source)
	}
	if got.Edge.Target != target.ID {
		t.Errorf("Expected edge target %s, got %s", target.ID, got.Edge.Target)
	}
	if got.Edge.Type != "annotation" {
		t.Errorf("Expected edge type 'annotation', got %q", got.Edge.Type)
	}
}

func TestSaveAnnotation_Update(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	target := makeNode(repo, "note", "Target")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	ann := makeAnnotation(repo, target, "commenting")
	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update motivation and node body.
	ann.Motivation = "highlighting"
	ann.Node.Body = json.RawMessage(`"updated annotation body"`)
	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if ann.Version != 2 {
		t.Errorf("Expected version 2 after update, got %d", ann.Version)
	}

	got, err := a.GetAnnotation(ann.ID)
	if err != nil {
		t.Fatalf("GetAnnotation after update failed: %v", err)
	}
	if got.Motivation != "highlighting" {
		t.Errorf("Expected motivation 'highlighting', got %q", got.Motivation)
	}
	if string(got.Node.Body) != `"updated annotation body"` {
		t.Errorf("Expected updated node body, got %q", string(got.Node.Body))
	}
	if got.Version != 2 {
		t.Errorf("Expected version 2, got %d", got.Version)
	}
	// The embedded node version should also have been bumped (saveNode update).
	if got.Node.Version != 2 {
		t.Errorf("Expected node version 2 after annotation update, got %d", got.Node.Version)
	}
}

func TestDeleteAnnotation(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	target := makeNode(repo, "note", "Target for Deletion")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	ann := makeAnnotation(repo, target, "commenting")
	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	annNodeID := ann.Node.ID
	annEdgeID := ann.Edge.ID

	if err := a.DeleteAnnotation(ann.ID); err != nil {
		t.Fatalf("DeleteAnnotation failed: %v", err)
	}

	// Annotation should be gone.
	_, err := a.GetAnnotation(ann.ID)
	if err == nil {
		t.Error("Expected error after annotation deletion, got nil")
	}

	// Embedded node should be deleted.
	_, err = a.GetNode(annNodeID)
	if err == nil {
		t.Error("Expected annotation node to be deleted")
	}

	// Embedded edge should be deleted.
	_, err = a.GetEdge(annEdgeID)
	if err == nil {
		t.Error("Expected annotation edge to be deleted")
	}

	// Target node should still exist.
	_, err = a.GetNode(target.ID)
	if err != nil {
		t.Errorf("Target node should still exist: %v", err)
	}
}

func TestListAnnotations(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	target := makeNode(repo, "note", "Target for List")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		ann := makeAnnotation(repo, target, "commenting")
		if err := a.SaveAnnotation(ann); err != nil {
			t.Fatalf("SaveAnnotation %d failed: %v", i, err)
		}
	}

	// List all annotations.
	annotations, total, err := a.ListAnnotations(repo.ID, core.ListOptions{Limit: 10, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListAnnotations failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
	if len(annotations) != 3 {
		t.Errorf("Expected 3 annotations, got %d", len(annotations))
	}

	// Verify each annotation has hydrated node and edge.
	for i, ann := range annotations {
		if ann.Node.ID.String() == "00000000-0000-0000-0000-000000000000" {
			t.Errorf("Annotation %d: expected hydrated node, got zero ID", i)
		}
		if ann.Edge.ID.String() == "00000000-0000-0000-0000-000000000000" {
			t.Errorf("Annotation %d: expected hydrated edge, got zero ID", i)
		}
	}

	// Test pagination: limit 2, offset 0.
	page1, pagTotal, err := a.ListAnnotations(repo.ID, core.ListOptions{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListAnnotations (page 1) failed: %v", err)
	}
	if pagTotal != 3 {
		t.Errorf("Expected total 3 with pagination, got %d", pagTotal)
	}
	if len(page1) != 2 {
		t.Errorf("Expected 2 annotations in page 1, got %d", len(page1))
	}

	// Pagination: limit 2, offset 2 (should get 1 result).
	page2, _, err := a.ListAnnotations(repo.ID, core.ListOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListAnnotations (page 2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("Expected 1 annotation in last page, got %d", len(page2))
	}
}
