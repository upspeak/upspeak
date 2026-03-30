package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// createTestRepo is a helper that creates and saves a repository for use in
// node/edge/thread/annotation tests.
func createTestRepo(t *testing.T, a *LocalArchive) *core.Repository {
	t.Helper()
	repo := &core.Repository{
		ID:          core.NewID(),
		Slug:        "test-repo",
		Name:        "Test Repository",
		Description: "A repository for testing",
		OwnerID:     testOwnerID,
	}
	if err := a.SaveRepository(repo); err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	return repo
}

// makeNode builds a Node with sensible defaults for the given repo.
func makeNode(repo *core.Repository, nodeType, subject string) *core.Node {
	return &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        nodeType,
		Subject:     subject,
		ContentType: "text/plain",
		Body:        json.RawMessage(`"test body"`),
		Metadata:    []core.Metadata{{Key: "k", Value: json.RawMessage(`"v"`)}},
		CreatedBy:   testOwnerID,
	}
}

func TestSaveAndGetNode(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "Hello World")

	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	if node.Version != 1 {
		t.Errorf("Expected version 1 after create, got %d", node.Version)
	}
	if node.ShortID != "NODE-1" {
		t.Errorf("Expected short ID NODE-1, got %s", node.ShortID)
	}
	if node.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if node.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}

	got, err := a.GetNode(node.ID)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}

	if got.ID != node.ID {
		t.Errorf("Expected ID %s, got %s", node.ID, got.ID)
	}
	if got.RepoID != repo.ID {
		t.Errorf("Expected RepoID %s, got %s", repo.ID, got.RepoID)
	}
	if got.Type != "note" {
		t.Errorf("Expected type 'note', got %q", got.Type)
	}
	if got.Subject != "Hello World" {
		t.Errorf("Expected subject 'Hello World', got %q", got.Subject)
	}
	if got.ContentType != "text/plain" {
		t.Errorf("Expected content type 'text/plain', got %q", got.ContentType)
	}
	if string(got.Body) != `"test body"` {
		t.Errorf("Expected body '\"test body\"', got %q", string(got.Body))
	}
	if len(got.Metadata) != 1 || got.Metadata[0].Key != "k" {
		t.Errorf("Expected metadata [{k: \"v\"}], got %+v", got.Metadata)
	}
	if got.CreatedBy != testOwnerID {
		t.Errorf("Expected created_by %s, got %s", testOwnerID, got.CreatedBy)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
	if got.ShortID != "NODE-1" {
		t.Errorf("Expected short ID NODE-1, got %s", got.ShortID)
	}
}

func TestSaveNode_Update(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "Original")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update.
	node.Subject = "Updated"
	node.Body = json.RawMessage(`"updated body"`)
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if node.Version != 2 {
		t.Errorf("Expected version 2 after update, got %d", node.Version)
	}

	got, err := a.GetNode(node.ID)
	if err != nil {
		t.Fatalf("GetNode after update failed: %v", err)
	}
	if got.Subject != "Updated" {
		t.Errorf("Expected subject 'Updated', got %q", got.Subject)
	}
	if string(got.Body) != `"updated body"` {
		t.Errorf("Expected body '\"updated body\"', got %q", string(got.Body))
	}
	if got.Version != 2 {
		t.Errorf("Expected version 2, got %d", got.Version)
	}
}

func TestSaveNode_VersionConflict(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "Test")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Simulate a stale version.
	stale := *node
	stale.Version = 999
	stale.Subject = "Stale Update"

	err := a.SaveNode(&stale)
	if err == nil {
		t.Fatal("Expected version conflict error, got nil")
	}

	if _, ok := err.(*core.VersionConflictError); !ok {
		t.Errorf("Expected *VersionConflictError, got %T: %v", err, err)
	}
}

