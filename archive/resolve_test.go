package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestResolveRef_ByUUID(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "Resolve by UUID")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	id, entityType, err := a.ResolveRef(repo.ID, node.ID.String())
	if err != nil {
		t.Fatalf("ResolveRef by UUID failed: %v", err)
	}
	if id != node.ID {
		t.Errorf("Expected resolved ID %s, got %s", node.ID, id)
	}
	if entityType != "node" {
		t.Errorf("Expected entity type 'node', got %q", entityType)
	}
}

func TestResolveRef_ByShortID(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "Resolve by Short ID")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	// The first node in this repo should be NODE-1.
	id, entityType, err := a.ResolveRef(repo.ID, "NODE-1")
	if err != nil {
		t.Fatalf("ResolveRef by short ID failed: %v", err)
	}
	if id != node.ID {
		t.Errorf("Expected resolved ID %s, got %s", node.ID, id)
	}
	if entityType != "node" {
		t.Errorf("Expected entity type 'node', got %q", entityType)
	}
}

func TestResolveRef_Edge(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	nodeA, nodeB := createTwoNodes(t, a, repo)

	edge := makeEdge(repo, nodeA, nodeB, "reply", "Test Edge")
	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("SaveEdge failed: %v", err)
	}

	// Resolve by UUID.
	id, entityType, err := a.ResolveRef(repo.ID, edge.ID.String())
	if err != nil {
		t.Fatalf("ResolveRef edge by UUID failed: %v", err)
	}
	if id != edge.ID {
		t.Errorf("Expected resolved ID %s, got %s", edge.ID, id)
	}
	if entityType != "edge" {
		t.Errorf("Expected entity type 'edge', got %q", entityType)
	}

	// Resolve by short ID (EDGE-1).
	id, entityType, err = a.ResolveRef(repo.ID, "EDGE-1")
	if err != nil {
		t.Fatalf("ResolveRef edge by short ID failed: %v", err)
	}
	if id != edge.ID {
		t.Errorf("Expected resolved ID %s, got %s", edge.ID, id)
	}
	if entityType != "edge" {
		t.Errorf("Expected entity type 'edge', got %q", entityType)
	}
}

func TestResolveRef_Thread(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Resolve Thread")
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("SaveThread failed: %v", err)
	}

	// Resolve by UUID.
	id, entityType, err := a.ResolveRef(repo.ID, thread.ID.String())
	if err != nil {
		t.Fatalf("ResolveRef thread by UUID failed: %v", err)
	}
	if id != thread.ID {
		t.Errorf("Expected resolved ID %s, got %s", thread.ID, id)
	}
	if entityType != "thread" {
		t.Errorf("Expected entity type 'thread', got %q", entityType)
	}

	// Resolve by short ID (THREAD-1).
	id, entityType, err = a.ResolveRef(repo.ID, "THREAD-1")
	if err != nil {
		t.Fatalf("ResolveRef thread by short ID failed: %v", err)
	}
	if id != thread.ID {
		t.Errorf("Expected resolved ID %s, got %s", thread.ID, id)
	}
	if entityType != "thread" {
		t.Errorf("Expected entity type 'thread', got %q", entityType)
	}
}

func TestResolveRef_Annotation(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	target := makeNode(repo, "note", "Target for Resolve")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	ann := &core.Annotation{
		ID:         core.NewID(),
		RepoID:     repo.ID,
		Motivation: "commenting",
		CreatedBy:  testOwnerID,
		Node: core.Node{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Type:        "annotation",
			Subject:     "Annotation content",
			ContentType: "text/plain",
			Body:        json.RawMessage(`"annotation body"`),
			CreatedBy:   testOwnerID,
		},
		Edge: core.Edge{
			ID:        core.NewID(),
			RepoID:    repo.ID,
			Type:      "annotation",
			Target:    target.ID,
			Weight:    1.0,
			CreatedBy: testOwnerID,
		},
	}
	ann.Edge.Source = ann.Node.ID

	if err := a.SaveAnnotation(ann); err != nil {
		t.Fatalf("SaveAnnotation failed: %v", err)
	}

	// Resolve by UUID.
	id, entityType, err := a.ResolveRef(repo.ID, ann.ID.String())
	if err != nil {
		t.Fatalf("ResolveRef annotation by UUID failed: %v", err)
	}
	if id != ann.ID {
		t.Errorf("Expected resolved ID %s, got %s", ann.ID, id)
	}
	if entityType != "annotation" {
		t.Errorf("Expected entity type 'annotation', got %q", entityType)
	}

	// Resolve by short ID (ANNO-1).
	id, entityType, err = a.ResolveRef(repo.ID, "ANNO-1")
	if err != nil {
		t.Fatalf("ResolveRef annotation by short ID failed: %v", err)
	}
	if id != ann.ID {
		t.Errorf("Expected resolved ID %s, got %s", ann.ID, id)
	}
	if entityType != "annotation" {
		t.Errorf("Expected entity type 'annotation', got %q", entityType)
	}
}

func TestResolveRef_NotFound(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Non-existent UUID.
	fakeID := core.NewID()
	_, _, err := a.ResolveRef(repo.ID, fakeID.String())
	if err == nil {
		t.Error("Expected error for nonexistent UUID, got nil")
	}
	if _, ok := err.(*core.ErrorNotFound); !ok {
		t.Errorf("Expected *ErrorNotFound for UUID, got %T: %v", err, err)
	}

	// Non-existent short ID.
	_, _, err = a.ResolveRef(repo.ID, "NODE-9999")
	if err == nil {
		t.Error("Expected error for nonexistent short ID, got nil")
	}
	if _, ok := err.(*core.ErrorNotFound); !ok {
		t.Errorf("Expected *ErrorNotFound for short ID, got %T: %v", err, err)
	}

	// Completely invalid ref.
	_, _, err = a.ResolveRef(repo.ID, "not-a-valid-ref")
	if err == nil {
		t.Error("Expected error for invalid ref, got nil")
	}
}