func TestSaveBatchNodes(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	nodes := []*core.Node{
		makeNode(repo, "note", "Batch 1"),
		makeNode(repo, "note", "Batch 2"),
		makeNode(repo, "task", "Batch 3"),
	}

	if err := a.SaveBatchNodes(repo.ID, nodes); err != nil {
		t.Fatalf("SaveBatchNodes failed: %v", err)
	}

	for i, node := range nodes {
		if node.Version != 1 {
			t.Errorf("Node %d: expected version 1, got %d", i, node.Version)
		}
		expectedShortID := core.FormatShortID(core.PrefixNode, i+1)
		if node.ShortID != expectedShortID {
			t.Errorf("Node %d: expected short ID %s, got %s", i, expectedShortID, node.ShortID)
		}

		// Verify it can be retrieved.
		got, err := a.GetNode(node.ID)
		if err != nil {
			t.Fatalf("GetNode for batch node %d failed: %v", i, err)
		}
		if got.Subject != node.Subject {
			t.Errorf("Node %d: expected subject %q, got %q", i, node.Subject, got.Subject)
		}
	}
}

func TestDeleteNode(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	node := makeNode(repo, "note", "To Delete")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := a.DeleteNode(node.ID); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	_, err := a.GetNode(node.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
	if _, ok := err.(*core.ErrorNotFound); !ok {
		t.Errorf("Expected *ErrorNotFound, got %T: %v", err, err)
	}
}

func TestListNodes(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Create 5 nodes: 3 of type "note", 2 of type "task".
	for i := 0; i < 3; i++ {
		node := makeNode(repo, "note", "Note node")
		if err := a.SaveNode(node); err != nil {
			t.Fatalf("SaveNode (note %d) failed: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		node := makeNode(repo, "task", "Task node")
		if err := a.SaveNode(node); err != nil {
			t.Fatalf("SaveNode (task %d) failed: %v", i, err)
		}
	}

	// List all nodes.
	nodes, total, err := a.ListNodes(repo.ID, "", core.ListOptions{Limit: 10, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListNodes (all) failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(nodes) != 5 {
		t.Errorf("Expected 5 nodes, got %d", len(nodes))
	}

	// Filter by type "note".
	notes, noteTotal, err := a.ListNodes(repo.ID, "note", core.ListOptions{Limit: 10, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListNodes (note filter) failed: %v", err)
	}
	if noteTotal != 3 {
		t.Errorf("Expected 3 notes total, got %d", noteTotal)
	}
	if len(notes) != 3 {
		t.Errorf("Expected 3 notes, got %d", len(notes))
	}

	// Test pagination: limit 2, offset 0.
	page1, pagTotal, err := a.ListNodes(repo.ID, "", core.ListOptions{Limit: 2, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListNodes (page 1) failed: %v", err)
	}
	if pagTotal != 5 {
		t.Errorf("Expected total 5 with pagination, got %d", pagTotal)
	}
	if len(page1) != 2 {
		t.Errorf("Expected 2 nodes in page 1, got %d", len(page1))
	}

	// Test pagination: limit 2, offset 4 (should get 1 result).
	page3, _, err := a.ListNodes(repo.ID, "", core.ListOptions{Limit: 2, Offset: 4, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListNodes (page 3) failed: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("Expected 1 node in last page, got %d", len(page3))
	}
}

func TestGetNodeEdges(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	nodeA := makeNode(repo, "note", "Node A")
	nodeB := makeNode(repo, "note", "Node B")
	if err := a.SaveNode(nodeA); err != nil {
		t.Fatalf("SaveNode A failed: %v", err)
	}
	if err := a.SaveNode(nodeB); err != nil {
		t.Fatalf("SaveNode B failed: %v", err)
	}

	// Create 3 edges: 2 outgoing from A, 1 incoming to A (from B).
	edgeOut1 := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      "reply",
		Source:    nodeA.ID,
		Target:    nodeB.ID,
		Label:     "replies to",
		Weight:    1.0,
		CreatedBy: testOwnerID,
	}
	edgeOut2 := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      "link",
		Source:    nodeA.ID,
		Target:    nodeB.ID,
		Label:     "links to",
		Weight:    0.5,
		CreatedBy: testOwnerID,
	}
	edgeIn := &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      "reply",
		Source:    nodeB.ID,
		Target:    nodeA.ID,
		Label:     "replies back",
		Weight:    1.0,
		CreatedBy: testOwnerID,
	}

	for _, e := range []*core.Edge{edgeOut1, edgeOut2, edgeIn} {
		if err := a.SaveEdge(e); err != nil {
			t.Fatalf("SaveEdge failed: %v", err)
		}
	}

	// All edges connected to A (both directions).
	allEdges, allTotal, err := a.GetNodeEdges(nodeA.ID, core.EdgeQueryOptions{
		Direction:   "both",
		ListOptions: core.ListOptions{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("GetNodeEdges (both) failed: %v", err)
	}
	if allTotal != 3 {
		t.Errorf("Expected 3 edges total (both), got %d", allTotal)
	}
	if len(allEdges) != 3 {
		t.Errorf("Expected 3 edges (both), got %d", len(allEdges))
	}

	// Outgoing only from A.
	outEdges, outTotal, err := a.GetNodeEdges(nodeA.ID, core.EdgeQueryOptions{
		Direction:   "outgoing",
		ListOptions: core.ListOptions{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("GetNodeEdges (outgoing) failed: %v", err)
	}
	if outTotal != 2 {
		t.Errorf("Expected 2 outgoing edges, got %d", outTotal)
	}
	if len(outEdges) != 2 {
		t.Errorf("Expected 2 outgoing edges, got %d", len(outEdges))
	}

	// Incoming only to A.
	inEdges, inTotal, err := a.GetNodeEdges(nodeA.ID, core.EdgeQueryOptions{
		Direction:   "incoming",
		ListOptions: core.ListOptions{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("GetNodeEdges (incoming) failed: %v", err)
	}
	if inTotal != 1 {
		t.Errorf("Expected 1 incoming edge, got %d", inTotal)
	}
	if len(inEdges) != 1 {
		t.Errorf("Expected 1 incoming edge, got %d", len(inEdges))
	}

	// Filter by type "reply" (both directions from A).
	replyEdges, replyTotal, err := a.GetNodeEdges(nodeA.ID, core.EdgeQueryOptions{
		Direction:   "both",
		Type:        "reply",
		ListOptions: core.ListOptions{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("GetNodeEdges (type filter) failed: %v", err)
	}
	if replyTotal != 2 {
		t.Errorf("Expected 2 reply edges, got %d", replyTotal)
	}
	if len(replyEdges) != 2 {
		t.Errorf("Expected 2 reply edges, got %d", len(replyEdges))
	}
}

func TestGetNodeAnnotations(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Create a target node.
	target := makeNode(repo, "note", "Target Node")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode (target) failed: %v", err)
	}

	// Create 2 annotations targeting the node.
	for i := 0; i < 2; i++ {
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
				Source:     core.NewID(), // will be the annotation node ID after save
				Target:    target.ID,
				Weight:    1.0,
				CreatedBy: testOwnerID,
			},
		}
		// Set edge source to the annotation's embedded node.
		ann.Edge.Source = ann.Node.ID

		if err := a.SaveAnnotation(ann); err != nil {
			t.Fatalf("SaveAnnotation %d failed: %v", i, err)
		}
	}

	annotations, total, err := a.GetNodeAnnotations(target.ID, core.AnnotationQueryOptions{
		ListOptions: core.ListOptions{Limit: 10, Offset: 0},
	})
	if err != nil {
		t.Fatalf("GetNodeAnnotations failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected 2 annotations, got %d", total)
	}
	if len(annotations) != 2 {
		t.Errorf("Expected 2 annotations, got %d", len(annotations))
	}
}
